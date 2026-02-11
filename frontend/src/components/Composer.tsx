import { type ChangeEvent, type FormEvent, useRef } from 'react';
import { type UploadedFile } from '../lib/api';

type ComposerProps = {
  prompt: string;
  onPromptChange: (value: string) => void;
  onSend: (e: FormEvent<HTMLFormElement>) => void;
  onStop: () => void;
  isStreaming: boolean;
  uploadingAttachments: boolean;
  pendingAttachments: UploadedFile[];
  onAttachmentChange: (e: ChangeEvent<HTMLInputElement>) => void;
  onRemoveAttachment: (fileId: string) => void;
  error: string | null;
  streamWarning: string | null;
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
  isStreaming,
  uploadingAttachments,
  pendingAttachments,
  onAttachmentChange,
  onRemoveAttachment,
  error,
  streamWarning,
}: ComposerProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);

  const canSend = prompt.trim().length > 0 && !isStreaming && !uploadingAttachments;

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
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

        <textarea
          className="composer-textarea"
          value={prompt}
          onChange={(e) => onPromptChange(e.target.value)}
          onKeyDown={handleKeyDown}
          onFocus={() => document.documentElement.setAttribute('data-composer-focused', '')}
          onBlur={() => document.documentElement.removeAttribute('data-composer-focused')}
          placeholder="Ask anything..."
          rows={2}
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
              {uploadingAttachments ? 'Uploading...' : 'Attach'}
            </button>
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
      </form>
    </div>
  );
}
