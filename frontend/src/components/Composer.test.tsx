import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { type FormEvent, useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import Composer from './Composer';

afterEach(() => {
  cleanup();
});

function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    writable: true,
    value: width,
  });
}

function ComposerHarness({ onSend }: { onSend: (event: FormEvent<HTMLFormElement>) => void }) {
  const [prompt, setPrompt] = useState('');

  return (
    <Composer
      prompt={prompt}
      onPromptChange={setPrompt}
      onSend={(event) => {
        event.preventDefault();
        onSend(event);
      }}
      onStop={() => undefined}
      reasoningOptions={[{ value: 'medium', label: 'Medium' }]}
      reasoningEffort="medium"
      supportsReasoning
      reasoningDisabled={false}
      onReasoningEffortChange={() => undefined}
      grounding
      deepResearch={false}
      onToggleGrounding={() => undefined}
      onToggleDeepResearch={() => undefined}
      isStreaming={false}
      uploadingAttachments={false}
      pendingAttachments={[]}
      onAttachmentChange={() => undefined}
      onRemoveAttachment={() => undefined}
      error={null}
      streamWarning={null}
    />
  );
}

describe('Composer keyboard behavior', () => {
  it('submits on desktop when pressing Enter', async () => {
    setViewportWidth(1024);
    const onSend = vi.fn();
    const user = userEvent.setup();

    render(<ComposerHarness onSend={onSend} />);
    const textarea = screen.getByPlaceholderText('Ask anything...');

    await user.type(textarea, 'Desktop prompt');
    await user.keyboard('{Enter}');

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(textarea).toHaveValue('Desktop prompt');
  });

  it('inserts a newline on desktop when pressing Shift+Enter', async () => {
    setViewportWidth(1024);
    const onSend = vi.fn();
    const user = userEvent.setup();

    render(<ComposerHarness onSend={onSend} />);
    const textarea = screen.getByPlaceholderText('Ask anything...');

    await user.type(textarea, 'Desktop multiline');
    await user.keyboard('{Shift>}{Enter}{/Shift}');

    expect(onSend).not.toHaveBeenCalled();
    expect(textarea).toHaveValue('Desktop multiline\n');
  });

  it('inserts a newline on mobile when pressing Enter', async () => {
    setViewportWidth(375);
    const onSend = vi.fn();
    const user = userEvent.setup();

    render(<ComposerHarness onSend={onSend} />);
    const textarea = screen.getByPlaceholderText('Ask anything...');

    await user.type(textarea, 'Mobile prompt');
    await user.keyboard('{Enter}');

    expect(onSend).not.toHaveBeenCalled();
    expect(textarea).toHaveValue('Mobile prompt\n');
  });
});
