export type User = {
  id: string;
  email: string;
  name?: string;
  avatarUrl?: string;
};

export type Model = {
  id: string;
  name: string;
  provider: string;
  contextWindow: number;
  promptPriceMicrosUsd: number;
  outputPriceMicrosUsd: number;
  supportsReasoning?: boolean;
  curated: boolean;
};

export type ReasoningMode = 'chat' | 'deep_research';
export type ReasoningEffort = 'low' | 'medium' | 'high';

export type ReasoningPreset = {
  modelId: string;
  mode: ReasoningMode;
  effort: ReasoningEffort;
};

export type ModelPreferences = {
  lastUsedModelId: string;
  lastUsedDeepResearchModelId: string;
};

export type ModelCatalog = {
  models: Model[];
  curatedModels: Model[];
  favorites: string[];
  preferences: ModelPreferences;
  reasoningPresets?: ReasoningPreset[];
};

export type Conversation = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
};

export type ConversationMessage = {
  id: string;
  conversationId: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  reasoningContent?: string | null;
  modelId?: string | null;
  usage?: Usage | null;
  groundingEnabled: boolean;
  deepResearchEnabled: boolean;
  citations: Citation[];
  createdAt: string;
};

export type Citation = {
  url: string;
  title?: string;
  snippet?: string;
  sourceProvider?: string;
};

export type Usage = {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  reasoningTokens?: number;
  costMicrosUsd?: number;
  byokInferenceCostMicrosUsd?: number;
  tokensPerSecond?: number;
  modelId?: string;
  providerName?: string;
};

export type UploadedFile = {
  id: string;
  filename: string;
  mediaType: string;
  sizeBytes: number;
  createdAt: string;
};

export type ChatRequest = {
  conversationId?: string;
  message: string;
  modelId: string;
  reasoningEffort?: ReasoningEffort;
  grounding: boolean;
  deepResearch: boolean;
  fileIds?: string[];
};

export type ResearchPhase = 'planning' | 'searching' | 'reading' | 'evaluating' | 'iterating' | 'synthesizing' | 'finalizing';
export type ProgressDecision = 'search_more' | 'finalize' | 'fallback';

export type StreamEvent =
  | { type: 'metadata'; grounding: boolean; deepResearch: boolean; modelId: string; reasoningEffort?: ReasoningEffort; conversationId?: string }
  | {
      type: 'progress';
      phase: ResearchPhase;
      message?: string;
      pass?: number;
      totalPasses?: number;
      loop?: number;
      maxLoops?: number;
      sourcesConsidered?: number;
      sourcesRead?: number;
      title?: string;
      detail?: string;
      isQuickStep?: boolean;
      decision?: ProgressDecision;
    }
  | { type: 'warning'; scope: string; message: string }
  | { type: 'citations'; citations: Citation[] }
  | { type: 'token'; delta: string }
  | { type: 'reasoning'; delta: string }
  | { type: 'usage'; usage: Usage }
  | { type: 'error'; message: string }
  | { type: 'done' };

const apiBase = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

export class APIError extends Error {
  status: number;
  code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.code = code;
  }
}

function toAPIError(status: number, body: ErrorEnvelope | null): APIError {
  const message = body?.error?.message ?? `Request failed with status ${status}`;
  return new APIError(message, status, body?.error?.code);
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    credentials: 'include',
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw toAPIError(response.status, body);
  }

  return (await response.json()) as T;
}

export async function getMe(): Promise<User> {
  const response = await requestJSON<{ user: User }>('/v1/auth/me', {
    method: 'GET',
  });
  return response.user;
}

export async function authWithGoogle(idToken: string, devHeaders?: Record<string, string>): Promise<User> {
  const response = await requestJSON<{ user: User }>('/v1/auth/google', {
    method: 'POST',
    body: JSON.stringify({ idToken }),
    headers: {
      ...(devHeaders ?? {}),
    },
  });
  return response.user;
}

