import { cleanup, render, screen, waitFor } from '@testing-library/react';
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
        curated: true,
      },
    ],
    favorites: [],
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
  it('renders research phases and completion state from progress events', async () => {
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: true, modelId: 'openrouter/free', conversationId: 'conv-1' });
      onEvent({ type: 'progress', phase: 'planning', message: 'Planned 3 research passes', totalPasses: 3 });
      onEvent({ type: 'progress', phase: 'searching', message: 'Searching pass 1 of 3', pass: 1, totalPasses: 3 });
      onEvent({ type: 'progress', phase: 'synthesizing', message: 'Synthesizing evidence' });
      onEvent({ type: 'token', delta: 'Final answer [1].' });
      onEvent({ type: 'progress', phase: 'finalizing', message: 'Finalizing citations' });
      onEvent({ type: 'done' });
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText('Ask anything...');

    await user.click(screen.getAllByRole('switch')[1]);
    await user.type(screen.getByPlaceholderText('Ask anything...'), 'Need deep research output');
    await user.click(screen.getAllByRole('button', { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(await screen.findByTestId('research-timeline')).toBeInTheDocument();
    expect(screen.getByText('Planning')).toBeInTheDocument();
    expect(screen.getByText('Searching')).toBeInTheDocument();
    expect(screen.getByText('Synthesizing')).toBeInTheDocument();
    expect(screen.getByText('Finalizing')).toBeInTheDocument();
    expect(screen.getByText('Complete')).toBeInTheDocument();
  });

  it('ignores progress rendering for non-deep-research sends', async () => {
    streamMessageMock.mockImplementation(async (_request: api.ChatRequest, onEvent: (event: api.StreamEvent) => void) => {
      onEvent({ type: 'metadata', grounding: true, deepResearch: false, modelId: 'openrouter/free', conversationId: 'conv-1' });
      onEvent({ type: 'progress', phase: 'planning', message: 'Should be ignored' });
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

    expect(screen.queryByTestId('research-timeline')).not.toBeInTheDocument();
  });
});
