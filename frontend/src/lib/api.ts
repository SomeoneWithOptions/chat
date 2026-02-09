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
  curated: boolean;
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
  modelId?: string | null;
  groundingEnabled: boolean;
  deepResearchEnabled: boolean;
  createdAt: string;
};

export type ChatRequest = {
  conversationId?: string;
  message: string;
  modelId: string;
  grounding: boolean;
  deepResearch: boolean;
};

export type StreamEvent =
  | { type: 'metadata'; grounding: boolean; deepResearch: boolean; modelId: string; conversationId?: string }
  | { type: 'token'; delta: string }
  | { type: 'done' };

const apiBase = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

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
    const message = body?.error?.message ?? `Request failed with status ${response.status}`;
    throw new Error(message);
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

export async function listModels(): Promise<Model[]> {
  const response = await requestJSON<{ models: Model[] }>('/v1/models', {
    method: 'GET',
  });
  return response.models;
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

export async function streamMessage(
  request: ChatRequest,
  onEvent: (event: StreamEvent) => void,
): Promise<void> {
  const response = await fetch(`${apiBase}/v1/chat/messages`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    const message = body?.error?.message ?? `Request failed with status ${response.status}`;
    throw new Error(message);
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
        const event = JSON.parse(jsonPayload) as StreamEvent;
        onEvent(event);
      }
    }
  }
}
