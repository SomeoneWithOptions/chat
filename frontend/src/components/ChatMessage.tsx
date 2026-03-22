import { isValidElement, type HTMLAttributes, type ReactNode, useEffect, useState } from 'react';
import { type AgentSummary, type Citation, type ProgressDecision, type ThinkingTrace, type Usage, type CouncilSourceResult, type CouncilAnalysis } from '../lib/api';
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
  responseMode?: 'chat' | 'deep_research' | 'agent';
  agentSummaries?: AgentSummary[];
  agentSources?: import('../lib/api').CouncilSourceResult[];
  agentAnalysis?: import('../lib/api').CouncilAnalysis;
  agentResultModelId?: string;
  agentResultUsage?: Usage;
  agentRunId?: string;
  citations: Citation[];
  groundingEnabled?: boolean;
  deepResearchEnabled?: boolean;
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

type StatusTone = 'neutral' | 'running' | 'done' | 'warning' | 'stopped';

function phaseChipLabel(phase: ThinkingTrace['entries'][number]['phase']): string {
  switch (phase) {
    case 'planning':
      return 'Planning';
    case 'searching':
      return 'Searching';
    case 'reading':
      return 'Reading';
    case 'evaluating':
      return 'Checking';
    case 'iterating':
      return 'Refining';
    case 'synthesizing':
      return 'Writing';
    case 'finalizing':
      return 'Finishing';
    default:
      return 'Thinking';
  }
}

function generationStatusMeta(status: NonNullable<ThinkingTrace['status']>): { label: string; tone: StatusTone; animate: boolean } {
  if (status === 'running') return { label: 'Thinking', tone: 'running', animate: true };
  if (status === 'stopped') return { label: 'Paused', tone: 'stopped', animate: false };
  return { label: 'Ready', tone: 'done', animate: false };
}

