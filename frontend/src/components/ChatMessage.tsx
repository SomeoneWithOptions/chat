import { isValidElement, type HTMLAttributes, type ReactNode, useEffect, useState } from 'react';
import { type Citation, type Usage } from '../lib/api';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';

type MessageData = {
  id: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  reasoningContent?: string;
  modelId?: string | null;
  usage?: Usage | null;
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

  const trimmedSnippet = citation.snippet?.trim();
  if (trimmedSnippet) {
    const preview = trimmedSnippet.replace(/\s+/g, ' ');
    if (preview.length <= 96) return preview;
    return `${preview.slice(0, 93).trimEnd()}...`;
  }

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

function formatCostMicros(micros?: number): string {
  if (micros === undefined) return 'Unavailable';
  const dollars = micros / 1_000_000;
  return `$${dollars.toFixed(6)}`;
}

function formatTotalCostMicros(costMicros?: number, byokCostMicros?: number): string {
  if (costMicros === undefined && byokCostMicros === undefined) return 'Unavailable';
  return formatCostMicros((costMicros ?? 0) + (byokCostMicros ?? 0));
}

function formatTokensPerSecond(tokensPerSecond?: number): string {
  if (tokensPerSecond === undefined) return 'Unavailable';
  return `${tokensPerSecond.toFixed(2)} tok/s`;
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
  const [reasoningExpanded, setReasoningExpanded] = useState(false);
  const [sourcesExpanded, setSourcesExpanded] = useState(false);
  const [usageExpanded, setUsageExpanded] = useState(false);
  const showThinkingTrace = isAssistant && !!thinkingTrace && thinkingTrace.steps.length > 0;
  const tracePanelID = `${message.id}-thinking-trace`;
  const reasoningPanelID = `${message.id}-reasoning`;
  const sourcesPanelID = `${message.id}-sources`;
  const usagePanelID = `${message.id}-usage`;
  
  // Show reasoning panel if there's reasoning content (either persisted or streaming)
  const hasReasoningContent = isAssistant && !!message.reasoningContent && message.reasoningContent.length > 0;
  // Auto-expand reasoning during streaming when no content yet, auto-collapse when content starts
  const isReasoningStreaming = isStreaming && isAssistant && hasReasoningContent && !message.content;
  const hasUsage = isAssistant && !!message.usage;

  useEffect(() => {
    setTraceExpanded(false);
    setReasoningExpanded(false);
    setSourcesExpanded(false);
    setUsageExpanded(false);
  }, [message.id]);

  // Auto-expand reasoning panel during streaming when reasoning arrives but content hasn't started
  useEffect(() => {
    if (isReasoningStreaming) {
      setReasoningExpanded(true);
    } else if (isStreaming && message.content) {
      // Auto-collapse when content starts arriving
      setReasoningExpanded(false);
    }
  }, [isReasoningStreaming, isStreaming, message.content]);

  // Generate a preview of reasoning content (first ~100 chars)
  const reasoningPreview = message.reasoningContent
    ? message.reasoningContent.slice(0, 100).replace(/\n/g, ' ').trim() + (message.reasoningContent.length > 100 ? '...' : '')
    : '';
  const usagePreview = message.usage
    ? `${formatTotalCostMicros(message.usage.costMicrosUsd, message.usage.byokInferenceCostMicrosUsd)} / ${formatTokensPerSecond(message.usage.tokensPerSecond)}`
    : '';
  const usageModel = message.usage?.modelId ?? message.modelId ?? 'Unavailable';
  const usageProvider = message.usage?.providerName ?? 'Unavailable';

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

          {hasReasoningContent && (
            <div className={`reasoning-trace ${isReasoningStreaming ? 'streaming' : ''}`}>
              <button
                type="button"
                className="reasoning-trace-toggle"
                onClick={() => setReasoningExpanded((open) => !open)}
                aria-expanded={reasoningExpanded}
                aria-controls={reasoningPanelID}
              >
                <span className="reasoning-trace-heading">
                  <svg
                    className="reasoning-trace-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z" />
                    <path d="M9 21h6" />
                    <path d="M9 18h6" />
                  </svg>
                  <span className="reasoning-trace-title">
                    {isReasoningStreaming ? 'Reasoning' : 'Model Reasoning'}
                  </span>
                  {!reasoningExpanded && (
                    <span className="reasoning-trace-preview">{reasoningPreview}</span>
                  )}
                </span>
                <svg
                  className={`reasoning-trace-chevron ${reasoningExpanded ? 'open' : ''}`}
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

              <div
                id={reasoningPanelID}
                className={`reasoning-trace-content ${reasoningExpanded ? 'expanded' : 'collapsed'}`}
              >
                <div className="reasoning-trace-markdown">
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    skipHtml
                    components={markdownComponents}
                  >
                    {message.reasoningContent || ''}
                  </ReactMarkdown>
                </div>
              </div>
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
          <div className="grounding-sources">
            <button
              type="button"
              className="grounding-sources-toggle"
              onClick={() => setSourcesExpanded((open) => !open)}
              aria-expanded={sourcesExpanded}
              aria-controls={sourcesPanelID}
            >
              <span className="grounding-sources-heading">
                <svg
                  className="grounding-sources-icon"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <circle cx="12" cy="12" r="10" />
                  <path d="M2 12h20" />
                  <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                </svg>
                <span className="grounding-sources-title">Sources</span>
                {!sourcesExpanded && (
                  <span className="grounding-sources-count">
                    {message.citations.length} {message.citations.length === 1 ? 'source' : 'sources'}
                  </span>
                )}
              </span>
              <svg
                className={`grounding-sources-chevron ${sourcesExpanded ? 'open' : ''}`}
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

            <div
              id={sourcesPanelID}
              className={`grounding-sources-content ${sourcesExpanded ? 'expanded' : 'collapsed'}`}
            >
              <ol className="grounding-sources-list">
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
                  </li>
                ))}
              </ol>
            </div>
          </div>
        )}

        {hasUsage && message.usage && (
          <div className="llm-usage">
            <button
              type="button"
              className="llm-usage-toggle"
              onClick={() => setUsageExpanded((open) => !open)}
              aria-expanded={usageExpanded}
              aria-controls={usagePanelID}
            >
              <span className="llm-usage-heading">
                <svg
                  className="llm-usage-icon"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <rect x="4" y="4" width="16" height="16" rx="2" />
                  <path d="M9 9h6" />
                  <path d="M9 13h6" />
                  <path d="M9 17h4" />
                </svg>
                <span className="llm-usage-title">Usage</span>
                {!usageExpanded && (
                  <span className="llm-usage-preview">{usagePreview}</span>
                )}
              </span>
              <svg
                className={`llm-usage-chevron ${usageExpanded ? 'open' : ''}`}
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

            <div
              id={usagePanelID}
              className={`llm-usage-content ${usageExpanded ? 'expanded' : 'collapsed'}`}
            >
              <dl className="llm-usage-grid">
                <div className="llm-usage-row">
                  <dt>Model</dt>
                  <dd>{usageModel}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>Provider</dt>
                  <dd>{usageProvider}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>Input tokens</dt>
                  <dd>{message.usage.promptTokens.toLocaleString()}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>Output tokens</dt>
                  <dd>{message.usage.completionTokens.toLocaleString()}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>Total tokens</dt>
                  <dd>{message.usage.totalTokens.toLocaleString()}</dd>
                </div>
                {typeof message.usage.reasoningTokens === 'number' && (
                  <div className="llm-usage-row">
                    <dt>Reasoning tokens</dt>
                    <dd>{message.usage.reasoningTokens.toLocaleString()}</dd>
                  </div>
                )}
                <div className="llm-usage-row">
                  <dt>Price (USD)</dt>
                  <dd>{formatCostMicros(message.usage.costMicrosUsd)}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>BYOK inference (USD)</dt>
                  <dd>{formatCostMicros(message.usage.byokInferenceCostMicrosUsd)}</dd>
                </div>
                <div className="llm-usage-row">
                  <dt>Tokens per second</dt>
                  <dd>{formatTokensPerSecond(message.usage.tokensPerSecond)}</dd>
                </div>
              </dl>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export type { MessageData, ThinkingTrace, ThinkingTraceStep };
