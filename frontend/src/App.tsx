import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import {
  authWithGoogle,
  createConversation,
  getMe,
  listConversationMessages,
  listConversations,
  listModels,
  logout,
  streamMessage,
  type Conversation,
  type Model,
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
  const [selectedModel, setSelectedModel] = useState('openrouter/free');
  const [prompt, setPrompt] = useState('');
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [grounding, setGrounding] = useState(true);
  const [deepResearch, setDeepResearch] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const [loadingConversations, setLoadingConversations] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [loadingAuth, setLoadingAuth] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [devEmail, setDevEmail] = useState('acastesol@gmail.com');
  const [devSub, setDevSub] = useState('dev-user-1');

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
      setError((err as Error).message);
    } finally {
      setLoadingConversations(false);
    }
  }, []);

  useEffect(() => {
    if (!user) {
      setConversations([]);
      setActiveConversationId(null);
      setMessages([]);
      return;
    }

    void (async () => {
      try {
        const availableModels = await listModels();
        setModels(availableModels);
        if (availableModels.length > 0) {
          setSelectedModel((current) => {
            const found = availableModels.some((model) => model.id === current);
            return found ? current : availableModels[0].id;
          });
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
    void refreshConversations();
  }, [refreshConversations, user]);

  useEffect(() => {
    if (!user) {
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
  }, [activeConversationId, isStreaming, user]);

  const currentModel = useMemo(
    () => models.find((model) => model.id === selectedModel) ?? null,
    [models, selectedModel],
  );

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
      setMessages([]);
      setConversations([]);
      setActiveConversationId(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function handleNewConversation() {
    if (isStreaming) {
      return;
    }
    setError(null);
    setMessages([]);

    try {
      const conversation = await createConversation();
      setConversations((existing) => [conversation, ...existing.filter((item) => item.id !== conversation.id)]);
      setActiveConversationId(conversation.id);
    } catch (err) {
      setError((err as Error).message);
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
      if (resolvedConversationID) {
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
          <label>
            Model
            <select value={selectedModel} onChange={(event) => setSelectedModel(event.target.value)}>
              {models.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.name} ({model.id})
                </option>
              ))}
            </select>
          </label>

          <label className="checkbox">
            <input type="checkbox" checked={grounding} onChange={(event) => setGrounding(event.target.checked)} />
            Grounding
          </label>

          <label className="checkbox">
            <input
              type="checkbox"
              checked={deepResearch}
              onChange={(event) => setDeepResearch(event.target.checked)}
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
        </section>
      ) : null}

      <section className="card conversations">
        <div className="conversations-header">
          <p className="eyebrow">Conversations</p>
          <button type="button" onClick={handleNewConversation} disabled={isStreaming}>
            New Chat
          </button>
        </div>
        {loadingConversations ? <p className="empty">Loading conversations...</p> : null}
        {!loadingConversations && conversations.length === 0 ? (
          <p className="empty">No saved conversations yet.</p>
        ) : null}
        {!loadingConversations && conversations.length > 0 ? (
          <div className="conversation-list">
            {conversations.map((conversation) => (
              <button
                key={conversation.id}
                type="button"
                className={`conversation-item ${activeConversationId === conversation.id ? 'active' : ''}`}
                onClick={() => setActiveConversationId(conversation.id)}
                disabled={isStreaming}
              >
                {conversation.title}
              </button>
            ))}
          </div>
        ) : null}
      </section>

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
