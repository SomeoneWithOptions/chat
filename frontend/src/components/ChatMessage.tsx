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

const markdownComponents: Components = {
  a: ({ node: _node, ...props }) => (
    <a {...props} target="_blank" rel="noreferrer" />
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
