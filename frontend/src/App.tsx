import { ChangeEvent, FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  uploadFile,
  streamMessage,
  updateModelFavorite,
  updateModelPreference,
  type Conversation,
  type Model,
  type ModelPreferences,
  type ResearchPhase,
  type UploadedFile,
  type User,
} from './lib/api';
import Sidebar from './components/Sidebar';
import Toggle from './components/Toggle';
import ModelSelector from './components/ModelSelector';
import ChatMessage, { type MessageData } from './components/ChatMessage';
import Composer from './components/Composer';

function formatPrice(micros: number): string {
  if (micros <= 0) return 'Free';
  const dollars = micros / 1_000_000;
  if (dollars < 0.001) return `$${dollars.toFixed(6)}`;
  return `$${dollars.toFixed(4)}`;
}

type ResearchActivity = {
  phase: ResearchPhase;
  message: string;
  pass?: number;
  totalPasses?: number;
};

const researchPhases: ResearchPhase[] = ['planning', 'searching', 'synthesizing', 'finalizing'];

const researchPhaseLabels: Record<ResearchPhase, string> = {
  planning: 'Planning',
  searching: 'Searching',
  synthesizing: 'Synthesizing',
  finalizing: 'Finalizing',
};

const researchPhaseDefaults: Record<ResearchPhase, string> = {
  planning: 'Preparing research strategy',
  searching: 'Collecting web evidence',
  synthesizing: 'Drafting structured response',
  finalizing: 'Ordering citations and finishing output',
};

