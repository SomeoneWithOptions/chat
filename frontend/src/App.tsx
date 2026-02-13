import { ChangeEvent, FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  APIError,
  authWithGoogle,
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
  updateModelReasoningPreset,
  type Conversation,
  type Model,
  type ModelPreferences,
  type ReasoningEffort,
  type ReasoningMode,
  type ReasoningPreset,
  type ResearchPhase,
  type UploadedFile,
  type User,
} from './lib/api';
import Sidebar from './components/Sidebar';
import Toggle from './components/Toggle';
import ModelSelector from './components/ModelSelector';
import ChatMessage, { type MessageData, type ThinkingTrace } from './components/ChatMessage';
import Composer from './components/Composer';
import useVirtualKeyboard from './hooks/useVirtualKeyboard';

const GoogleIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" style={{ display: 'block' }}>
    <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
    <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
    <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
    <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
  </svg>
);

const googleIdentityScriptSrc = 'https://accounts.google.com/gsi/client';

type GoogleCredentialResponse = {
  credential?: string;
};

type GoogleIdentityServices = {
  accounts: {
    id: {
      initialize: (config: {
        client_id: string;
        callback: (response: GoogleCredentialResponse) => void;
      }) => void;
      renderButton: (
        parent: HTMLElement,
        options: {
          theme?: 'outline' | 'filled_blue' | 'filled_black';
          size?: 'large' | 'medium' | 'small';
          text?: 'signin_with' | 'signup_with' | 'continue_with' | 'signin';
          shape?: 'rectangular' | 'pill' | 'circle' | 'square';
          logo_alignment?: 'left' | 'center';
          width?: string;
        },
      ) => void;
    };
  };
};

let googleIdentityScriptLoadPromise: Promise<void> | null = null;

function readGoogleIdentityServices(): GoogleIdentityServices | null {
  const googleValue = (window as Window & { google?: unknown }).google;
  if (!googleValue || typeof googleValue !== 'object') return null;

  const google = googleValue as Partial<GoogleIdentityServices>;
  if (!google.accounts?.id) return null;
  return google as GoogleIdentityServices;
}

function loadGoogleIdentityScript(): Promise<void> {
  if (readGoogleIdentityServices()) {
    return Promise.resolve();
  }

  if (googleIdentityScriptLoadPromise) {
    return googleIdentityScriptLoadPromise;
  }

  googleIdentityScriptLoadPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${googleIdentityScriptSrc}"]`);
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true });
      existing.addEventListener('error', () => reject(new Error('Failed to load Google Identity script')), { once: true });
      return;
    }

    const script = document.createElement('script');
    script.src = googleIdentityScriptSrc;
    script.async = true;
    script.defer = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error('Failed to load Google Identity script'));
    document.head.appendChild(script);
  });

  googleIdentityScriptLoadPromise = googleIdentityScriptLoadPromise.catch((err) => {
    googleIdentityScriptLoadPromise = null;
    throw err;
  });

  return googleIdentityScriptLoadPromise;
}

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

const reasoningEffortOptions: Array<{ value: ReasoningEffort; label: string }> = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
];

const defaultReasoningEffortByMode: Record<ReasoningMode, ReasoningEffort> = {
  chat: 'medium',
  deep_research: 'high',
};

function resolveReasoningEffort(
  presets: ReasoningPreset[],
  modelId: string,
  mode: ReasoningMode,
): ReasoningEffort {
  const existing = presets.find((preset) => preset.modelId === modelId && preset.mode === mode);
  if (existing) return existing.effort;
  return defaultReasoningEffortByMode[mode];
}

function upsertReasoningPreset(
  presets: ReasoningPreset[],
  next: ReasoningPreset,
): ReasoningPreset[] {
  const filtered = presets.filter((preset) => !(preset.modelId === next.modelId && preset.mode === next.mode));
  return [...filtered, next];
}

