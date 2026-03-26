import { type ChangeEvent, type FormEvent, useEffect, useRef, useState } from 'react';
import { type ReasoningEffort, type UploadedFile, type Model } from '../lib/api';
import ModelDropdown from './ModelDropdown';

type ComposerProps = {
  prompt: string;
  onPromptChange: (value: string) => void;
  onSend: (e: FormEvent<HTMLFormElement>) => void;
  onStop: () => void;
  reasoningOptions: Array<{ value: ReasoningEffort; label: string }>;
  reasoningEffort: ReasoningEffort;
  supportsReasoning: boolean;
  reasoningDisabled: boolean;
  onReasoningEffortChange: (effort: ReasoningEffort) => void;
  grounding: boolean;
  deepResearch: boolean;
  fusionMode: boolean;
  groundingLocked?: boolean;
  onToggleGrounding: () => void;
  onToggleDeepResearch: () => void;
  onToggleFusionMode: () => void;
  isStreaming: boolean;
  uploadingAttachments: boolean;
  pendingAttachments: UploadedFile[];
  onAttachmentChange: (e: ChangeEvent<HTMLInputElement>) => void;
  onRemoveAttachment: (fileId: string) => void;
  onEnhance: () => void;
  enhanceDisabled?: boolean;
  error: string | null;
  streamWarning: string | null;
  sendDisabled?: boolean;
  models?: Model[];
  selectedSourceModels?: string[];
  onSourceModelsChange?: (models: string[]) => void;
  selectedFusionModel?: string | null;
  onFusionModelChange?: (model: string | null) => void;
  compactForActiveFusionRun?: boolean;
};

