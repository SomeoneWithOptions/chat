import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ChatMessage from './ChatMessage';

afterEach(() => {
  cleanup();
});

describe('ChatMessage generation trace', () => {
  it('renders compact summary by default and expands details on click', async () => {
    const user = userEvent.setup();

    render(
      <ChatMessage
        message={{
          id: 'assistant-1',
          role: 'assistant',
          content: 'Answer text',
          reasoningContent: 'Reasoning paragraph.',
          thinkingTrace: {
            status: 'done',
            summary: 'Finalizing answer: Ordering citations and sending response',
            entries: [
              {
                phase: 'planning',
                title: 'Planning next step',
                detail: 'Checking what evidence is still missing',
              },
            ],
          },
          citations: [],
        }}
      />,
    );

    expect(screen.getByText('Finalizing answer: Ordering citations and sending response')).toBeInTheDocument();
    expect(screen.queryByText('Planning next step')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /thinking/i }));

    expect(screen.getByText('Planning next step')).toBeInTheDocument();
    expect(screen.queryByText('Reasoning paragraph.')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /model reasoning/i }));

    expect(screen.getByText('Reasoning paragraph.')).toBeInTheDocument();
  });

  it('does not render generation trace when neither trace entries nor reasoning are present', () => {
    render(
      <ChatMessage
        message={{
          id: 'assistant-2',
          role: 'assistant',
          content: 'Answer only',
          citations: [],
        }}
      />,
    );

    expect(screen.queryByRole('button', { name: /thinking/i })).not.toBeInTheDocument();
  });
});

describe('ChatMessage user edit actions', () => {
  it('renders inline edit controls and emits callbacks', async () => {
    const user = userEvent.setup();
    const onStartEdit = vi.fn();
    const onEditDraftChange = vi.fn();
    const onSaveEdit = vi.fn();
    const onCancelEdit = vi.fn();

    const { rerender } = render(
      <ChatMessage
        message={{
          id: 'user-1',
          role: 'user',
          content: 'Original text',
          citations: [],
        }}
        onStartEdit={onStartEdit}
      />,
    );

    await user.click(screen.getByRole('button', { name: /edit message/i }));
    expect(onStartEdit).toHaveBeenCalledWith('user-1', 'Original text');

    rerender(
      <ChatMessage
        message={{
          id: 'user-1',
          role: 'user',
          content: 'Original text',
          citations: [],
        }}
        isEditing
        editDraft="Edited text"
        onStartEdit={onStartEdit}
        onEditDraftChange={onEditDraftChange}
        onSaveEdit={onSaveEdit}
        onCancelEdit={onCancelEdit}
      />,
    );

    expect(screen.getByRole('button', { name: /editing message/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /copy message/i })).toBeDisabled();
    expect(screen.getByRole('textbox', { name: /edit message/i })).toHaveValue('Edited text');

    await user.type(screen.getByRole('textbox', { name: /edit message/i }), '!');
    expect(onEditDraftChange).toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: /save & resend/i }));
    expect(onSaveEdit).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: /cancel/i }));
    expect(onCancelEdit).toHaveBeenCalledTimes(1);
  });
});