function buildInitialThinkingTrace(options: { grounding: boolean; deepResearch: boolean }): ThinkingTrace {
  if (options.deepResearch) {
    return {
      status: 'running',
      summary: researchPhaseDefaults.planning,
      steps: researchPhases.map((phase, index) => ({
        id: phase,
        label: researchPhaseLabels[phase],
        detail: researchPhaseDefaults[phase],
        status: index === 0 ? 'active' : 'pending',
      })),
    };
  }

  return {
    status: 'running',
    summary: 'Reviewing your request',
    steps: [
      {
        id: 'understand',
        label: 'Understand request',
        detail: 'Reading your prompt and recent thread context.',
        status: 'active',
      },
      {
        id: 'grounding',
        label: 'Check sources',
        detail: options.grounding
          ? 'Grounding is on, gathering useful web context.'
          : 'Grounding is off for this message.',
        status: options.grounding ? 'pending' : 'done',
      },
      {
        id: 'compose',
        label: 'Compose response',
        detail: 'Preparing a clear final response.',
        status: 'pending',
      },
    ],
  };
}

export default function App() {
  // ─── Auth State ───────────────────────────────
  const googleClientID = (import.meta.env.VITE_GOOGLE_CLIENT_ID ?? '').trim();
  const googleSignInEnabled = googleClientID !== '';
  const [user, setUser] = useState<User | null>(null);
  const [loadingAuth, setLoadingAuth] = useState(true);
  const [signingInWithGoogle, setSigningInWithGoogle] = useState(false);
  const [devEmail, setDevEmail] = useState('acastesol@gmail.com');
  const [devSub, setDevSub] = useState('dev-user-1');
  const googleButtonContainerRef = useRef<HTMLDivElement | null>(null);

  // ─── Model State ──────────────────────────────
  const [models, setModels] = useState<Model[]>([]);
  const [curatedModels, setCuratedModels] = useState<Model[]>([]);
  const [favoriteModelIds, setFavoriteModelIds] = useState<string[]>([]);
  const [modelPreferences, setModelPreferences] = useState<ModelPreferences>({
    lastUsedModelId: 'openrouter/free',
    lastUsedDeepResearchModelId: 'openrouter/free',
  });
  const [reasoningPresets, setReasoningPresets] = useState<ReasoningPreset[]>([]);
  const [showAllModels, setShowAllModels] = useState(false);
  const [selectedModel, setSelectedModel] = useState('openrouter/free');
  const [updatingModelPreference, setUpdatingModelPreference] = useState(false);
  const [updatingReasoningPreset, setUpdatingReasoningPreset] = useState(false);

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
  const [researchPanelExpanded, setResearchPanelExpanded] = useState(false);
  const [thinkingTrace, setThinkingTrace] = useState<ThinkingTrace | null>(null);
  const [activeAssistantMessageId, setActiveAssistantMessageId] = useState<string | null>(null);

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

  const messagesContainerRef = useRef<HTMLDivElement | null>(null);
  const shouldAutoScrollRef = useRef(true);
  const streamAbortControllerRef = useRef<AbortController | null>(null);

  // ─── Virtual keyboard handling (mobile) ─────
  useVirtualKeyboard();

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

  const authenticateWithGoogleToken = useCallback(async (idToken: string) => {
    setSigningInWithGoogle(true);
    setError(null);
    try {
      const authenticatedUser = await authWithGoogle(idToken);
      setUser(authenticatedUser);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSigningInWithGoogle(false);
    }
  }, []);

  useEffect(() => {
    if (!googleSignInEnabled || loadingAuth || user) return;

    let cancelled = false;
    void (async () => {
      try {
        await loadGoogleIdentityScript();
        if (cancelled) return;

        const google = readGoogleIdentityServices();
        const container = googleButtonContainerRef.current;
        if (!google || !container) {
          throw new Error('Google Identity Services is unavailable');
        }

        google.accounts.id.initialize({
          client_id: googleClientID,
          callback: (response) => {
            const credential = response.credential?.trim();
            if (!credential) {
              setError('Google sign-in did not return a credential.');
              return;
            }
            void authenticateWithGoogleToken(credential);
          },
        });

        container.replaceChildren();
        const width = Math.max(240, Math.min(400, container.clientWidth || 360));
        google.accounts.id.renderButton(container, {
          theme: 'filled_black',
          size: 'large',
          text: 'continue_with',
          shape: 'pill',
          logo_alignment: 'left',
          width: `${width}`,
        });
      } catch (err) {
        if (!cancelled) {
          setError((err as Error).message);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [authenticateWithGoogleToken, googleClientID, googleSignInEnabled, loadingAuth, user]);

  // ─── Model Effects ────────────────────────────

  useEffect(() => {
    if (!user) {
      setModels([]);
      setCuratedModels([]);
      setFavoriteModelIds([]);
      setModelPreferences({ lastUsedModelId: 'openrouter/free', lastUsedDeepResearchModelId: 'openrouter/free' });
      setReasoningPresets([]);
      setShowAllModels(false);
      setConversations([]);
      setActiveConversationId(null);
      setConversationAPISupported(true);
      setMessages([]);
      setStreamWarning(null);
      setResearchActivity([]);
      setResearchCompleted(false);
      setThinkingTrace(null);
      setActiveAssistantMessageId(null);
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
        setReasoningPresets(catalog.reasoningPresets ?? []);
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
      if (!isStreaming) {
        setMessages([]);
        setThinkingTrace(null);
        setActiveAssistantMessageId(null);
      }
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
            reasoningContent: m.reasoningContent ?? '',
            citations: m.citations ?? [],
          })),
        );
        setThinkingTrace(null);
        setActiveAssistantMessageId(null);
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

  // Keep auto-scroll enabled only while user is near the bottom.
  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container) return;

    const bottomThresholdPx = 96;
    const updateAutoScroll = () => {
      const distanceFromBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
      shouldAutoScrollRef.current = distanceFromBottom <= bottomThresholdPx;
    };

    updateAutoScroll();
    container.addEventListener('scroll', updateAutoScroll, { passive: true });
    return () => {
      container.removeEventListener('scroll', updateAutoScroll);
    };
  }, [user]);

  // Auto-scroll on new messages when the user is already following the stream.
  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container || !shouldAutoScrollRef.current) return;

    container.scrollTo({
      top: container.scrollHeight,
      behavior: isStreaming ? 'auto' : 'smooth',
    });
  }, [isStreaming, messages]);

  useEffect(() => {
    shouldAutoScrollRef.current = true;
  }, [activeConversationId]);

  // ─── Computed ─────────────────────────────────

  const favoriteModelIdSet = useMemo(() => new Set(favoriteModelIds), [favoriteModelIds]);

  const visibleModels = useMemo(() => {
    let base: Model[];

    if (showAllModels) {
      base = models;
    } else {
      const byID = new Map(models.map((model) => [model.id, model] as const));
      const merged = new Map<string, Model>();

      const include = (model: Model | undefined) => {
        if (!model) return;
        merged.set(model.id, model);
      };

      include(byID.get('openrouter/free'));
      for (const modelId of favoriteModelIds) include(byID.get(modelId));
      for (const model of curatedModels) include(model);

      base = merged.size > 0 ? [...merged.values()] : models;
    }

    return [...base].sort((a, b) => {
      const aFav = favoriteModelIdSet.has(a.id) ? 1 : 0;
      const bFav = favoriteModelIdSet.has(b.id) ? 1 : 0;
      if (aFav !== bFav) return bFav - aFav;
      return a.name.localeCompare(b.name);
    });
  }, [curatedModels, favoriteModelIdSet, favoriteModelIds, models, showAllModels]);

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

  const activeMode: ReasoningMode = deepResearch ? 'deep_research' : 'chat';
  const currentModelSupportsReasoning = currentModel?.supportsReasoning === true;
  const selectedReasoningEffort = useMemo(
    () => resolveReasoningEffort(reasoningPresets, selectedModel, activeMode),
    [activeMode, reasoningPresets, selectedModel],
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
      setReasoningPresets([]);
      setShowAllModels(false);
      setSelectedModel('openrouter/free');
      setMessages([]);
      setStreamWarning(null);
      setResearchActivity([]);
      setResearchCompleted(false);
      setThinkingTrace(null);
      setActiveAssistantMessageId(null);
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

  async function handleReasoningEffortChange(next: ReasoningEffort) {
    if (!user || !currentModelSupportsReasoning) return;
    setError(null);
    setUpdatingReasoningPreset(true);
    setReasoningPresets((existing) =>
      upsertReasoningPreset(existing, { modelId: selectedModel, mode: activeMode, effort: next }),
    );
    try {
      const presets = await updateModelReasoningPreset(selectedModel, activeMode, next);
      setReasoningPresets(presets);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUpdatingReasoningPreset(false);
    }
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
    setSidebarCollapsed(true);
    setActiveConversationId(null);
    setMessages([]);
    setThinkingTrace(null);
    setActiveAssistantMessageId(null);
  }

  async function handleDeleteConversation(conversationId: string) {
    if (isStreaming || deletingConversationId !== null || isDeletingAll || !conversationAPISupported) return;
    const conversation = conversations.find((c) => c.id === conversationId);
    const confirmed = window.confirm(`Delete "${conversation?.title ?? 'this chat'}"? This cannot be undone.`);
    if (!confirmed) return;

    setError(null);
    setDeletingConversationId(conversationId);
    if (activeConversationId === conversationId) {
      setMessages([]);
      setThinkingTrace(null);
      setActiveAssistantMessageId(null);
    }

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
      setThinkingTrace(null);
      setActiveAssistantMessageId(null);
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
      reasoningContent: '',
      citations: [],
    };
    const assistantMessage: MessageData = {
      id: crypto.randomUUID(),
      role: 'assistant',
      content: '',
      reasoningContent: '',
      citations: [],
    };

    setMessages((existing) => [...existing, userMessage, assistantMessage]);
    setPrompt('');
    setIsStreaming(true);
    shouldAutoScrollRef.current = true;
    setStreamWarning(null);
    setError(null);
    setResearchActivity([]);
    setResearchCompleted(false);
    setActiveAssistantMessageId(assistantMessage.id);
    setThinkingTrace(buildInitialThinkingTrace({ grounding, deepResearch: isDeepResearchRequest }));

    let resolvedConversationID = activeConversationId;
    const abortController = new AbortController();
    streamAbortControllerRef.current = abortController;
    let refreshedConversationListForStream = false;

    try {
      await streamMessage(
        {
          conversationId: activeConversationId ?? undefined,
          message: userMessage.content,
          modelId: selectedModel,
          reasoningEffort: currentModelSupportsReasoning ? selectedReasoningEffort : undefined,
          grounding,
          deepResearch,
          fileIds: attachmentIDs.length > 0 ? attachmentIDs : undefined,
        },
        (eventData) => {
          if (eventData.type === 'metadata') {
            if (!isDeepResearchRequest) {
              setThinkingTrace((existing) => {
                if (!existing) return existing;
                return {
                  ...existing,
                  summary: grounding ? 'Gathering grounded context' : 'Drafting response',
                  steps: existing.steps.map((step) => {
                    if (step.id === 'understand') return { ...step, status: 'done' };
                    if (step.id === 'grounding') return { ...step, status: grounding ? 'active' : 'done' };
                    if (step.id === 'compose') return { ...step, status: grounding ? 'pending' : 'active' };
                    return step;
                  }),
                };
              });
            }
            if (eventData.conversationId) {
              resolvedConversationID = eventData.conversationId;
              setActiveConversationId(eventData.conversationId);
              if (!refreshedConversationListForStream && conversationAPISupported) {
                refreshedConversationListForStream = true;
                void refreshConversations(eventData.conversationId, { fallbackToFirstConversation: false });
              }
            }
            return;
          }
          if (eventData.type === 'progress') {
            if (!isDeepResearchRequest) return;
            const currentPhaseIndex = researchPhases.indexOf(eventData.phase);
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
            setThinkingTrace((existing) => {
              const previous = new Map(existing?.steps.map((step) => [step.id, step]));
              return {
                status: 'running',
                summary: eventData.message ?? researchPhaseDefaults[eventData.phase],
                steps: researchPhases.map((phase, index) => ({
                  id: phase,
                  label: researchPhaseLabels[phase],
                  detail:
                    phase === eventData.phase
                      ? eventData.message ?? previous.get(phase)?.detail ?? researchPhaseDefaults[phase]
                      : previous.get(phase)?.detail ?? researchPhaseDefaults[phase],
                  status: index < currentPhaseIndex ? 'done' : index === currentPhaseIndex ? 'active' : 'pending',
                })),
              };
            });
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
            setThinkingTrace((existing) => {
              if (!existing) return existing;
              return {
                ...existing,
                status: 'done',
                summary: 'Thought process complete',
                steps: existing.steps.map((step) => ({ ...step, status: 'done' })),
              };
            });
            return;
          }
          if (eventData.type === 'token') {
            if (!isDeepResearchRequest) {
              setThinkingTrace((existing) => {
                if (!existing) return existing;
                return {
                  ...existing,
                  summary: 'Writing response',
                  steps: existing.steps.map((step) => {
                    if (step.id === 'compose') return { ...step, status: 'active' };
                    return { ...step, status: 'done' };
                  }),
                };
              });
            }
            setMessages((existing) =>
              existing.map((m) =>
                m.id === assistantMessage.id ? { ...m, content: `${m.content}${eventData.delta}` } : m,
              ),
            );
            return;
          }
          if (eventData.type === 'reasoning') {
            setMessages((existing) =>
              existing.map((m) =>
                m.id === assistantMessage.id ? { ...m, reasoningContent: `${m.reasoningContent ?? ''}${eventData.delta}` } : m,
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
        setThinkingTrace((existing) => {
          if (!existing) return existing;
          return { ...existing, status: 'stopped', summary: 'Stopped by user' };
        });
        return;
      }
      setThinkingTrace((existing) => {
        if (!existing) return existing;
        return { ...existing, status: 'stopped', summary: 'Stopped due to an error' };
      });
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

          {googleSignInEnabled ? (
            <div className="google-signin-section">
              <div className="google-auth-wrapper">
                <div className="google-btn-visual">
                  <GoogleIcon />
                  <span>Continue with Google</span>
                </div>
                <div ref={googleButtonContainerRef} className="google-signin-overlay" />
              </div>
              {signingInWithGoogle && <div className="auth-inline-status">Signing in...</div>}
            </div>
          ) : (
            <>
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
            </>
          )}

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
            <div className={`reasoning-control ${currentModelSupportsReasoning ? '' : 'disabled'}`}>
              <span className="reasoning-label">Thinking</span>
              {currentModelSupportsReasoning ? (
                <select
                  className="reasoning-select"
                  value={selectedReasoningEffort}
                  onChange={(event) => void handleReasoningEffortChange(event.target.value as ReasoningEffort)}
                  disabled={isStreaming || updatingReasoningPreset}
                  aria-label="Thinking effort"
                >
                  {reasoningEffortOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="reasoning-unavailable">Unavailable</span>
              )}
            </div>

            <div className="header-divider" />

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
            <div className="model-info-divider" />
            <div className="model-info-item">
              <span className="model-info-label">Thinking</span>
              <span className="model-info-value">
                {currentModelSupportsReasoning ? selectedReasoningEffort : 'N/A'}
              </span>
            </div>
            {(updatingModelPreference || updatingReasoningPreset) && (
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
          <section className={`research-panel ${researchPanelExpanded ? 'expanded' : 'collapsed'}`} data-testid="research-timeline">
            <button
              type="button"
              className="research-panel-header"
              onClick={() => setResearchPanelExpanded((open) => !open)}
              aria-expanded={researchPanelExpanded}
            >
              <span className="research-panel-title">Research Activity</span>
              <span className="research-panel-header-right">
                <span className={`research-panel-status ${isStreaming ? 'running' : researchCompleted ? 'done' : ''}`}>
                  {researchStatus}
                </span>
                <svg
                  className={`research-panel-chevron ${researchPanelExpanded ? 'open' : ''}`}
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <polyline points="6 9 12 15 18 9" />
                </svg>
              </span>
            </button>

            {/* Collapsed: show only the active (or last completed) step */}
            {!researchPanelExpanded && (() => {
              const activeIndex = latestResearchPhaseIndex >= 0 ? latestResearchPhaseIndex : 0;
              const activePhase = researchPhases[activeIndex];
              const latest = latestResearchByPhase.get(activePhase);
              let state: 'pending' | 'active' | 'done' = 'pending';
              if (researchActivity.length > 0) {
                state = researchCompleted ? 'done' : 'active';
              }

              return (
                <div className="research-active-step">
                  <span className={`research-step-marker ${state}`} />
                  <div className="research-step-content">
                    <div className="research-step-row">
                      <span className="research-step-title">{researchPhaseLabels[activePhase]}</span>
                      {latest?.pass && latest.totalPasses ? (
                        <span className="research-step-pass">
                          {latest.pass}/{latest.totalPasses}
                        </span>
                      ) : null}
                    </div>
                    <p className="research-step-message">{latest?.message ?? researchPhaseDefaults[activePhase]}</p>
                  </div>
                </div>
              );
            })()}

            {/* Expanded: show all phases */}
            {researchPanelExpanded && (
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
            )}
          </section>
        )}

        {/* Messages */}
        <div ref={messagesContainerRef} className="messages-container">
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

          {messages.map((message) => {
            const isActiveAssistant = message.id === activeAssistantMessageId;
            return (
              <ChatMessage
                key={message.id}
                message={message}
                isStreaming={isStreaming && isActiveAssistant}
                thinkingTrace={isActiveAssistant ? thinkingTrace : null}
              />
            );
          })}
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