const acceptedAttachmentTypes = '.txt,.md,.pdf,.csv,.json';

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export default function Composer({
  prompt,
  onPromptChange,
  onSend,
  onStop,
  reasoningOptions,
  reasoningEffort,
  supportsReasoning,
  reasoningDisabled,
  onReasoningEffortChange,
  grounding,
  deepResearch,
  fusionMode,
  groundingLocked = false,
  onToggleGrounding,
  onToggleDeepResearch,
  onToggleFusionMode,
  isStreaming,
  uploadingAttachments,
  pendingAttachments,
  onAttachmentChange,
  onRemoveAttachment,
  error,
  streamWarning,
  sendDisabled = false,
  onEnhance,
  enhanceDisabled = false,
  models = [],
  selectedSourceModels = [],
  onSourceModelsChange,
  selectedFusionModel,
  onFusionModelChange,
  compactForActiveFusionRun = false,
}: ComposerProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [isMobileViewport, setIsMobileViewport] = useState(
    () => typeof window !== 'undefined' && window.innerWidth <= 768,
  );
  const [isActiveFusionRunCollapsed, setIsActiveFusionRunCollapsed] =
    useState(false);

  // Auto-grow: resize textarea to fit content on every prompt change
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';           // collapse first to get accurate scrollHeight
    el.style.height = `${el.scrollHeight}px`; // expand to fit content
  }, [prompt]);

  useEffect(() => {
    function handleResize() {
      setIsMobileViewport(window.innerWidth <= 768);
    }

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const canCompactForActiveFusionRun =
    compactForActiveFusionRun && isMobileViewport;

  useEffect(() => {
    if (canCompactForActiveFusionRun) {
      setIsActiveFusionRunCollapsed(true);
      return;
    }

    setIsActiveFusionRunCollapsed(false);
  }, [canCompactForActiveFusionRun]);

  const canSend = prompt.trim().length > 0 && !isStreaming && !uploadingAttachments && !sendDisabled;
  const showCompactFusionBar =
    canCompactForActiveFusionRun && isActiveFusionRunCollapsed;
  const selectedFusionModelName = selectedFusionModel
    ? models.find((model) => model.id === selectedFusionModel)?.name ||
      selectedFusionModel
    : null;
  const fusionRunTitle = isStreaming ? 'Fusion queued' : 'Fusion in progress';
  const fusionRunDetail =
    selectedSourceModels.length > 0
      ? `${selectedSourceModels.length} source${selectedSourceModels.length === 1 ? '' : 's'}${selectedFusionModelName ? ` with ${selectedFusionModelName}` : ''}`
      : selectedFusionModelName
        ? `Using ${selectedFusionModelName}`
        : 'Status updates are showing in the thread';

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    const isMobile = window.innerWidth < 768;

    if (isMobile && e.key === 'Enter') {
      return;
    }

    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (canSend) {
        const form = (e.target as HTMLTextAreaElement).closest('form');
        if (form) form.requestSubmit();
      }
    }
  }

  return (
    <div className="composer-wrapper">
      {streamWarning && (
        <div className="warning-message">{streamWarning}</div>
      )}
      {error && (
        <div className="error-message">{error}</div>
      )}

      <form className="composer" onSubmit={onSend}>
        <input
          ref={fileInputRef}
          type="file"
          accept={acceptedAttachmentTypes}
          multiple
          onChange={onAttachmentChange}
          className="visually-hidden"
        />

        {canCompactForActiveFusionRun && (
          <div className="composer-mobile-fusion-banner">
            <div className="composer-mobile-fusion-copy">
              <span className="composer-mobile-fusion-eyebrow">{fusionRunTitle}</span>
              <span className="composer-mobile-fusion-title">{fusionRunDetail}</span>
            </div>
            <div className="composer-mobile-fusion-actions">
              {isStreaming && (
                <button
                  type="button"
                  className="composer-mobile-fusion-toggle composer-mobile-fusion-toggle-stop"
                  onClick={onStop}
                >
                  Stop
                </button>
              )}
              <button
                type="button"
                className="composer-mobile-fusion-toggle"
                onClick={() =>
                  setIsActiveFusionRunCollapsed((collapsed) => !collapsed)
                }
              >
                {showCompactFusionBar ? 'Show controls' : 'Hide controls'}
              </button>
            </div>
          </div>
        )}

        {!showCompactFusionBar && (
          <>
            <div className={`composer-reasoning-row ${supportsReasoning ? '' : 'disabled'}`}>
              <span className="composer-reasoning-label">Thinking</span>
              {supportsReasoning ? (
                <select
                  className="reasoning-select"
                  value={reasoningEffort}
                  onChange={(event) => onReasoningEffortChange(event.target.value as ReasoningEffort)}
                  disabled={reasoningDisabled}
                  aria-label="Thinking effort"
                >
                  {reasoningOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="reasoning-unavailable">Unavailable</span>
              )}
            </div>

            {fusionMode && (
              <div className="composer-fusion-tray">
                <div className="fusion-section">
                  <span className="fusion-label">Sources ({selectedSourceModels.length}/5)</span>
                  <div className="fusion-models">
                    {selectedSourceModels.map((id) => {
                      const m = models.find((x) => x.id === id);
                      return (
                        <span key={id} className="fusion-chip">
                          {m?.name || id}
                          <button
                            type="button"
                            onClick={() => onSourceModelsChange?.(selectedSourceModels.filter((x) => x !== id))}
                            disabled={isStreaming}
                          >
                            &times;
                          </button>
                        </span>
                      );
                    })}
                    {selectedSourceModels.length < 5 && (
                      <ModelDropdown
                        models={models}
                        value={null}
                        onChange={(id) => {
                          if (!selectedSourceModels.includes(id)) {
                            onSourceModelsChange?.([...selectedSourceModels, id]);
                          }
                        }}
                        placeholder="+ Add Source"
                        disabledIds={selectedSourceModels}
                        disabled={isStreaming}
                        variant="dashed"
                      />
                    )}
                  </div>
                </div>
                {selectedSourceModels.length > 0 && (
                  <div className="fusion-section">
                    <span className="fusion-label">Fuse with</span>
                    <ModelDropdown
                      models={models}
                      value={selectedFusionModel ?? null}
                      onChange={(id) => onFusionModelChange?.(id)}
                      placeholder="Select Fusion Model"
                      disabled={isStreaming}
                      variant="solid"
                    />
                  </div>
                )}
              </div>
            )}

            <textarea
              ref={textareaRef}
              className="composer-textarea"
              value={prompt}
              onChange={(e) => onPromptChange(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask anything..."
            />

            {pendingAttachments.length > 0 && (
              <div className="composer-attachments">
                {pendingAttachments.map((attachment) => (
                  <div key={attachment.id} className="attachment-chip">
                    <span className="attachment-chip-name">{attachment.filename}</span>
                    <span className="attachment-chip-size">{formatBytes(attachment.sizeBytes)}</span>
                    <button
                      type="button"
                      className="attachment-chip-remove"
                      onClick={() => onRemoveAttachment(attachment.id)}
                      disabled={isStreaming || uploadingAttachments}
                      aria-label="Remove attachment"
                    >
                      &times;
                    </button>
                  </div>
                ))}
              </div>
            )}

            <div className="composer-toolbar">
              <div className="composer-toolbar-left">
                <button
                  type="button"
                  className="btn-attach"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={isStreaming || uploadingAttachments}
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
                  </svg>
                  <span className="attach-text">{uploadingAttachments ? 'Uploading...' : 'Attach'}</span>
                </button>

                <div className="composer-mode-buttons">
                  <button
                    type="button"
                    className={`composer-mode-button ${grounding ? 'active' : 'inactive'}`}
                    onClick={onToggleGrounding}
                    aria-pressed={grounding}
                    title="Grounding"
                    disabled={groundingLocked}
                  >
                  <svg className="mode-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2Z" />
                    <path d="M2 12h20" />
                    <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10Z" />
                  </svg>
                    <span className="mode-text">Grounding</span>
                  </button>
                  <button
                    type="button"
                    className={`composer-mode-button ${deepResearch ? 'active' : 'inactive'}`}
                    onClick={onToggleDeepResearch}
                    aria-pressed={deepResearch}
                    title="Deep Research"
                  >
                    <svg className="mode-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z"/>
                      <path d="M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z"/>
                      <path d="M15 13a4.5 4.5 0 0 1-3-4 4.5 4.5 0 0 1-3 4"/>
                      <path d="M17.599 6.5a3 3 0 0 0 .399-1.375"/>
                      <path d="M6.003 5.125A3 3 0 0 0 6.401 6.5"/>
                      <path d="M3.477 10.896a4 4 0 0 1 .585-.396"/>
                      <path d="M19.938 10.5a4 4 0 0 1 .585.396"/>
                      <path d="M6 18a4 4 0 0 1-1.967-.516"/>
                      <path d="M19.967 17.484A4 4 0 0 1 18 18"/>
                    </svg>
                    <span className="mode-text">Deep Research</span>
                  </button>
                  <button
                    type="button"
                    className={`composer-mode-button ${fusionMode ? 'active' : 'inactive'}`}
                    onClick={onToggleFusionMode}
                    aria-pressed={fusionMode}
                    title="Fusion"
                  >
                    <svg className="mode-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M4 12h6" />
                      <path d="M14 12h6" />
                      <path d="M12 4v6" />
                      <path d="M12 14v6" />
                      <circle cx="12" cy="12" r="2.5" />
                      <circle cx="4" cy="12" r="1.5" />
                      <circle cx="20" cy="12" r="1.5" />
                      <circle cx="12" cy="4" r="1.5" />
                      <circle cx="12" cy="20" r="1.5" />
                    </svg>
                    <span className="mode-text">Fusion</span>
                  </button>
                  <button
                    type="button"
                    className="composer-mode-button composer-enhance-button"
                    onClick={onEnhance}
                    disabled={enhanceDisabled}
                    title="Enhance prompt"
                  >
                    <svg className="mode-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z" />
                      <path d="M20 3v4" />
                      <path d="M22 5h-4" />
                    </svg>
                    <span className="mode-text">Enhance</span>
                  </button>
                </div>
              </div>

              <div className="composer-toolbar-right">
                {isStreaming ? (
                  <button
                    type="button"
                    className="btn-send btn-stop"
                    onClick={onStop}
                  >
                    Stop
                  </button>
                ) : (
                  <button
                    type="submit"
                    className="btn-send"
                    disabled={!canSend}
                  >
                    <>
                      Send
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                        <line x1="22" y1="2" x2="11" y2="13" />
                        <polygon points="22 2 15 22 11 13 2 9 22 2" />
                      </svg>
                    </>
                  </button>
                )}
              </div>
            </div>
          </>
        )}
      </form>
    </div>
  );
}