export async function logout(): Promise<void> {
  await requestJSON<{ success: boolean }>('/v1/auth/logout', {
    method: 'POST',
  });
}

export async function listModels(): Promise<ModelCatalog> {
  const response = await requestJSON<ModelCatalog>('/v1/models', {
    method: 'GET',
  });
  return response;
}

export async function updateModelPreference(mode: ReasoningMode, modelId: string): Promise<ModelPreferences> {
  const response = await requestJSON<{ preferences: ModelPreferences }>('/v1/models/preferences', {
    method: 'PUT',
    body: JSON.stringify({ mode, modelId }),
  });
  return response.preferences;
}

export async function updateModelReasoningPreset(
  modelId: string,
  mode: ReasoningMode,
  effort: ReasoningEffort,
): Promise<ReasoningPreset[]> {
  const response = await requestJSON<{ reasoningPresets: ReasoningPreset[] }>('/v1/models/reasoning-presets', {
    method: 'PUT',
    body: JSON.stringify({ modelId, mode, effort }),
  });
  return response.reasoningPresets;
}

export async function updateModelFavorite(modelId: string, favorite: boolean): Promise<string[]> {
  const response = await requestJSON<{ favorites: string[] }>('/v1/models/favorites', {
    method: 'PUT',
    body: JSON.stringify({ modelId, favorite }),
  });
  return response.favorites;
}

export async function createConversation(title?: string): Promise<Conversation> {
  const response = await requestJSON<{ conversation: Conversation }>('/v1/conversations', {
    method: 'POST',
    body: JSON.stringify(title ? { title } : {}),
  });
  return response.conversation;
}

export async function listConversations(): Promise<Conversation[]> {
  const response = await requestJSON<{ conversations: Conversation[] }>('/v1/conversations', {
    method: 'GET',
  });
  return response.conversations;
}

export async function listConversationMessages(conversationId: string): Promise<ConversationMessage[]> {
  const response = await requestJSON<{ messages: ConversationMessage[] }>(`/v1/conversations/${conversationId}/messages`, {
    method: 'GET',
  });
  return response.messages;
}

export async function deleteConversation(conversationId: string): Promise<void> {
  await requestJSON<{ success: boolean }>(`/v1/conversations/${conversationId}`, {
    method: 'DELETE',
  });
}

export async function deleteAllConversations(): Promise<void> {
  await requestJSON<{ success: boolean }>('/v1/conversations', {
    method: 'DELETE',
  });
}

export async function uploadFile(file: File): Promise<UploadedFile> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch(`${apiBase}/v1/files`, {
    method: 'POST',
    credentials: 'include',
    body: formData,
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw toAPIError(response.status, body);
  }

  const payload = (await response.json()) as { file: UploadedFile };
  return payload.file;
}

export async function streamMessage(
  request: ChatRequest,
  onEvent: (event: StreamEvent) => void,
  options?: { signal?: AbortSignal },
): Promise<void> {
  const response = await fetch(`${apiBase}/v1/chat/messages`, {
    method: 'POST',
    credentials: 'include',
    signal: options?.signal,
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw toAPIError(response.status, body);
  }

  if (!response.body) {
    throw new Error('Streaming response body is missing');
  }

  const decoder = new TextDecoder();
  const reader = response.body.getReader();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }

    buffer += decoder.decode(value, { stream: true });

    const chunks = buffer.split('\n\n');
    buffer = chunks.pop() ?? '';

    for (const chunk of chunks) {
      const dataLines = chunk
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line.startsWith('data:'));

      for (const line of dataLines) {
        const jsonPayload = line.slice('data:'.length).trim();
        if (!jsonPayload) {
          continue;
        }
        let event: StreamEvent;
        try {
          event = JSON.parse(jsonPayload) as StreamEvent;
        } catch {
          continue;
        }
        onEvent(event);
      }
    }
  }
}
