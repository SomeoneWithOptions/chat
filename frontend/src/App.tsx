import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import {
  APIError,
  authWithGoogle,
  createConversation,
  deleteAllConversations,
  deleteConversation,
  getMe,
  listConversationMessages,
  listConversations,
  listModels,
  logout,
  streamMessage,
  updateModelFavorite,
  updateModelPreference,
  type Conversation,
  type Model,
  type ModelPreferences,
  type User,
} from './lib/api';

type Message = {
  id: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
};

function formatPrice(micros: number): string {
  if (micros <= 0) return '$0';
  return `$${(micros / 1_000_000).toFixed(6)}`;
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [models, setModels] = useState<Model[]>([]);
  const [curatedModels, setCuratedModels] = useState<Model[]>([]);
  const [favoriteModelIds, setFavoriteModelIds] = useState<string[]>([]);
  const [modelPreferences, setModelPreferences] = useState<ModelPreferences>({
    lastUsedModelId: 'openrouter/free',
    lastUsedDeepResearchModelId: 'openrouter/free',
  });
  const [showAllModels, setShowAllModels] = useState(false);
  const [selectedModel, setSelectedModel] = useState('openrouter/free');
  const [prompt, setPrompt] = useState('');
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [conversationAPISupported, setConversationAPISupported] = useState(true);
  const [grounding, setGrounding] = useState(true);
  const [deepResearch, setDeepResearch] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const [deletingConversationId, setDeletingConversationId] = useState<string | null>(null);
  const [isDeletingAll, setIsDeletingAll] = useState(false);
  const [loadingConversations, setLoadingConversations] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [loadingAuth, setLoadingAuth] = useState(true);
  const [updatingModelPreference, setUpdatingModelPreference] = useState(false);
  const [updatingModelFavorite, setUpdatingModelFavorite] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [devEmail, setDevEmail] = useState('acastesol@gmail.com');
  const [devSub, setDevSub] = useState('dev-user-1');

  function isNotFoundError(err: unknown): err is APIError {
    return err instanceof APIError && err.status === 404;
  }

  function isConversationNotFoundError(err: unknown): err is APIError {
    return err instanceof APIError && err.status === 404 && err.code === 'conversation_not_found';
  }

  useEffect(() => {
    void (async () => {
      try {
        const currentUser = await getMe();
        setUser(currentUser);
      } catch {
        setUser(null);
      } finally {
        setLoadingAuth(false);
      }
    })();
  }, []);

  const refreshConversations = useCallback(async (preferredConversationId?: string) => {
    setLoadingConversations(true);
    try {
      const availableConversations = await listConversations();
      setConversationAPISupported(true);
      setConversations(availableConversations);
      setActiveConversationId((current) => {
        if (preferredConversationId && availableConversations.some((conversation) => conversation.id === preferredConversationId)) {
          return preferredConversationId;
        }
        if (current && availableConversations.some((conversation) => conversation.id === current)) {
          return current;
        }
        return availableConversations[0]?.id ?? null;
      });
      if (availableConversations.length === 0) {
        setMessages([]);
      }
    } catch (err) {
      if (isNotFoundError(err)) {
        setConversationAPISupported(false);
        setConversations([]);
        setActiveConversationId(null);
        setMessages([]);
        return;
      }
      setError((err as Error).message);
    } finally {
      setLoadingConversations(false);
    }
  }, []);

  useEffect(() => {
    if (!user) {
      setModels([]);
      setCuratedModels([]);
      setFavoriteModelIds([]);
      setModelPreferences({
        lastUsedModelId: 'openrouter/free',
        lastUsedDeepResearchModelId: 'openrouter/free',
      });
      setShowAllModels(false);
      setConversations([]);
      setActiveConversationId(null);
      setConversationAPISupported(true);
      setMessages([]);
      return;
    }

    void (async () => {
      try {
        const catalog = await listModels();
        setModels(catalog.models);
        setCuratedModels(catalog.curatedModels);
        setFavoriteModelIds(catalog.favorites);
        setModelPreferences(catalog.preferences);
        setShowAllModels(catalog.curatedModels.length === 0);

        if (catalog.models.length > 0) {
          const preferredModelId = catalog.preferences.lastUsedModelId || catalog.models[0].id;
          setSelectedModel(catalog.models.some((model) => model.id === preferredModelId) ? preferredModelId : catalog.models[0].id);
        }
      } catch (err) {
        setError((err as Error).message);
      }
    })();
  }, [user]);

  useEffect(() => {
    if (!user) {
      return;
    }
    if (!conversationAPISupported) {
      return;
    }
    void refreshConversations();
  }, [conversationAPISupported, refreshConversations, user]);

  useEffect(() => {
    if (!user) {
      return;
    }
    if (!conversationAPISupported) {
      return;
    }
    if (!activeConversationId) {
      setMessages([]);
      return;
    }
    if (isStreaming) {
      return;
    }

    let cancelled = false;
    setLoadingMessages(true);
    void (async () => {
      try {
        const conversationMessages = await listConversationMessages(activeConversationId);
        if (cancelled) {
          return;
        }
        setMessages(
          conversationMessages.map((message) => ({
            id: message.id,
            role: message.role,
            content: message.content,
          })),
        );
      } catch (err) {
        if (isConversationNotFoundError(err)) {
          await refreshConversations();
          return;
        }
        if (isNotFoundError(err)) {
          setConversationAPISupported(false);
          setConversations([]);
          setActiveConversationId(null);
          return;
        }
        if (!cancelled) {
          setError((err as Error).message);
        }
      } finally {
        if (!cancelled) {
          setLoadingMessages(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [activeConversationId, conversationAPISupported, isStreaming, user]);

  const favoriteModelIdSet = useMemo(() => new Set(favoriteModelIds), [favoriteModelIds]);

  const visibleModels = useMemo(() => {
    const base = showAllModels || curatedModels.length === 0 ? models : curatedModels;
    return [...base].sort((a, b) => {
      const aFavorite = favoriteModelIdSet.has(a.id) ? 1 : 0;
      const bFavorite = favoriteModelIdSet.has(b.id) ? 1 : 0;
      if (aFavorite !== bFavorite) {
        return bFavorite - aFavorite;
      }
      return a.name.localeCompare(b.name);
    });
  }, [curatedModels, favoriteModelIdSet, models, showAllModels]);

  const selectableModels = useMemo(() => {
    const byID = new Map<string, Model>();
    for (const model of visibleModels) {
      byID.set(model.id, model);
    }
    const selected = models.find((model) => model.id === selectedModel);
    if (selected) {
      byID.set(selected.id, selected);
    }
    return [...byID.values()];
  }, [models, selectedModel, visibleModels]);

  const currentModel = useMemo(
    () => models.find((model) => model.id === selectedModel) ?? null,
    [models, selectedModel],
  );

  const selectedModelIsFavorite = favoriteModelIdSet.has(selectedModel);

  async function handleDevLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    try {
      const authenticatedUser = await authWithGoogle('dev-token', {
        'X-Test-Email': devEmail,
        'X-Test-Google-Sub': devSub,
        'X-Test-Name': devEmail.split('@')[0],
      });
      setUser(authenticatedUser);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function handleLogout() {
    setError(null);
    try {
      await logout();
      setUser(null);
      setModels([]);
      setCuratedModels([]);
      setFavoriteModelIds([]);
      setModelPreferences({
        lastUsedModelId: 'openrouter/free',
        lastUsedDeepResearchModelId: 'openrouter/free',
      });
      setShowAllModels(false);
      setSelectedModel('openrouter/free');
      setMessages([]);
      setConversations([]);
      setActiveConversationId(null);
      setConversationAPISupported(true);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function persistModelSelection(mode: 'chat' | 'deep_research', modelId: string) {
    if (!user) {
      return;
    }
    setUpdatingModelPreference(true);
    try {
      const preferences = await updateModelPreference(mode, modelId);
      setModelPreferences(preferences);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUpdatingModelPreference(false);
    }
  }

  function handleModelChange(nextModelId: string) {
    setSelectedModel(nextModelId);
    setError(null);
    void persistModelSelection(deepResearch ? 'deep_research' : 'chat', nextModelId);
  }

  function handleDeepResearchChange(next: boolean) {
    setDeepResearch(next);
    setError(null);

    const preferredModelID = next ? modelPreferences.lastUsedDeepResearchModelId : modelPreferences.lastUsedModelId;
    const resolvedModelID =
      preferredModelID && models.some((model) => model.id === preferredModelID) ? preferredModelID : selectedModel;

    if (resolvedModelID !== selectedModel && resolvedModelID) {
      setSelectedModel(resolvedModelID);
    }
    if (resolvedModelID) {
      void persistModelSelection(next ? 'deep_research' : 'chat', resolvedModelID);
    }
  }

  async function handleToggleFavorite() {
    if (!selectedModel || !user) {
      return;
    }

    setError(null);
    setUpdatingModelFavorite(true);
    try {
      const favorites = await updateModelFavorite(selectedModel, !selectedModelIsFavorite);
      setFavoriteModelIds(favorites);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUpdatingModelFavorite(false);
    }
  }

  async function handleNewConversation() {
    if (isStreaming || !conversationAPISupported || deletingConversationId !== null || isDeletingAll) {
      return;
    }
    setError(null);
    setMessages([]);

    try {
      const conversation = await createConversation();
      setConversations((existing) => [conversation, ...existing.filter((item) => item.id !== conversation.id)]);
      setActiveConversationId(conversation.id);
    } catch (err) {
      if (isNotFoundError(err)) {
        setConversationAPISupported(false);
        return;
      }
      setError((err as Error).message);
    }
  }

  async function handleDeleteConversation(conversationId: string) {
    if (isStreaming || deletingConversationId !== null || isDeletingAll || !conversationAPISupported) {
      return;
    }

    const conversation = conversations.find((item) => item.id === conversationId);
    const confirmed = window.confirm(`Delete "${conversation?.title ?? 'this chat'}"? This cannot be undone.`);
    if (!confirmed) {
      return;
    }

    setError(null);
    setDeletingConversationId(conversationId);

    if (activeConversationId === conversationId) {
      setMessages([]);
    }

    try {
      await deleteConversation(conversationId);
      await refreshConversations();
    } catch (err) {
      if (isConversationNotFoundError(err)) {
        await refreshConversations();
        return;
      }
      if (isNotFoundError(err)) {
        setConversationAPISupported(false);
        setConversations([]);
        setActiveConversationId(null);
        setMessages([]);
        return;
      }
      setError((err as Error).message);
    } finally {
      setDeletingConversationId(null);
    }
  }

  async function handleDeleteAllConversations() {
    if (
      isStreaming ||
      deletingConversationId !== null ||
      isDeletingAll ||
      !conversationAPISupported ||
      conversations.length === 0
    ) {
      return;
    }

    const confirmed = window.confirm('Delete all conversations? This cannot be undone.');
    if (!confirmed) {
      return;
    }

    setError(null);
    setIsDeletingAll(true);

    try {
      await deleteAllConversations();
      setConversations([]);
      setActiveConversationId(null);
      setMessages([]);
    } catch (err) {
      if (isNotFoundError(err)) {
        setConversationAPISupported(false);
        setConversations([]);
        setActiveConversationId(null);
        setMessages([]);
        return;
      }
      setError((err as Error).message);
    } finally {
      setIsDeletingAll(false);
    }
  }

  async function handleSend(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!prompt.trim() || isStreaming) {
      return;
    }

    const userMessage: Message = {
      id: crypto.randomUUID(),
      role: 'user',
      content: prompt.trim(),
    };

    const assistantMessage: Message = {
      id: crypto.randomUUID(),
      role: 'assistant',
      content: '',
    };

    setMessages((existing) => [...existing, userMessage, assistantMessage]);
    setPrompt('');
    setIsStreaming(true);
    setError(null);

    let resolvedConversationID = activeConversationId;

    try {
      await streamMessage(
        {
          conversationId: activeConversationId ?? undefined,
          message: userMessage.content,
          modelId: selectedModel,
          grounding,
          deepResearch,
        },
        (eventData) => {
          if (eventData.type === 'metadata') {
            if (eventData.conversationId) {
              resolvedConversationID = eventData.conversationId;
              setActiveConversationId(eventData.conversationId);
            }
            return;
          }

          if (eventData.type === 'token') {
            setMessages((existing) =>
              existing.map((message) =>
                message.id === assistantMessage.id
                  ? { ...message, content: `${message.content}${eventData.delta}` }
                  : message,
              ),
            );
          }
        },
      );
      if (resolvedConversationID && conversationAPISupported) {
        await refreshConversations(resolvedConversationID);
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setIsStreaming(false);
    }
  }

  if (loadingAuth) {
    return <main className="screen center">Loading session...</main>;
  }

  if (!user) {
    return (
      <main className="screen center">
        <section className="card auth-card">
          <p className="eyebrow">SANETO CHAT</p>
          <h1>Sign In Required</h1>
          <p>
            Production login uses Google Identity Services. For local development, enable{' '}
            <code>AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true</code> on the backend and use this form.
          </p>

          <form onSubmit={handleDevLogin} className="auth-form">
            <label>
              Email
              <input value={devEmail} onChange={(event) => setDevEmail(event.target.value)} type="email" required />
            </label>
            <label>
              Google Subject
              <input value={devSub} onChange={(event) => setDevSub(event.target.value)} required />
            </label>
            <button type="submit">Dev Sign In</button>
          </form>

          {error ? <p className="error">{error}</p> : null}
        </section>
      </main>
    );
  }

  return (
    <main className="screen app-shell">
      <header className="topbar card">
        <div>
          <p className="eyebrow">Signed In</p>
          <strong>{user.email}</strong>
        </div>

        <div className="controls">
          <label className="model-select">
            Model
            <select value={selectedModel} onChange={(event) => handleModelChange(event.target.value)}>
              {selectableModels.map((model) => (
                <option key={model.id} value={model.id}>
                  {favoriteModelIdSet.has(model.id) ? '★ ' : ''}{model.name} ({model.id})
                </option>
              ))}
            </select>
          </label>

          <label className="checkbox">
            <input
              type="checkbox"
              checked={showAllModels}
              onChange={(event) => setShowAllModels(event.target.checked)}
            />
            Show All Models
          </label>

          <button
            type="button"
            onClick={() => void handleToggleFavorite()}
            disabled={updatingModelFavorite || models.length === 0}
          >
            {updatingModelFavorite
              ? 'Updating...'
              : selectedModelIsFavorite
                ? 'Unfavorite Model'
                : 'Favorite Model'}
          </button>

          <label className="checkbox">
            <input type="checkbox" checked={grounding} onChange={(event) => setGrounding(event.target.checked)} />
            Grounding
          </label>

          <label className="checkbox">
            <input
              type="checkbox"
              checked={deepResearch}
              onChange={(event) => handleDeepResearchChange(event.target.checked)}
            />
            Deep Research
          </label>

          <button type="button" onClick={handleLogout}>
            Sign Out
          </button>
        </div>
      </header>

      {currentModel ? (
        <section className="card model-meta">
          <p>
            <strong>{currentModel.name}</strong> • Context window: {currentModel.contextWindow.toLocaleString()} • Prompt:{' '}
            {formatPrice(currentModel.promptPriceMicrosUsd)} / token • Completion:{' '}
            {formatPrice(currentModel.outputPriceMicrosUsd)} / token
          </p>
          <p>
            {showAllModels || curatedModels.length === 0
              ? `Showing ${visibleModels.length.toLocaleString()} model${visibleModels.length === 1 ? '' : 's'}`
              : `Showing ${visibleModels.length.toLocaleString()} curated model${visibleModels.length === 1 ? '' : 's'}`}
            {' • '}
            Favorites: {favoriteModelIds.length.toLocaleString()}
            {updatingModelPreference ? ' • Saving preference...' : ''}
          </p>
        </section>
      ) : null}

      {conversationAPISupported ? (
        <section className="card conversations">
          <div className="conversations-header">
            <p className="eyebrow">Conversations</p>
            <div className="conversation-actions">
              <button
                type="button"
                onClick={handleDeleteAllConversations}
                disabled={isStreaming || deletingConversationId !== null || isDeletingAll || conversations.length === 0}
              >
                {isDeletingAll ? 'Deleting...' : 'Delete All'}
              </button>
              <button
                type="button"
                onClick={handleNewConversation}
                disabled={isStreaming || deletingConversationId !== null || isDeletingAll}
              >
                New Chat
              </button>
            </div>
          </div>
          {loadingConversations ? <p className="empty">Loading conversations...</p> : null}
          {!loadingConversations && conversations.length === 0 ? (
            <p className="empty">No saved conversations yet.</p>
          ) : null}
          {!loadingConversations && conversations.length > 0 ? (
            <div className="conversation-list">
              {conversations.map((conversation) => (
                <div key={conversation.id} className="conversation-row">
                  <button
                    type="button"
                    className={`conversation-item ${activeConversationId === conversation.id ? 'active' : ''}`}
                    onClick={() => setActiveConversationId(conversation.id)}
                    disabled={isStreaming || deletingConversationId !== null || isDeletingAll}
                  >
                    {conversation.title}
                  </button>
                  <button
                    type="button"
                    className="conversation-delete"
                    onClick={() => void handleDeleteConversation(conversation.id)}
                    disabled={isStreaming || deletingConversationId !== null || isDeletingAll}
                  >
                    {deletingConversationId === conversation.id ? 'Deleting...' : 'Delete'}
                  </button>
                </div>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}

      <section className="card messages">
        {loadingMessages ? <p className="empty">Loading messages...</p> : null}
        {!loadingMessages && messages.length === 0 ? <p className="empty">No messages yet. Send one to start streaming.</p> : null}
        {messages.map((message) => (
          <article key={message.id} className={`bubble ${message.role}`}>
            <p className="role">{message.role}</p>
            <p>{message.content || (message.role === 'assistant' ? '...' : '')}</p>
          </article>
        ))}
      </section>

      <form className="card composer" onSubmit={handleSend}>
        <textarea
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          rows={4}
          placeholder="Ask something..."
        />
        <div className="composer-row">
          {error ? <p className="error">{error}</p> : <span />}
          <button type="submit" disabled={isStreaming || !prompt.trim()}>
            {isStreaming ? 'Streaming...' : 'Send'}
          </button>
        </div>
      </form>
    </main>
  );
}
