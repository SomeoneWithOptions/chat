import { isValidElement, type HTMLAttributes, type ReactNode, useEffect, useState } from 'react';
import { type Citation, type ProgressDecision, type ThinkingTrace, type Usage } from '../lib/api';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';

type MessageData = {
  id: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  reasoningContent?: string;
  thinkingTrace?: ThinkingTrace | null;
  modelId?: string | null;
  usage?: Usage | null;
  citations: Citation[];
};

type ChatMessageProps = {
  message: MessageData;
  isStreaming?: boolean;
  isEditing?: boolean;
  editDraft?: string;
  disableUserActions?: boolean;
  onStartEdit?: (messageID: string, content: string) => void;
  onEditDraftChange?: (value: string) => void;
  onSaveEdit?: () => void;
  onCancelEdit?: () => void;
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

function formatTraceCounter(entry: ThinkingTrace['entries'][number]): string | null {
  const counters: string[] = [];
  if (entry.loop && entry.maxLoops) counters.push(`loop ${entry.loop}/${entry.maxLoops}`);
  if (entry.pass && entry.totalPasses) counters.push(`${entry.pass}/${entry.totalPasses}`);
  if (entry.sourcesRead !== undefined || entry.sourcesConsidered !== undefined) {
    counters.push(`sources ${entry.sourcesRead ?? 0}/${entry.sourcesConsidered ?? 0}`);
  }
  return counters.length > 0 ? counters.join(' · ') : null;
}

function decisionLabel(decision: ProgressDecision | undefined): string | null {
  if (decision === 'fallback') return 'Fallback path';
  if (decision === 'finalize') return 'Ready to finalize';
  return null;
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

export default function ChatMessage({
  message,
  isStreaming,
  isEditing = false,
  editDraft = '',
  disableUserActions = false,
  onStartEdit,
  onEditDraftChange,
  onSaveEdit,
  onCancelEdit,
}: ChatMessageProps) {
  const isUser = message.role === 'user';
  const renderMarkdown = !isUser;
  const isAssistant = message.role === 'assistant';
  const thinkingTrace = message.thinkingTrace ?? null;
  const hasThinkingTrace = isAssistant && !!thinkingTrace && thinkingTrace.entries.length > 0;
  const hasReasoningContent = isAssistant && !!message.reasoningContent && message.reasoningContent.length > 0;
  const showGenerationTrace = hasThinkingTrace || hasReasoningContent;
  const showStreamingIndicator = isStreaming && isAssistant && !message.content && !showGenerationTrace;
  const [copiedUserMessage, setCopiedUserMessage] = useState(false);
  const [generationExpanded, setGenerationExpanded] = useState(false);
  const [reasoningExpanded, setReasoningExpanded] = useState(false);
  const [sourcesExpanded, setSourcesExpanded] = useState(false);
  const [usageExpanded, setUsageExpanded] = useState(false);
  const generationPanelID = `${message.id}-generation-trace`;
  const generationStatus = thinkingTrace?.status ?? (isStreaming ? 'running' : 'done');
  const generationSummary = thinkingTrace?.summary?.trim() || (isStreaming ? 'Working on your request' : 'Thought process');
  const generationStatusLabel =
    generationStatus === 'running' ? 'Running' : generationStatus === 'stopped' ? 'Stopped' : 'Complete';
  const reasoningPanelID = `${message.id}-reasoning`;
  const sourcesPanelID = `${message.id}-sources`;
  const usagePanelID = `${message.id}-usage`;
  const hasUsage = isAssistant && !!message.usage;

  useEffect(() => {
    setGenerationExpanded(false);
    setReasoningExpanded(false);
    setSourcesExpanded(false);
    setUsageExpanded(false);
  }, [message.id]);

  useEffect(() => {
    if (thinkingTrace && thinkingTrace.status !== 'running') {
      setGenerationExpanded(false);
    }
  }, [thinkingTrace?.status, thinkingTrace]);

  const usagePreview = message.usage
    ? `${formatTotalCostMicros(message.usage.costMicrosUsd, message.usage.byokInferenceCostMicrosUsd)} / ${formatTokensPerSecond(message.usage.tokensPerSecond)}`
    : '';
  const usageModel = message.usage?.modelId ?? message.modelId ?? 'Unavailable';
  const usageProvider = message.usage?.providerName ?? 'Unavailable';

  useEffect(() => {
    if (!copiedUserMessage) return;
    const timeoutId = window.setTimeout(() => setCopiedUserMessage(false), 1800);
    return () => window.clearTimeout(timeoutId);
  }, [copiedUserMessage]);

  async function handleCopyUserMessage() {
    const didCopy = await copyToClipboard(message.content);
    if (didCopy) setCopiedUserMessage(true);
  }

  function handleStartEdit() {
    onStartEdit?.(message.id, message.content);
  }

  const canSaveEdit = editDraft.trim().length > 0 && !disableUserActions;

  return (
    <div className={`message ${message.role}`}>
      <div className="message-inner">
        {!isUser && (
          <div className="message-role">
            {message.role}
          </div>
        )}

        <div className={`message-content ${renderMarkdown ? 'markdown' : 'plain'}`}>
          {showGenerationTrace && (
            <div className={`generation-trace ${generationStatus} ${isStreaming ? 'streaming' : ''}`}>
              <button
                type="button"
                className="generation-trace-toggle"
                onClick={() => setGenerationExpanded((open) => !open)}
                aria-expanded={generationExpanded}
                aria-controls={generationPanelID}
              >
                <span className="generation-trace-heading">
                  <span className="generation-trace-title">Thinking</span>
                  <span className="generation-trace-summary">{generationSummary}</span>
                </span>
                <span className={`generation-trace-status ${generationStatus}`}>{generationStatusLabel}</span>
                <svg
                  className={`generation-trace-chevron ${generationExpanded ? 'open' : ''}`}
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
                id={generationPanelID}
                className={`generation-trace-content ${generationExpanded ? 'expanded' : 'collapsed'}`}
              >
                {generationExpanded && (
                  <>
                    {hasThinkingTrace && thinkingTrace && (
                      <ol className="generation-trace-entries">
                        {thinkingTrace.entries.map((entry, index) => {
                          const counter = formatTraceCounter(entry);
                          const decision = decisionLabel(entry.decision);
                          return (
                            <li key={`${message.id}-trace-${index}`} className="generation-trace-entry">
                              <div className="generation-trace-entry-row">
                                <span className="generation-trace-entry-title">{entry.title}</span>
                                {counter && <span className="generation-trace-entry-counter">{counter}</span>}
                              </div>
                              {entry.detail && <p className="generation-trace-entry-detail">{entry.detail}</p>}
                              {decision && <p className="generation-trace-entry-decision">{decision}</p>}
                            </li>
                          );
                        })}
                      </ol>
                    )}

                    {hasReasoningContent && (
                      <div className="generation-reasoning">
                        <button
                          type="button"
                          className="generation-reasoning-toggle"
                          onClick={() => setReasoningExpanded((open) => !open)}
                          aria-expanded={reasoningExpanded}
                          aria-controls={reasoningPanelID}
                        >
                          <span className="generation-reasoning-title">Model reasoning</span>
                          <svg
                            className={`generation-reasoning-chevron ${reasoningExpanded ? 'open' : ''}`}
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
                          className={`generation-reasoning-content ${reasoningExpanded ? 'expanded' : 'collapsed'}`}
                        >
                          {reasoningExpanded && (
                            <div className="generation-reasoning-markdown">
                              <ReactMarkdown
                                remarkPlugins={[remarkGfm]}
                                skipHtml
                                components={markdownComponents}
                              >
                                {message.reasoningContent || ''}
                              </ReactMarkdown>
                            </div>
                          )}
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
          )}

          {isUser && isEditing ? (
            <div className="message-user-edit-shell">
              <textarea
                className="message-user-edit-textarea"
                value={editDraft}
                onChange={(event) => onEditDraftChange?.(event.target.value)}
                disabled={disableUserActions}
                aria-label="Edit message"
              />
            </div>
          ) : renderMarkdown ? (
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

        {isUser && (
          <div className="message-user-actions">
            <button
              type="button"
              className={`message-user-copy-button ${copiedUserMessage ? 'copied' : ''}`}
              onClick={handleCopyUserMessage}
              disabled={!message.content || isEditing}
              aria-label={copiedUserMessage ? 'Message copied' : 'Copy message'}
              title={copiedUserMessage ? 'Message copied' : 'Copy message'}
            >
              {copiedUserMessage ? (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <rect x="9" y="9" width="11" height="11" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
              )}
            </button>
            <button
              type="button"
              className={`message-user-edit-button ${isEditing ? 'editing' : ''}`}
              onClick={handleStartEdit}
              disabled={disableUserActions || isEditing}
              aria-label={isEditing ? 'Editing message' : 'Edit message'}
              title={isEditing ? 'Editing message' : 'Edit message'}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z" />
              </svg>
            </button>
            {isEditing && (
              <div className="message-user-edit-controls">
                <button
                  type="button"
                  className="message-user-edit-cancel"
                  onClick={onCancelEdit}
                  disabled={disableUserActions}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  className="message-user-edit-save"
                  onClick={onSaveEdit}
                  disabled={!canSaveEdit}
                >
                  Save &amp; Resend
                </button>
              </div>
            )}
          </div>
        )}

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

export type { MessageData };
