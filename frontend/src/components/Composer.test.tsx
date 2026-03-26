import { cleanup, render, screen, waitFor } from '@testing-library/react';
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

function ComposerHarness({
  onSend,
  compactForActiveFusionRun = false,
  fusionMode = false,
}: {
  onSend: (event: FormEvent<HTMLFormElement>) => void;
  compactForActiveFusionRun?: boolean;
  fusionMode?: boolean;
}) {
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
      fusionMode={fusionMode}
      groundingLocked={false}
      onToggleGrounding={() => undefined}
      onToggleDeepResearch={() => undefined}
      onToggleFusionMode={() => undefined}
      isStreaming={false}
      uploadingAttachments={false}
      pendingAttachments={[]}
      onAttachmentChange={() => undefined}
      onRemoveAttachment={() => undefined}
      error={null}
      streamWarning={null}
      onEnhance={() => undefined}
      enhanceDisabled={false}
      compactForActiveFusionRun={compactForActiveFusionRun}
      models={[
        {
          id: 'fusion-model',
          name: 'OpenAI GPT-5.4',
          provider: 'openai',
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          supportsReasoning: true,
          curated: true,
        },
      ]}
      selectedFusionModel="fusion-model"
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

  it('collapses the composer for active mobile fusion runs and allows reopening controls', async () => {
    setViewportWidth(375);
    const onSend = vi.fn();
    const user = userEvent.setup();

    render(
      <ComposerHarness
        onSend={onSend}
        compactForActiveFusionRun
        fusionMode
      />,
    );

    await waitFor(() => {
      expect(screen.queryByPlaceholderText('Ask anything...')).not.toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: 'Show controls' })).toBeInTheDocument();
    expect(screen.getByText('Fusion in progress')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show controls' }));

    expect(screen.getByPlaceholderText('Ask anything...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Hide controls' })).toBeInTheDocument();
  });

  it('keeps mobile fusion controls collapsed after the run ends until the user reopens them', async () => {
    setViewportWidth(375);
    const onSend = vi.fn();

    const { rerender } = render(
      <ComposerHarness
        onSend={onSend}
        compactForActiveFusionRun
        fusionMode
      />,
    );

    await waitFor(() => {
      expect(screen.queryByPlaceholderText('Ask anything...')).not.toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: 'Show controls' })).toBeInTheDocument();

    rerender(
      <ComposerHarness
        onSend={onSend}
        compactForActiveFusionRun={false}
        fusionMode
      />,
    );

    await waitFor(() => {
      expect(screen.queryByPlaceholderText('Ask anything...')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Fusion ready')).toBeInTheDocument();
    expect(screen.getByText('Review the run or reopen controls')).toBeInTheDocument();
  });

  it('keeps the full composer visible on desktop even during an active fusion run', () => {
    setViewportWidth(1024);
    const onSend = vi.fn();

    render(
      <ComposerHarness
        onSend={onSend}
        compactForActiveFusionRun
      />,
    );

    expect(screen.getByPlaceholderText('Ask anything...')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Show controls' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Hide controls' })).not.toBeInTheDocument();
  });
});