function sourceStatusMeta(status: CouncilSourceResult['status']): { label: string; tone: StatusTone; animate: boolean; description: string } {
  switch (status) {
    case 'queued':
      return {
        label: 'Starting',
        tone: 'neutral',
        animate: true,
        description: 'Preparing this model to start its pass.',
      };
    case 'running':
      return {
        label: 'Thinking',
        tone: 'running',
        animate: true,
        description: 'Reviewing sources and drafting a candidate answer.',
      };
    case 'degraded':
      return {
        label: 'Partial',
        tone: 'warning',
        animate: false,
        description: 'Finished with warnings or missing pieces.',
      };
    case 'failed':
      return {
        label: 'Stopped',
        tone: 'stopped',
        animate: false,
        description: 'This pass stopped before it could finish.',
      };
    default:
      return {
        label: 'Ready',
        tone: 'done',
        animate: false,
        description: 'This model finished its pass and is ready to inspect.',
      };
  }
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

function ThinkingStatusChip({
  label,
  tone,
  animate,
  className = '',
}: {
  label: string;
  tone: StatusTone;
  animate: boolean;
  className?: string;
}) {
  const classes = ['thinking-status-chip', tone, animate ? 'is-animated' : '', className]
    .filter(Boolean)
    .join(' ');

  return (
    <span className={classes}>
      <span className="thinking-status-chip-label">{label}</span>
      {animate ? (
        <span className="thinking-status-chip-dots" aria-hidden="true">
          <span />
          <span />
          <span />
        </span>
      ) : (
        <span className="thinking-status-chip-mark" aria-hidden="true" />
      )}
    </span>
  );
}

function CouncilSourceCard({ source, isStreaming }: { source: CouncilSourceResult; isStreaming: boolean }) {
  const [expanded, setExpanded] = useState(false);

  const isWorking = source.status === 'queued' || source.status === 'running';
  const statusMeta = sourceStatusMeta(source.status);

  return (
    <div className={`council-source-card status-${source.status}`}>
      <button className="council-source-header" onClick={() => setExpanded(!expanded)} type="button">
        <span className="council-source-heading">
          <span className="council-source-title">{source.modelId}</span>
          <span className="council-source-summary">{statusMeta.description}</span>
        </span>
        <div className="council-source-badges">
          {source.readableSources !== undefined && (
            <span className="badge" title={`${source.readableSources} readable sources`}>
              {source.readableSources}/15 sources
            </span>
          )}
          {source.durationMs !== undefined && (
            <span className="badge">{(source.durationMs / 1000).toFixed(1)}s</span>
          )}
          <ThinkingStatusChip
            label={statusMeta.label}
            tone={statusMeta.tone}
            animate={statusMeta.animate}
            className="council-source-status"
          />
        </div>
        <svg
          className={`chevron ${expanded ? 'open' : ''}`}
          viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>

      {expanded && (
        <div className="council-source-body">
          {source.error ? (
            <div className="error-message">{source.error}</div>
          ) : source.response ? (
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{source.response}</ReactMarkdown>
          ) : (isWorking || isStreaming) ? (
            <div className="council-source-placeholder">
              <ThinkingStatusChip label={statusMeta.label} tone={statusMeta.tone} animate={statusMeta.animate} />
              <p>{statusMeta.description}</p>
            </div>
          ) : (
            <p className="council-source-empty">No source response was recorded for this pass.</p>
          )}
        </div>
      )}
    </div>
  );
}

function CouncilAnalysisView({ analysis }: { analysis: CouncilAnalysis }) {
  return (
    <div className="council-analysis-view">
      <h3 className="council-section-title">Analysis</h3>
      {analysis.agreement && analysis.agreement.length > 0 && (
        <details className="analysis-category">
          <summary>Agreement</summary>
          <ul>
            {analysis.agreement.map((item, i) => (
              <li key={i}>
                {item.point} {item.sourceModels && item.sourceModels.length > 0 && <span className="source-badges">({item.sourceModels.join(', ')})</span>}
              </li>
            ))}
          </ul>
        </details>
      )}
      {analysis.keyDifferences && analysis.keyDifferences.length > 0 && (
        <details className="analysis-category">
          <summary>Key Differences</summary>
          {analysis.keyDifferences.map((group, i) => (
            <div key={i} className="difference-group">
              <strong>{group.topic}</strong>
              <ul>
                {group.positions.map((pos, j) => (
                  <li key={j}><em>{pos.sourceModel}:</em> {pos.summary}</li>
                ))}
              </ul>
            </div>
          ))}
        </details>
      )}
      {analysis.partialCoverage && analysis.partialCoverage.length > 0 && (
        <details className="analysis-category">
          <summary>Partial Coverage</summary>
          <ul>
            {analysis.partialCoverage.map((item, i) => (
              <li key={i}>
                {item.point} {item.sourceModels && item.sourceModels.length > 0 && <span className="source-badges">({item.sourceModels.join(', ')})</span>}
              </li>
            ))}
          </ul>
        </details>
      )}
      {analysis.uniqueInsights && analysis.uniqueInsights.length > 0 && (
        <details className="analysis-category">
          <summary>Unique Insights</summary>
          <ul>
            {analysis.uniqueInsights.map((item, i) => (
              <li key={i}>
                {item.point} {item.sourceModels && item.sourceModels.length > 0 && <span className="source-badges">({item.sourceModels.join(', ')})</span>}
              </li>
            ))}
          </ul>
        </details>
      )}
      {analysis.blindSpots && analysis.blindSpots.length > 0 && (
        <details className="analysis-category">
          <summary>Blind Spots</summary>
          <ul>
            {analysis.blindSpots.map((item, i) => (
              <li key={i}>{item.point}</li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

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
  const [agentsExpanded, setAgentsExpanded] = useState(false);
  const [sourcesExpanded, setSourcesExpanded] = useState(false);
  const [usageExpanded, setUsageExpanded] = useState(false);
  const generationPanelID = `${message.id}-generation-trace`;
  const generationStatus = thinkingTrace?.status ?? (isStreaming ? 'running' : 'done');
  const generationStatusPresentation = generationStatusMeta(generationStatus);
  const generationSummary = thinkingTrace?.summary?.trim() || (isStreaming ? 'Working on your request' : 'Thought process');
  const reasoningPanelID = `${message.id}-reasoning`;
  const agentsPanelID = `${message.id}-agents`;
  const sourcesPanelID = `${message.id}-sources`;
  const usagePanelID = `${message.id}-usage`;
  const hasUsage = isAssistant && !!message.usage;
  const hasAgentSummaries = isAssistant && !!message.agentSummaries && message.agentSummaries.length > 0;
  const latestTraceIndex = thinkingTrace?.entries.length ? thinkingTrace.entries.length - 1 : -1;

  useEffect(() => {
    setGenerationExpanded(false);
    setReasoningExpanded(false);
    setAgentsExpanded(false);
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
                  <ThinkingStatusChip
                    label={generationStatusPresentation.label}
                    tone={generationStatusPresentation.tone}
                    animate={generationStatusPresentation.animate}
                  />
                  <span className="generation-trace-summary">{generationSummary}</span>
                </span>
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
                          const isCurrentStep = generationStatus === 'running' && index === latestTraceIndex;
                          return (
                            <li
                              key={`${message.id}-trace-${index}`}
                              className={`generation-trace-entry ${isCurrentStep ? 'current' : ''}`}
                            >
                              <div className="generation-trace-entry-row">
                                <span className="generation-trace-entry-heading">
                                  <ThinkingStatusChip
                                    label={phaseChipLabel(entry.phase)}
                                    tone={isCurrentStep ? 'running' : 'neutral'}
                                    animate={isCurrentStep}
                                    className="generation-trace-entry-chip"
                                  />
                                  <span className="generation-trace-entry-title">{entry.title}</span>
                                </span>
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

          {message.agentSources && message.agentSources.length > 0 && (
            <div className="council-sources-container">
              <h3 className="council-section-title">Sources</h3>
              <div className="council-sources-list">
                {message.agentSources.map((source, idx) => (
                  <CouncilSourceCard key={idx} source={source} isStreaming={!!isStreaming} />
                ))}
              </div>
            </div>
          )}

          {message.agentAnalysis && (
            <CouncilAnalysisView analysis={message.agentAnalysis} />
          )}

          {((message.agentResultModelId || message.agentSources?.length)) ? (
            <div className="council-result-container">
              <h3 className="council-section-title">
                Result {message.agentResultModelId && <span className="council-fused-badge">Fused by {message.agentResultModelId}</span>}
              </h3>
            </div>
          ) : null}

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
            <ThinkingStatusChip
              label="Thinking"
              tone="running"
              animate
              className="message-streaming-indicator"
            />
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

        {hasAgentSummaries && message.agentSummaries && (
          <div className="agent-summaries">
            <button
              type="button"
              className="agent-summaries-toggle"
              onClick={() => setAgentsExpanded((open) => !open)}
              aria-expanded={agentsExpanded}
              aria-controls={agentsPanelID}
            >
              <span className="agent-summaries-heading">
                <span className="agent-summaries-title">Agents</span>
                {!agentsExpanded && (
                  <span className="agent-summaries-count">
                    {message.agentSummaries.length} summaries
                  </span>
                )}
              </span>
              <svg
                className={`agent-summaries-chevron ${agentsExpanded ? 'open' : ''}`}
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
              id={agentsPanelID}
              className={`agent-summaries-content ${agentsExpanded ? 'expanded' : 'collapsed'}`}
            >
              {agentsExpanded && (
                <ol className="agent-summaries-list">
                  {message.agentSummaries.map((summary) => (
                    <li key={`${message.id}-${summary.role}`} className="agent-summary-item">
                      <div className="agent-summary-head">
                        <span className="agent-summary-role">{summary.role}</span>
                        {typeof summary.confidence === 'number' && (
                          <span className="agent-summary-confidence">{Math.round(summary.confidence * 100)}%</span>
                        )}
                      </div>
                      <p className="agent-summary-text">{summary.summary}</p>
                      {summary.objections && summary.objections.length > 0 && (
                        <p className="agent-summary-objections">{summary.objections.join(' · ')}</p>
                      )}
                    </li>
                  ))}
                </ol>
              )}
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
