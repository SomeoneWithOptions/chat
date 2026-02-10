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

type ThinkingTraceStep = {
  id: string;
  label: string;
  detail: string;
  status: 'pending' | 'active' | 'done';
};

type ThinkingTrace = {
  status: 'running' | 'done' | 'stopped';
  summary: string;
  steps: ThinkingTraceStep[];
};

type ChatMessageProps = {
  message: MessageData;
  isStreaming?: boolean;
  thinkingTrace?: ThinkingTrace | null;
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

export default function ChatMessage({ message, isStreaming, thinkingTrace }: ChatMessageProps) {
  const isUser = message.role === 'user';
  const renderMarkdown = !isUser;
  const isAssistant = message.role === 'assistant';
  const showStreamingIndicator = isStreaming && isAssistant && !message.content;
  const [traceExpanded, setTraceExpanded] = useState(false);
  const showThinkingTrace = isAssistant && !!thinkingTrace && thinkingTrace.steps.length > 0;
  const tracePanelID = `${message.id}-thinking-trace`;

  useEffect(() => {
    setTraceExpanded(false);
  }, [message.id]);

  return (
    <div className={`message ${message.role}`}>
      <div className="message-inner">
        {!isUser && (
          <div className="message-role">
            {message.role}
          </div>
        )}

        <div className={`message-content ${renderMarkdown ? 'markdown' : 'plain'}`}>
          {showThinkingTrace && thinkingTrace && (
            <div className={`thinking-trace ${thinkingTrace.status}`}>
              <button
                type="button"
                className="thinking-trace-toggle"
                onClick={() => setTraceExpanded((open) => !open)}
                aria-expanded={traceExpanded}
                aria-controls={tracePanelID}
              >
                <span className="thinking-trace-heading">
                  <span className="thinking-trace-title">
                    {thinkingTrace.status === 'running' ? 'Thinking' : 'Thought Process'}
                  </span>
                  <span className="thinking-trace-summary">{thinkingTrace.summary}</span>
                </span>
                <svg
                  className={`thinking-trace-chevron ${traceExpanded ? 'open' : ''}`}
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <polyline points="6 9 12 15 18 9" />
                </svg>
              </button>

              {traceExpanded && (
                <ol id={tracePanelID} className="thinking-trace-steps">
                  {thinkingTrace.steps.map((step) => (
                    <li key={step.id} className={`thinking-trace-step ${step.status}`}>
                      <span className="thinking-trace-step-dot" />
                      <div className="thinking-trace-step-content">
                        <span className="thinking-trace-step-label">{step.label}</span>
                        <span className="thinking-trace-step-detail">{step.detail}</span>
                      </div>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          )}

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
            <span className="message-streaming-indicator" aria-live="polite">
              <span className="message-streaming-label">Thinking</span>
              <span className="message-streaming-dots" aria-hidden="true">
                <span />
                <span />
                <span />
              </span>
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

export type { MessageData, ThinkingTrace, ThinkingTraceStep };
