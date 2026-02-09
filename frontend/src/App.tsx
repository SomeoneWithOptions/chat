import { FormEvent, useEffect, useMemo, useState } from 'react';
import { authWithGoogle, getMe, listModels, logout, streamMessage, type Model, type User } from './lib/api';

type Message = {
  id: string;
  role: 'user' | 'assistant';
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
  const [grounding, setGrounding] = useState(true);
  const [deepResearch, setDeepResearch] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
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

  useEffect(() => {
    if (!user) {
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

    try {
      await streamMessage(
        {
          message: userMessage.content,
          modelId: selectedModel,
          grounding,
          deepResearch,
        },
        (eventData) => {
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

      <section className="card messages">
        {messages.length === 0 ? <p className="empty">No messages yet. Send one to start streaming.</p> : null}
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