export default function App() {
  // ─── Auth State ───────────────────────────────
  const [user, setUser] = useState<User | null>(null);
  const [loadingAuth, setLoadingAuth] = useState(true);
  const [devEmail, setDevEmail] = useState('acastesol@gmail.com');
  const [devSub, setDevSub] = useState('dev-user-1');

  // ─── Model State ──────────────────────────────
  const [models, setModels] = useState<Model[]>([]);
  const [curatedModels, setCuratedModels] = useState<Model[]>([]);
  const [favoriteModelIds, setFavoriteModelIds] = useState<string[]>([]);
  const [modelPreferences, setModelPreferences] = useState<ModelPreferences>({
    lastUsedModelId: 'openrouter/free',
    lastUsedDeepResearchModelId: 'openrouter/free',
  });
  const [showAllModels, setShowAllModels] = useState(false);
  const [selectedModel, setSelectedModel] = useState('openrouter/free');
  const [updatingModelPreference, setUpdatingModelPreference] = useState(false);

  // ─── Chat State ───────────────────────────────
  const [prompt, setPrompt] = useState('');
  const [messages, setMessages] = useState<MessageData[]>([]);
  const [grounding, setGrounding] = useState(true);
  const [deepResearch, setDeepResearch] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamWarning, setStreamWarning] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [researchActivity, setResearchActivity] = useState<ResearchActivity[]>([]);
  const [researchCompleted, setResearchCompleted] = useState(false);

  // ─── Conversation State ───────────────────────
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [conversationAPISupported, setConversationAPISupported] = useState(true);
  const [deletingConversationId, setDeletingConversationId] = useState<string | null>(null);
  const [isDeletingAll, setIsDeletingAll] = useState(false);
  const [loadingConversations, setLoadingConversations] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);

  // ─── Attachment State ─────────────────────────
  const [uploadingAttachments, setUploadingAttachments] = useState(false);
  const [pendingAttachments, setPendingAttachments] = useState<UploadedFile[]>([]);

  // ─── UI State ─────────────────────────────────
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => window.innerWidth < 768);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const streamAbortControllerRef = useRef<AbortController | null>(null);

  // ─── Helpers ──────────────────────────────────

  function isNotFoundError(err: unknown): err is APIError {
    return err instanceof APIError && err.status === 404;
  }

  function isConversationNotFoundError(err: unknown): err is APIError {
    return err instanceof APIError && err.status === 404 && err.code === 'conversation_not_found';
  }

  function isAbortError(err: unknown): boolean {
    return err instanceof DOMException && err.name === 'AbortError';
  }

  // ─── Auth Effects ─────────────────────────────

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

  // ─── Model Effects ────────────────────────────

  useEffect(() => {
    if (!user) {
      setModels([]);
      setCuratedModels([]);
      setFavoriteModelIds([]);
      setModelPreferences({ lastUsedModelId: 'openrouter/free', lastUsedDeepResearchModelId: 'openrouter/free' });
      setShowAllModels(false);
      setConversations([]);
      setActiveConversationId(null);
      setConversationAPISupported(true);
      setMessages([]);
      setStreamWarning(null);
      setResearchActivity([]);
      setResearchCompleted(false);
      setPendingAttachments([]);
      setUploadingAttachments(false);
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
          setSelectedModel(
            catalog.models.some((m) => m.id === preferredModelId) ? preferredModelId : catalog.models[0].id,
          );
        }
      } catch (err) {
        setError((err as Error).message);
      }
    })();
  }, [user]);

  // ─── Conversation Effects ─────────────────────

  const refreshConversations = useCallback(async (
    preferredConversationId?: string,
    options?: { fallbackToFirstConversation?: boolean },
  ) => {
    setLoadingConversations(true);
    try {
      const availableConversations = await listConversations();
      const fallbackToFirstConversation = options?.fallbackToFirstConversation ?? true;
      setConversationAPISupported(true);
      setConversations(availableConversations);
      setActiveConversationId((current) => {
        if (preferredConversationId && availableConversations.some((c) => c.id === preferredConversationId)) {
          return preferredConversationId;
        }
        if (current && availableConversations.some((c) => c.id === current)) {
          return current;
        }
        return fallbackToFirstConversation ? (availableConversations[0]?.id ?? null) : null;
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
    if (!user || !conversationAPISupported) return;
    void refreshConversations(undefined, { fallbackToFirstConversation: false });
  }, [conversationAPISupported, refreshConversations, user]);

  useEffect(() => {
    if (!user || !conversationAPISupported) return;
    if (!activeConversationId) {
      setMessages([]);
      return;
    }
    if (isStreaming) return;

    let cancelled = false;
    setLoadingMessages(true);
    void (async () => {
      try {
        const conversationMessages = await listConversationMessages(activeConversationId);
        if (cancelled) return;
        setMessages(
          conversationMessages.map((m) => ({
            id: m.id,
            role: m.role,
            content: m.content,
            citations: m.citations ?? [],
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
        if (!cancelled) setError((err as Error).message);
      } finally {
        if (!cancelled) setLoadingMessages(false);
      }
    })();

    return () => { cancelled = true; };
  }, [activeConversationId, conversationAPISupported, isStreaming, user]);

  // Auto-scroll on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // ─── Computed ─────────────────────────────────

  const favoriteModelIdSet = useMemo(() => new Set(favoriteModelIds), [favoriteModelIds]);

  const visibleModels = useMemo(() => {
    const base = showAllModels || curatedModels.length === 0 ? models : curatedModels;
    return [...base].sort((a, b) => {
      const aFav = favoriteModelIdSet.has(a.id) ? 1 : 0;
      const bFav = favoriteModelIdSet.has(b.id) ? 1 : 0;
      if (aFav !== bFav) return bFav - aFav;
      return a.name.localeCompare(b.name);
    });
  }, [curatedModels, favoriteModelIdSet, models, showAllModels]);

  const selectableModels = useMemo(() => {
    const byID = new Map<string, Model>();
    for (const model of visibleModels) byID.set(model.id, model);
    const selected = models.find((m) => m.id === selectedModel);
    if (selected) byID.set(selected.id, selected);
    return [...byID.values()];
  }, [models, selectedModel, visibleModels]);

  const currentModel = useMemo(
    () => models.find((m) => m.id === selectedModel) ?? null,
    [models, selectedModel],
  );

  const latestResearchByPhase = useMemo(() => {
    const byPhase = new Map<ResearchPhase, ResearchActivity>();
    for (const entry of researchActivity) {
      byPhase.set(entry.phase, entry);
    }
    return byPhase;
  }, [researchActivity]);

  const latestResearchPhaseIndex = useMemo(() => {
    let highest = -1;
    for (const entry of researchActivity) {
      const index = researchPhases.indexOf(entry.phase);
      if (index > highest) highest = index;
    }
    return highest;
  }, [researchActivity]);

  const researchStatus = useMemo(() => {
    if (isStreaming && researchActivity.length > 0) return 'Running';
    if (researchCompleted) return 'Complete';
    if (researchActivity.length > 0) return 'Stopped';
    return 'Ready';
  }, [isStreaming, researchActivity.length, researchCompleted]);

  // ─── Handlers ─────────────────────────────────

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
      setModelPreferences({ lastUsedModelId: 'openrouter/free', lastUsedDeepResearchModelId: 'openrouter/free' });
      setShowAllModels(false);
      setSelectedModel('openrouter/free');
      setMessages([]);
      setStreamWarning(null);
      setResearchActivity([]);
      setResearchCompleted(false);
      setConversations([]);
      setActiveConversationId(null);
      setConversationAPISupported(true);
      setPendingAttachments([]);
      setUploadingAttachments(false);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function persistModelSelection(mode: 'chat' | 'deep_research', modelId: string) {
    if (!user) return;
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
      preferredModelID && models.some((m) => m.id === preferredModelID) ? preferredModelID : selectedModel;
    if (resolvedModelID !== selectedModel && resolvedModelID) setSelectedModel(resolvedModelID);
    if (resolvedModelID) void persistModelSelection(next ? 'deep_research' : 'chat', resolvedModelID);
  }

  async function handleToggleFavorite(modelId: string) {
    if (!user) return;
    setError(null);
    try {
      const favorites = await updateModelFavorite(modelId, !favoriteModelIdSet.has(modelId));
      setFavoriteModelIds(favorites);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function handleNewConversation() {
    if (isStreaming || !conversationAPISupported || deletingConversationId !== null || isDeletingAll) return;
    setError(null);
    setMessages([]);
    try {
      const conversation = await createConversation();
      setConversations((existing) => [conversation, ...existing.filter((c) => c.id !== conversation.id)]);
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
    if (isStreaming || deletingConversationId !== null || isDeletingAll || !conversationAPISupported) return;
    const conversation = conversations.find((c) => c.id === conversationId);
    const confirmed = window.confirm(`Delete "${conversation?.title ?? 'this chat'}"? This cannot be undone.`);
    if (!confirmed) return;

    setError(null);
    setDeletingConversationId(conversationId);
    if (activeConversationId === conversationId) setMessages([]);

    try {
      await deleteConversation(conversationId);
      await refreshConversations();
    } catch (err) {
      if (isConversationNotFoundError(err)) { await refreshConversations(); return; }
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
    if (isStreaming || deletingConversationId !== null || isDeletingAll || !conversationAPISupported || conversations.length === 0)
      return;
    const confirmed = window.confirm('Delete all conversations? This cannot be undone.');
    if (!confirmed) return;

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

  async function handleAttachmentChange(event: ChangeEvent<HTMLInputElement>) {
    const files = event.target.files;
    if (!files || files.length === 0) return;
    setError(null);
    setUploadingAttachments(true);
    try {
      const uploaded = await Promise.all(Array.from(files).map((file) => uploadFile(file)));
      setPendingAttachments((existing) => {
        const byID = new Map<string, UploadedFile>();
        for (const a of existing) byID.set(a.id, a);
        for (const a of uploaded) byID.set(a.id, a);
        return [...byID.values()];
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUploadingAttachments(false);
      event.target.value = '';
    }
  }

  function handleRemoveAttachment(fileId: string) {
    setPendingAttachments((existing) => existing.filter((a) => a.id !== fileId));
  }

  function handleStopStreaming() {
    streamAbortControllerRef.current?.abort();
  }

  async function handleSend(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!prompt.trim() || isStreaming || uploadingAttachments) return;

    const isDeepResearchRequest = deepResearch;
    const attachmentIDs = pendingAttachments.map((a) => a.id);
    const userMessage: MessageData = {
      id: crypto.randomUUID(),
      role: 'user',
      content: prompt.trim(),
      citations: [],
    };
    const assistantMessage: MessageData = {
      id: crypto.randomUUID(),
      role: 'assistant',
      content: '',
      citations: [],
    };

    setMessages((existing) => [...existing, userMessage, assistantMessage]);
    setPrompt('');
    setIsStreaming(true);
    setStreamWarning(null);
    setError(null);
    setResearchActivity([]);
    setResearchCompleted(false);

    let resolvedConversationID = activeConversationId;
    const abortController = new AbortController();
    streamAbortControllerRef.current = abortController;

    try {
      await streamMessage(
        {
          conversationId: activeConversationId ?? undefined,
          message: userMessage.content,
          modelId: selectedModel,
          grounding,
          deepResearch,
          fileIds: attachmentIDs.length > 0 ? attachmentIDs : undefined,
        },
        (eventData) => {
          if (eventData.type === 'metadata') {
            if (eventData.conversationId) {
              resolvedConversationID = eventData.conversationId;
              setActiveConversationId(eventData.conversationId);
            }
            return;
          }
          if (eventData.type === 'progress') {
            if (!isDeepResearchRequest) return;
            setResearchActivity((existing) =>
              [
                ...existing,
                {
                  phase: eventData.phase,
                  message: eventData.message ?? researchPhaseDefaults[eventData.phase],
                  pass: eventData.pass,
                  totalPasses: eventData.totalPasses,
                },
              ].slice(-30),
            );
            return;
          }
          if (eventData.type === 'warning') { setStreamWarning(eventData.message); return; }
          if (eventData.type === 'error') { setError(eventData.message); return; }
          if (eventData.type === 'citations') {
            setMessages((existing) =>
              existing.map((m) => m.id === assistantMessage.id ? { ...m, citations: eventData.citations } : m),
            );
            return;
          }
          if (eventData.type === 'done') {
            if (isDeepResearchRequest) setResearchCompleted(true);
            return;
          }
          if (eventData.type === 'token') {
            setMessages((existing) =>
              existing.map((m) =>
                m.id === assistantMessage.id ? { ...m, content: `${m.content}${eventData.delta}` } : m,
              ),
            );
          }
        },
        { signal: abortController.signal },
      );
      if (resolvedConversationID && conversationAPISupported) {
        await refreshConversations(resolvedConversationID);
      }
      setPendingAttachments([]);
    } catch (err) {
      if (isAbortError(err)) {
        setStreamWarning('Response stopped.');
        return;
      }
      setError((err as Error).message);
    } finally {
      if (streamAbortControllerRef.current === abortController) {
        streamAbortControllerRef.current = null;
      }
      setIsStreaming(false);
    }
  }

  // ─── Loading Screen ───────────────────────────

  if (loadingAuth) {
    return (
      <div className="loading-screen">
        <span className="loading-text">Loading...</span>
      </div>
    );
  }

  // ─── Auth Screen ──────────────────────────────

  if (!user) {
    return (
      <div className="auth-screen">
        <div className="auth-card">
          <h1 className="auth-brand">
            <em>Chat</em>
          </h1>
          <p className="auth-subtitle">Sign in to continue</p>

          <form onSubmit={handleDevLogin} className="auth-form">
            <div className="form-field">
              <label className="form-label" htmlFor="dev-email">Email</label>
              <input
                id="dev-email"
                className="form-input"
                value={devEmail}
                onChange={(e) => setDevEmail(e.target.value)}
                type="email"
                required
              />
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="dev-sub">Google Subject</label>
              <input
                id="dev-sub"
                className="form-input"
                value={devSub}
                onChange={(e) => setDevSub(e.target.value)}
                required
              />
            </div>
            <button type="submit" className="btn-primary">
              Sign In
            </button>
          </form>

          <div className="auth-note">
            Production uses Google Identity Services. For local dev, enable{' '}
            <code>AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true</code> on the backend.
          </div>

          {error && <div className="auth-error">{error}</div>}
        </div>
      </div>
    );
  }

  // ─── Main App ─────────────────────────────────

  return (
    <div className="app-layout">
      <Sidebar
        conversations={conversations}
        activeConversationId={activeConversationId}
        onSelectConversation={setActiveConversationId}
        onNewChat={() => void handleNewConversation()}
        onDeleteConversation={(id) => void handleDeleteConversation(id)}
        onDeleteAllConversations={() => void handleDeleteAllConversations()}
        onLogout={() => void handleLogout()}
        userEmail={user.email}
        isStreaming={isStreaming}
        deletingConversationId={deletingConversationId}
        isDeletingAll={isDeletingAll}
        loadingConversations={loadingConversations}
        conversationAPISupported={conversationAPISupported}
        collapsed={sidebarCollapsed}
        onToggleCollapsed={() => setSidebarCollapsed(!sidebarCollapsed)}
      />

      <div className="main-content">
        {/* Header */}
        <header className="header">
          <div className="header-left">
            <button
              className="btn-sidebar-toggle"
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              aria-label="Toggle sidebar"
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <line x1="3" y1="12" x2="21" y2="12" />
                <line x1="3" y1="6" x2="21" y2="6" />
                <line x1="3" y1="18" x2="21" y2="18" />
              </svg>
            </button>

            <ModelSelector
              models={selectableModels}
              selectedModelId={selectedModel}
              onSelectModel={handleModelChange}
              favoriteModelIds={favoriteModelIdSet}
              onToggleFavorite={(id) => void handleToggleFavorite(id)}
              showAllModels={showAllModels}
              onToggleShowAll={setShowAllModels}
              disabled={models.length === 0}
            />
          </div>

          <div className="header-right">
            <Toggle
              checked={grounding}
              onChange={setGrounding}
              label="Grounding"
            />

            <div className="header-divider" />

            <Toggle
              checked={deepResearch}
              onChange={handleDeepResearchChange}
              label="Deep Research"
            />
          </div>
        </header>

        {/* Model info bar */}
        {currentModel && (
          <div className="model-info-bar">
            <span className={`mode-chip ${deepResearch ? 'deep' : 'chat'}`}>
              {deepResearch ? 'Deep Research Mode' : 'Chat Mode'}
            </span>
            <div className="model-info-divider" />
            <div className="model-info-item">
              <span className="model-info-label">Context</span>
              <span className="model-info-value">{currentModel.contextWindow.toLocaleString()}</span>
            </div>
            <div className="model-info-divider" />
            <div className="model-info-item">
              <span className="model-info-label">Prompt</span>
              <span className="model-info-value">{formatPrice(currentModel.promptPriceMicrosUsd)}/tok</span>
            </div>
            <div className="model-info-divider" />
            <div className="model-info-item">
              <span className="model-info-label">Output</span>
              <span className="model-info-value">{formatPrice(currentModel.outputPriceMicrosUsd)}/tok</span>
            </div>
            {updatingModelPreference && (
              <>
                <div className="model-info-divider" />
                <span className="model-info-item" style={{ color: 'var(--accent)', fontStyle: 'italic' }}>
                  Saving...
                </span>
              </>
            )}
          </div>
        )}

        {(deepResearch || researchActivity.length > 0) && (
          <section className="research-panel" data-testid="research-timeline">
            <div className="research-panel-header">
              <span className="research-panel-title">Research Activity</span>
              <span className={`research-panel-status ${isStreaming ? 'running' : researchCompleted ? 'done' : ''}`}>
                {researchStatus}
              </span>
            </div>
            <ol className="research-timeline">
              {researchPhases.map((phase, index) => {
                const latest = latestResearchByPhase.get(phase);
                let state: 'pending' | 'active' | 'done' = 'pending';
                if (index < latestResearchPhaseIndex) state = 'done';
                if (index === latestResearchPhaseIndex) state = researchCompleted ? 'done' : 'active';
                if (!isStreaming && researchCompleted && index <= latestResearchPhaseIndex) state = 'done';

                return (
                  <li key={phase} className={`research-step ${state}`}>
                    <span className="research-step-marker" />
                    <div className="research-step-content">
                      <div className="research-step-row">
                        <span className="research-step-title">{researchPhaseLabels[phase]}</span>
                        {latest?.pass && latest.totalPasses ? (
                          <span className="research-step-pass">
                            {latest.pass}/{latest.totalPasses}
                          </span>
                        ) : null}
                      </div>
                      <p className="research-step-message">{latest?.message ?? researchPhaseDefaults[phase]}</p>
                    </div>
                  </li>
                );
              })}
            </ol>
          </section>
        )}

        {/* Messages */}
        <div className="messages-container">
          {loadingMessages && (
            <div className="messages-empty">
              <span className="loading-text">Loading messages...</span>
            </div>
          )}

          {!loadingMessages && messages.length === 0 && (
            <div className="messages-empty">
              <span className="messages-empty-title">What's on your mind?</span>
              <span className="messages-empty-subtitle">
                Start a conversation. Your messages will appear here.
              </span>
            </div>
          )}

          {messages.map((message, index) => (
            <ChatMessage
              key={message.id}
              message={message}
              isStreaming={isStreaming && index === messages.length - 1}
            />
          ))}
          <div ref={messagesEndRef} />
        </div>

        {/* Composer */}
        <Composer
          prompt={prompt}
          onPromptChange={setPrompt}
          onSend={handleSend}
          onStop={handleStopStreaming}
          isStreaming={isStreaming}
          uploadingAttachments={uploadingAttachments}
          pendingAttachments={pendingAttachments}
          onAttachmentChange={handleAttachmentChange}
          onRemoveAttachment={handleRemoveAttachment}
          error={error}
          streamWarning={streamWarning}
        />
      </div>
    </div>
  );
}
