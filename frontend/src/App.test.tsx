import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import * as api from './lib/api';

const getMeMock = vi.fn();
const authWithGoogleMock = vi.fn();
const logoutMock = vi.fn();
const listModelsMock = vi.fn();
const updateModelPreferenceMock = vi.fn();
const updateModelFavoriteMock = vi.fn();
const updateModelReasoningPresetMock = vi.fn();
const createConversationMock = vi.fn();
const listConversationsMock = vi.fn();
const listConversationMessagesMock = vi.fn();
const deleteConversationMock = vi.fn();
const deleteAllConversationsMock = vi.fn();
const uploadFileMock = vi.fn();
const streamMessageMock = vi.fn();

beforeEach(() => {
  vi.restoreAllMocks();

  getMeMock.mockReset();
  authWithGoogleMock.mockReset();
  logoutMock.mockReset();
  listModelsMock.mockReset();
  updateModelPreferenceMock.mockReset();
  updateModelFavoriteMock.mockReset();
  updateModelReasoningPresetMock.mockReset();
  createConversationMock.mockReset();
  listConversationsMock.mockReset();
  listConversationMessagesMock.mockReset();
  deleteConversationMock.mockReset();
  deleteAllConversationsMock.mockReset();
  uploadFileMock.mockReset();
  streamMessageMock.mockReset();

  getMeMock.mockResolvedValue({ id: 'user-1', email: 'user@example.com', name: 'User' });
  listModelsMock.mockResolvedValue({
    models: [
      {
        id: 'openrouter/free',
        name: 'OpenRouter Free',
        provider: 'openrouter',
        contextWindow: 128000,
        promptPriceMicrosUsd: 0,
        outputPriceMicrosUsd: 0,
        supportsReasoning: true,
        curated: true,
      },
    ],
    curatedModels: [
      {
        id: 'openrouter/free',
        name: 'OpenRouter Free',
        provider: 'openrouter',
        contextWindow: 128000,
        promptPriceMicrosUsd: 0,
        outputPriceMicrosUsd: 0,
        supportsReasoning: true,
        curated: true,
      },
    ],
    favorites: [],
    reasoningPresets: [
      { modelId: 'openrouter/free', mode: 'chat', effort: 'medium' },
      { modelId: 'openrouter/free', mode: 'deep_research', effort: 'high' },
    ],
    preferences: {
      lastUsedModelId: 'openrouter/free',
      lastUsedDeepResearchModelId: 'openrouter/free',
    },
  });
  updateModelPreferenceMock.mockResolvedValue({
    lastUsedModelId: 'openrouter/free',
    lastUsedDeepResearchModelId: 'openrouter/free',
  });
  updateModelFavoriteMock.mockResolvedValue([]);
  updateModelReasoningPresetMock.mockImplementation(async (_modelId: string, mode: api.ReasoningMode, effort: api.ReasoningEffort) => [
    { modelId: 'openrouter/free', mode, effort },
    { modelId: 'openrouter/free', mode: mode === 'chat' ? 'deep_research' : 'chat', effort: mode === 'chat' ? 'high' : 'medium' },
  ]);
  createConversationMock.mockResolvedValue({
    id: 'conv-1',
    title: 'New Chat',
    createdAt: '2026-02-10T00:00:00Z',
    updatedAt: '2026-02-10T00:00:00Z',
  });
  listConversationsMock.mockResolvedValue([]);
  listConversationMessagesMock.mockResolvedValue([]);
  deleteConversationMock.mockResolvedValue(undefined);
  deleteAllConversationsMock.mockResolvedValue(undefined);
  authWithGoogleMock.mockResolvedValue({ id: 'user-1', email: 'user@example.com' });
  logoutMock.mockResolvedValue(undefined);
  uploadFileMock.mockResolvedValue({
    id: 'file-1',
    filename: 'notes.md',
    mediaType: 'text/markdown',
    sizeBytes: 100,
    createdAt: '2026-02-10T00:00:00Z',
  });
  streamMessageMock.mockResolvedValue(undefined);

  vi.spyOn(api, 'getMe').mockImplementation(getMeMock);
  vi.spyOn(api, 'authWithGoogle').mockImplementation(authWithGoogleMock);
  vi.spyOn(api, 'logout').mockImplementation(logoutMock);
  vi.spyOn(api, 'listModels').mockImplementation(listModelsMock);
  vi.spyOn(api, 'updateModelPreference').mockImplementation(updateModelPreferenceMock);
  vi.spyOn(api, 'updateModelFavorite').mockImplementation(updateModelFavoriteMock);
  vi.spyOn(api, 'updateModelReasoningPreset').mockImplementation(updateModelReasoningPresetMock);
  vi.spyOn(api, 'createConversation').mockImplementation(createConversationMock);
  vi.spyOn(api, 'listConversations').mockImplementation(listConversationsMock);
  vi.spyOn(api, 'listConversationMessages').mockImplementation(listConversationMessagesMock);
  vi.spyOn(api, 'deleteConversation').mockImplementation(deleteConversationMock);
  vi.spyOn(api, 'deleteAllConversations').mockImplementation(deleteAllConversationsMock);
  vi.spyOn(api, 'uploadFile').mockImplementation(uploadFileMock);
  vi.spyOn(api, 'streamMessage').mockImplementation(streamMessageMock);
});

