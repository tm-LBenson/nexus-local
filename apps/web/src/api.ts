export type Health = {
  status: string;
  env: string;
  version: string;
};

export type Readiness = {
  status: string;
  persistence_backend: string;
  run_migrations: boolean;
  object_storage_backend: string;
  embedding_backend: string;
  embedding_model: string;
  embedding_dimensions: number;
  embedding_gateway: string;
  embedding_gateway_auth: boolean;
  database_configured: boolean;
  object_store_configured: boolean;
  vector_backend: string;
  vector_collection: string;
  queue_backend: string;
  model_gateway: string;
  model_gateway_auth: boolean;
};

export type ModelTarget = {
  name: string;
  provider: string;
  base_url: string;
  model: string;
};

export type DocumentRegistration = {
  tenant_id: string;
  owner_id: string;
  name: string;
  storage_key: string;
  size_bytes: number;
};

export type RegisteredDocument = {
  id: string;
  tenant_id: string;
  owner_id: string;
  name: string;
  storage_key: string;
  size_bytes: number;
  status: string;
  created_at: string;
  updated_at: string;
};

export type RegisteredJob = {
  id: string;
  tenant_id: string;
  type: string;
  resource_type: string;
  resource_id: string;
  state: string;
  attempts: number;
  created_at: string;
  updated_at: string;
};

export type RegisterDocumentResponse = {
  document: RegisteredDocument;
  job: RegisteredJob;
};

export type SearchHit = {
  document_id: string;
  chunk_id: string;
  text: string;
  score: number;
  metadata: Record<string, string>;
};

export type SearchDocumentsResponse = {
  hits: SearchHit[];
};

export type Conversation = {
  id: string;
  tenant_id: string;
  owner_id: string;
  title: string;
  model_target: string;
  created_at: string;
  updated_at: string;
};

export type ConversationMessage = {
  id: string;
  tenant_id: string;
  conversation_id: string;
  role: string;
  content: string;
  created_at: string;
};

export type AskConversationResponse = {
  conversation: Conversation;
  user_message: ConversationMessage;
  assistant_message: ConversationMessage;
  hits: SearchHit[];
  completion: {
    model: string;
    content: string;
    finish_reason: string;
    usage?: Record<string, number>;
    metadata?: Record<string, string>;
  };
};

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export function apiBase() {
  return apiBaseUrl;
}

export async function getHealth() {
  return request<Health>('/healthz');
}

export async function getReadiness() {
  return request<Readiness>('/readyz');
}

export async function getModelTargets() {
  return request<{ targets: ModelTarget[] }>('/v1/model-targets');
}

export async function registerDocument(input: DocumentRegistration) {
  return request<RegisterDocumentResponse>('/v1/documents/register', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function uploadDocument(input: {
  tenant_id: string;
  owner_id: string;
  file: File;
}) {
  const form = new FormData();
  form.set('tenant_id', input.tenant_id);
  form.set('owner_id', input.owner_id);
  form.set('file', input.file);

  return requestForm<RegisterDocumentResponse>('/v1/documents/upload', {
    method: 'POST',
    body: form,
  });
}

export async function searchDocuments(input: {
  tenant_id: string;
  query: string;
  limit: number;
}) {
  return request<SearchDocumentsResponse>('/v1/search', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function askConversation(input: {
  tenant_id: string;
  owner_id: string;
  conversation_id?: string;
  model_target: string;
  question: string;
  limit: number;
}) {
  return request<AskConversationResponse>('/v1/conversations/ask', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(body || `API returned ${response.status}`);
  }

  return response.json() as Promise<T>;
}

async function requestForm<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, init);

  if (!response.ok) {
    const body = await response.text();
    throw new Error(body || `API returned ${response.status}`);
  }

  return response.json() as Promise<T>;
}
