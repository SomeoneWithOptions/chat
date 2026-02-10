import { isValidElement, type HTMLAttributes, type ReactNode, useEffect, useState } from 'react';
import { type Citation } from '../lib/api';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';

type MessageData = {
  id: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  citations: Citation[];
};

type ChatMessageProps = {
  message: MessageData;
  isStreaming?: boolean;
};

function citationLabel(citation: Citation, index: number): string {
  const trimmedTitle = citation.title?.trim();
  if (trimmedTitle) return trimmedTitle;
  try {
    const parsed = new URL(citation.url);
    return parsed.hostname.replace(/^www\./, '');
  } catch {
    return `Source ${index + 1}`;
  }
}

function extractNodeText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractNodeText).join('');
  if (isValidElement<{ children?: ReactNode }>(node)) {
    return extractNodeText(node.props.children);
  }
  return '';
}

async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false;

  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall back to document.execCommand for non-secure contexts.
    }
  }

  if (typeof document === 'undefined') return false;

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-9999px';
  document.body.appendChild(textarea);
  textarea.select();

  let copied = false;
  try {
    copied = document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }

  return copied;
}

function MarkdownCodeBlock({ children, ...props }: HTMLAttributes<HTMLPreElement>) {
  const [copied, setCopied] = useState(false);
  const codeText = extractNodeText(children).replace(/\n$/, '');

  useEffect(() => {
    if (!copied) return;
    const timeoutId = window.setTimeout(() => setCopied(false), 1800);
    return () => window.clearTimeout(timeoutId);
  }, [copied]);

  async function handleCopy() {
    const didCopy = await copyToClipboard(codeText);
    if (didCopy) setCopied(true);
  }

  return (
    <div className="markdown-code-block">
      <button
        type="button"
        className="code-copy-button"
        onClick={handleCopy}
        disabled={!codeText}
        aria-label={copied ? 'Code copied' : 'Copy code'}
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
      <pre {...props}>{children}</pre>
    </div>
  );
}

const markdownComponents: Components = {
  a: ({ node: _node, ...props }) => (
    <a {...props} target="_blank" rel="noreferrer" />
  ),
  pre: ({ node: _node, ...props }) => (
    <MarkdownCodeBlock {...props} />
  ),
};

export default function ChatMessage({ message, isStreaming }: ChatMessageProps) {
  const isUser = message.role === 'user';
  const renderMarkdown = !isUser;
  const isAssistant = message.role === 'assistant';
  const showStreamingIndicator = isStreaming && isAssistant && !message.content;

  return (
    <div className={`message ${message.role}`}>
      <div className="message-inner">
        {!isUser && (
          <div className="message-role">
            {message.role}
          </div>
        )}

        <div className={`message-content ${renderMarkdown ? 'markdown' : 'plain'}`}>
          {renderMarkdown ? (
            <div className="message-markdown">
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                skipHtml
                components={markdownComponents}
              >
                {message.content || ''}
              </ReactMarkdown>
            </div>
          ) : (
            message.content || ''
          )}
          {showStreamingIndicator && (
            <span className="message-streaming-indicator">
              <span />
              <span />
              <span />
            </span>
          )}
        </div>

        {message.citations.length > 0 && (
          <ol className="citations">
            {message.citations.map((citation, index) => (
              <li key={`${message.id}-cit-${index}`} className="citation-item">
                <a
                  href={citation.url}
                  target="_blank"
                  rel="noreferrer"
                  className="citation-link"
                >
                  <span className="citation-number">{index + 1}</span>
                  {citationLabel(citation, index)}
                </a>
                {citation.snippet && (
                  <p className="citation-snippet">{citation.snippet}</p>
                )}
              </li>
            ))}
          </ol>
        )}
      </div>
    </div>
  );
}

export type { MessageData };