afterEach(() => {
  cleanup();
});

describe('Deep research streaming UX', () => {
  it('keeps first message visible and shows thinking UI before conversation metadata arrives', async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async () =>
        await new Promise<void>((resolve) => {
          releaseStream = () => resolve();
        }),
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    await user.click(screen.getByRole('button', { name: /new conversation/i }));
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'First prompt');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText('First prompt')).toBeInTheDocument();
    expect(screen.getAllByText('Thinking').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /thinking/i })).toBeInTheDocument();

    if (typeof releaseStream === 'function') {
      releaseStream();
    }

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^stop$/i })).not.toBeInTheDocument();
    });
  });

  it('adds a conversation only after first send starts streaming', async () => {
    const createdConversation: api.Conversation = {
      id: 'conv-new',
      title: 'New Chat',
      createdAt: '2026-02-10T00:00:00Z',
      updatedAt: '2026-02-10T00:00:00Z',
    };

    let listCalls = 0;
    listConversationsMock.mockImplementation(async () => {
      listCalls += 1;
      if (listCalls === 1) return [];
      return [createdConversation];
    });

    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free', conversationId: 'conv-new' });
      onEvent({ type: 'token', delta: 'Assistant reply' });
      onEvent({ type: 'done' });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByText('No conversations yet');

    await user.click(screen.getByRole('button', { name: /new conversation/i }));

    expect(createConversationMock).not.toHaveBeenCalled();
    await screen.findByText('No conversations yet');

    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Start now');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });
    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      message: 'Start now',
      conversationId: undefined,
    });

    await waitFor(() => {
      expect(screen.getByText('New Chat')).toBeInTheDocument();
    });
  });

  it('collapses sidebar when starting a new conversation', async () => {
    const user = userEvent.setup();
    const { container } = render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    const sidebar = container.querySelector('aside.sidebar');
    expect(sidebar).not.toBeNull();
    expect(sidebar).not.toHaveClass('collapsed');

    await user.click(screen.getByRole('button', { name: /new conversation/i }));

    expect(sidebar).toHaveClass('collapsed');
  });

  it('renders research phases and completion state from progress events', async () => {
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: true, modelId: 'openrouter/free', conversationId: 'conv-1' });
      onEvent({
        type: 'progress',
        phase: 'planning',
        title: 'Planning next step',
        detail: 'Checking what evidence is still missing',
        totalPasses: 3,
        loop: 1,
        maxLoops: 3,
      });
      onEvent({
        type: 'progress',
        phase: 'searching',
        title: 'Searching trusted sources',
        detail: 'Searching trusted sources for corroboration',
        pass: 1,
        totalPasses: 3,
        loop: 1,
        maxLoops: 3,
      });
      onEvent({
        type: 'progress',
        phase: 'reading',
        title: 'Reading selected sources',
        detail: 'Using top-ranked pages to improve accuracy',
        loop: 1,
        maxLoops: 3,
        sourcesConsidered: 4,
        sourcesRead: 2,
      });
      onEvent({
        type: 'progress',
        phase: 'evaluating',
        title: 'Checking evidence quality',
        detail: 'Deciding whether we can answer confidently',
        loop: 1,
        maxLoops: 3,
        sourcesConsidered: 4,
        sourcesRead: 2,
      });
      onEvent({
        type: 'progress',
        phase: 'iterating',
        title: 'Running another pass',
        detail: 'Need one more search to close gaps',
        loop: 1,
        maxLoops: 3,
        sourcesConsidered: 4,
        sourcesRead: 2,
      });
      onEvent({
        type: 'progress',
        phase: 'synthesizing',
        title: 'Drafting response',
        detail: 'Grounding claims to collected sources',
      });
      onEvent({ type: 'token', delta: 'Final answer [1].' });
      onEvent({
        type: 'progress',
        phase: 'finalizing',
        title: 'Finalizing answer',
        detail: 'Ordering citations and sending response',
      });
      onEvent({ type: 'done' });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    await user.click(screen.getByRole('button', { name: /deep research/i }));
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Need deep research output');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    // Panel is visible and collapsed by default — shows only the active step
    const panel = await screen.findByTestId('research-timeline');
    expect(panel).toBeInTheDocument();
    expect(screen.getByText('Finalizing answer')).toBeInTheDocument();
    expect(screen.getByText('Complete')).toBeInTheDocument();

    // Expand the panel to see all phases
    await user.click(screen.getByRole('button', { name: /research activity/i }));

    expect(screen.getByText('Planning next step')).toBeInTheDocument();
    expect(screen.getByText('Searching trusted sources')).toBeInTheDocument();
    expect(screen.getByText('Reading selected sources')).toBeInTheDocument();
    expect(screen.getByText('Checking evidence quality')).toBeInTheDocument();
    expect(screen.getByText('Running another pass')).toBeInTheDocument();
    expect(screen.getByText('Drafting response')).toBeInTheDocument();
    expect(screen.getByText('Finalizing answer')).toBeInTheDocument();
  });

  it('renders usage details after sources when usage events are streamed', async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free' });
      onEvent({ type: 'token', delta: 'Answer with source [1].' });
      onEvent({
        type: 'citations',
        citations: [{ url: 'https://example.com/source', title: 'Example Source' }],
      });
      onEvent({
        type: 'usage',
        usage: {
          promptTokens: 120,
          completionTokens: 48,
          totalTokens: 168,
          costMicrosUsd: 420,
          byokInferenceCostMicrosUsd: 111,
          tokensPerSecond: 24.5,
          modelId: 'openai/gpt-4o-mini',
          providerName: 'OpenAI',
        },
      });
      await new Promise<void>((resolve) => {
        releaseStream = () => {
          onEvent({ type: 'done' });
          resolve();
        };
      });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Show usage');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    const sourcesButton = await screen.findByRole('button', { name: /sources/i });
    const usageButton = await screen.findByRole('button', { name: /usage/i });
    expect((sourcesButton.compareDocumentPosition(usageButton) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0).toBe(true);
    expect(screen.getByText('$0.000531 / 24.50 tok/s')).toBeInTheDocument();

    await user.click(usageButton);
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('openai/gpt-4o-mini')).toBeInTheDocument();
    expect(screen.getByText('Provider')).toBeInTheDocument();
    expect(screen.getByText('OpenAI')).toBeInTheDocument();
    expect(screen.getByText('Input tokens')).toBeInTheDocument();
    expect(screen.getByText('120')).toBeInTheDocument();
    expect(screen.getByText('$0.000420')).toBeInTheDocument();
    expect(screen.getByText('$0.000111')).toBeInTheDocument();
    expect(screen.getByText('24.50 tok/s')).toBeInTheDocument();

    if (releaseStream) releaseStream();
  });

  it('renders compact one-line progress for non-deep-research sends', async () => {
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free', conversationId: 'conv-1' });
      onEvent({ type: 'progress', phase: 'searching', title: 'Getting grounding results', isQuickStep: true });
      onEvent({ type: 'done' });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Normal chat request');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    const panel = await screen.findByTestId('research-timeline');
    expect(panel).toBeInTheDocument();
    expect(screen.getByText('Research Progress')).toBeInTheDocument();
    expect(screen.getByText('Getting grounding results')).toBeInTheDocument();
    expect(panel.querySelector('.research-step-message')).toBeNull();
  });

  it('includes selected reasoning effort in chat requests when model supports reasoning', async () => {
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free', conversationId: 'conv-1' });
      onEvent({ type: 'done' });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');
    await user.selectOptions(screen.getByLabelText('Thinking effort'), 'high');
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Use higher effort');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      reasoningEffort: 'high',
    });
  });

  it('stops auto-scrolling when user scrolls up while streaming', async () => {
    let emitEvent: ((event: api.StreamEvent) => void) | null = null;
    let releaseStream: (() => void) | undefined;

    streamMessageMock.mockImplementation(
      async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
        emitEvent = onEvent;
        onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free' });
        onEvent({ type: 'token', delta: 'hello' });
        await new Promise<void>((resolve) => {
          releaseStream = resolve;
        });
      },
    );

    const user = userEvent.setup();
    const { container } = render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    const messagesContainer = container.querySelector<HTMLDivElement>('.messages-container');
    expect(messagesContainer).not.toBeNull();
    if (!messagesContainer) return;

    Object.defineProperty(messagesContainer, 'scrollHeight', {
      configurable: true,
      value: 2000,
    });
    Object.defineProperty(messagesContainer, 'clientHeight', {
      configurable: true,
      value: 600,
    });
    Object.defineProperty(messagesContainer, 'scrollTop', {
      configurable: true,
      writable: true,
      value: 1400,
    });

    const scrollToSpy = vi.spyOn(messagesContainer, 'scrollTo');

    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Stream test');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(scrollToSpy.mock.calls.length).toBeGreaterThan(0);
    });

    const callsBeforeScrollUp = scrollToSpy.mock.calls.length;

    messagesContainer.scrollTop = 700;
    messagesContainer.dispatchEvent(new Event('scroll'));

    act(() => {
      emitEvent?.({ type: 'token', delta: ' world' });
    });

    await waitFor(() => {
      expect(screen.getByText('hello world')).toBeInTheDocument();
    });

    expect(scrollToSpy.mock.calls.length).toBe(callsBeforeScrollUp);

    if (releaseStream) {
      releaseStream();
    }
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^stop$/i })).not.toBeInTheDocument();
    });
  });
});

describe('Model selector filtering', () => {
  it('shows an always-visible copy icon on user messages and copies the sent text', async () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      configurable: true,
    });

    const createdConversation: api.Conversation = {
      id: 'conv-1',
      title: 'New Chat',
      createdAt: '2026-02-10T00:00:00Z',
      updatedAt: '2026-02-10T00:00:00Z',
    };
    let listCalls = 0;
    listConversationsMock.mockImplementation(async () => {
      listCalls += 1;
      if (listCalls === 1) return [];
      return [createdConversation];
    });

    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free', conversationId: 'conv-1' });
      await new Promise<void>((resolve) => {
        releaseStream = () => {
          onEvent({ type: 'done' });
          resolve();
        };
      });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Copy this user message');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    const copyButton = await screen.findByRole('button', { name: /copy message/i });
    expect(copyButton).toBeInTheDocument();

    await user.click(copyButton);

    expect(copyButton).toHaveAttribute('aria-label', 'Message copied');

    if (releaseStream) releaseStream();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^stop$/i })).not.toBeInTheDocument();
    });
  });

  it('keeps free + favorites visible when All is off and shows the rest when toggled on', async () => {
    listModelsMock.mockResolvedValueOnce({
      models: [
        {
          id: 'openrouter/free',
          name: 'OpenRouter Free',
          provider: 'openrouter',
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          curated: true,
        },
        {
          id: 'openrouter/latest-used',
          name: 'Latest Used Model',
          provider: 'openrouter',
          contextWindow: 128000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: false,
        },
        {
          id: 'openrouter/favorite-model',
          name: 'Favorite Model',
          provider: 'openrouter',
          contextWindow: 200000,
          promptPriceMicrosUsd: 35,
          outputPriceMicrosUsd: 45,
          curated: false,
        },
        {
          id: 'openrouter/other-model',
          name: 'Other Model',
          provider: 'openrouter',
          contextWindow: 64000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: false,
        },
      ],
      curatedModels: [
        {
          id: 'openrouter/free',
          name: 'OpenRouter Free',
          provider: 'openrouter',
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          curated: true,
        },
      ],
      favorites: ['openrouter/favorite-model'],
      preferences: {
        lastUsedModelId: 'openrouter/latest-used',
        lastUsedDeepResearchModelId: 'openrouter/latest-used',
      },
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    await user.click(screen.getByRole('button', { name: 'Latest Used Model' }));

    expect(screen.getByText('OpenRouter Free')).toBeInTheDocument();
    expect(screen.getByText('Favorite Model')).toBeInTheDocument();
    expect(screen.queryByText('Other Model')).not.toBeInTheDocument();

    await user.click(screen.getByRole('switch', { name: /show all models/i }));

    expect(screen.getByText('Other Model')).toBeInTheDocument();
  });

  it('shows context and pricing metadata using compact ctx with input and output pricing', async () => {
    listModelsMock.mockResolvedValueOnce({
      models: [
        {
          id: 'openrouter/million-context',
          name: 'Million Context Model',
          provider: 'openrouter',
          contextWindow: 1_000_000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: true,
        },
        {
          id: 'openrouter/compact-context',
          name: 'Compact Context Model',
          provider: 'openrouter',
          contextWindow: 256_000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: true,
        },
      ],
      curatedModels: [
        {
          id: 'openrouter/million-context',
          name: 'Million Context Model',
          provider: 'openrouter',
          contextWindow: 1_000_000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: true,
        },
        {
          id: 'openrouter/compact-context',
          name: 'Compact Context Model',
          provider: 'openrouter',
          contextWindow: 256_000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: true,
        },
      ],
      favorites: [],
      preferences: {
        lastUsedModelId: 'openrouter/million-context',
        lastUsedDeepResearchModelId: 'openrouter/million-context',
      },
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');
    await user.click(screen.getByRole('button', { name: 'Million Context Model' }));

    expect(
      screen.getByText((content) => content.includes('1M ctx') && content.includes('25.00$ In - 30.00$ Out')),
    ).toBeInTheDocument();
    expect(
      screen.getByText((content) => content.includes('256K ctx') && content.includes('10.00$ In - 12.00$ Out')),
    ).toBeInTheDocument();
  });
});
