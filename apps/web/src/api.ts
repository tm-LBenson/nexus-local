export type Health = {
  status: string;
  env: string;
  version: string;
};

export type Readiness = {
  status: string;
  auth_mode: string;
  deployment_profile: string;
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
  provider_preset: string;
  model_gateway: string;
  model_gateway_auth: boolean;
};

export type ModelTarget = {
  name: string;
  provider: string;
  base_url: string;
  model: string;
};

export type ModelTargetCheckResponse = {
  status: string;
  route: {
    target: string;
    provider: string;
    base_url: string;
    model: string;
  };
  model: string;
  finish_reason: string;
  latency_ms: number;
};

export type CurrentUser = {
  id: string;
  email: string;
  name: string;
};

export type Tenant = {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

export type TenantMembership = {
  tenant: Tenant;
  role: string;
};

export type TenantMember = {
  user: CurrentUser;
  role: string;
};

export type CurrentUserResponse = {
  user: CurrentUser;
  memberships: TenantMembership[];
};

export type CreateTenantResponse = {
  user: CurrentUser;
  tenant: Tenant;
  membership: TenantMembership;
};

export type ListTenantMembersResponse = {
  tenant: Tenant;
  members: TenantMember[];
};

export type AddTenantMemberResponse = {
  tenant: Tenant;
  member: TenantMember;
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

export type DataSource = {
  id: string;
  tenant_id: string;
  owner_id: string;
  type: string;
  name: string;
  root_path: string;
  include_patterns: string[];
  exclude_patterns: string[];
  scan_interval_minutes: number;
  next_scan_at?: string;
  status: string;
  last_scan_at?: string;
  last_scan_imported: number;
  last_scan_skipped: number;
  last_scan_failed: number;
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
  error_message: string;
  result_json?: string;
  created_at: string;
  updated_at: string;
};

export type DataSourceScanEntry = {
  tenant_id: string;
  job_id: string;
  source_id: string;
  path: string;
  outcome: string;
  reason: string;
  message: string;
  document_id?: string;
  size_bytes: number;
  content_hash?: string;
  created_at: string;
};

export type DataSourceScanSummary = {
  total: number;
  imported: number;
  skipped: number;
  failed: number;
  deleted: number;
  latest_job_id?: string;
  latest_at?: string;
  reasons: Record<string, number>;
};

export type DataSourceScanEntryPage = {
  total: number;
  limit: number;
  offset: number;
  outcome?: string;
  has_more: boolean;
};

export type RegisterDocumentResponse = {
  document: RegisteredDocument;
  job: RegisteredJob;
};

export type ListDocumentsResponse = {
  documents: RegisteredDocument[];
};

export type ListDataSourcesResponse = {
  sources: DataSource[];
};

export type DataSourceResponse = {
  source: DataSource;
  deleted_documents?: number;
};

export type DataSourceDetailResponse = {
  source: DataSource;
  jobs: RegisteredJob[];
  scan_entries: DataSourceScanEntry[];
  scan_summary: DataSourceScanSummary;
  scan_entries_page: DataSourceScanEntryPage;
  failed_documents: number;
};

export type DataSourceScanResponse = {
  source: DataSource;
  job: RegisteredJob;
};

export type DataSourceReindexResponse = {
  source: DataSource;
  jobs: RegisteredJob[];
  queued_documents: number;
  skipped_documents: number;
};

export type DocumentDetailResponse = {
  document: RegisteredDocument;
  jobs: RegisteredJob[];
};

export type ListJobsResponse = {
  jobs: RegisteredJob[];
};

export type AuditEvent = {
  id: string;
  tenant_id: string;
  actor_user_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  outcome: string;
  metadata: Record<string, string>;
  created_at: string;
};

export type ListAuditEventsResponse = {
  events: AuditEvent[];
};

export type SearchHit = {
  document_id: string;
  chunk_id: string;
  source: {
    document_id: string;
    document_name: string;
    chunk_id: string;
    chunk_index?: string;
    storage_key?: string;
  };
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

export type AskConversationInput = {
  tenant_id: string;
  conversation_id?: string;
  document_id?: string;
  model_target: string;
  question: string;
  limit: number;
};

export type AskConversationStreamHandlers = {
  signal?: AbortSignal;
  onStatus?: (message: string) => void;
  onDelta?: (content: string) => void;
  onDone?: (response: AskConversationResponse) => void;
};

export type ListConversationsResponse = {
  conversations: Conversation[];
};

export type ListConversationMessagesResponse = {
  messages: ConversationMessage[];
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

export async function checkModelTarget(input: { tenant_id: string; target: string }) {
  return request<ModelTargetCheckResponse>('/v1/model-targets/check', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function getCurrentUser() {
  return request<CurrentUserResponse>('/v1/me');
}

export async function createTenant(input: { name: string }) {
  return request<CreateTenantResponse>('/v1/tenants', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function deleteTenant(tenantId: string) {
  return request<{ tenant: Tenant }>(`/v1/tenants/${encodeURIComponent(tenantId)}`, {
    method: 'DELETE',
  });
}

export async function listTenantMembers(tenantId: string) {
  return request<ListTenantMembersResponse>(
    `/v1/tenants/${encodeURIComponent(tenantId)}/members`,
  );
}

export async function addTenantMember(
  tenantId: string,
  input: { user_id: string; email: string; name?: string; role: string },
) {
  return request<AddTenantMemberResponse>(
    `/v1/tenants/${encodeURIComponent(tenantId)}/members`,
    {
      method: 'POST',
      body: JSON.stringify(input),
    },
  );
}

export async function deleteTenantMember(tenantId: string, userId: string) {
  return request<AddTenantMemberResponse>(
    `/v1/tenants/${encodeURIComponent(tenantId)}/members/${encodeURIComponent(userId)}`,
    { method: 'DELETE' },
  );
}

export async function listDocuments(tenantId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<ListDocumentsResponse>(`/v1/documents?${params.toString()}`);
}

export async function listDataSources(tenantId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<ListDataSourcesResponse>(`/v1/data-sources?${params.toString()}`);
}

export async function createDataSource(input: {
  tenant_id: string;
  type: string;
  name: string;
  root_path: string;
  include_patterns: string[];
  exclude_patterns: string[];
  scan_interval_minutes: number;
}) {
  return request<DataSourceResponse>('/v1/data-sources', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function updateDataSource(
  sourceId: string,
  input: {
    tenant_id: string;
    type: string;
    name: string;
    root_path: string;
    include_patterns: string[];
    exclude_patterns: string[];
    scan_interval_minutes: number;
  },
) {
  return request<DataSourceResponse>(`/v1/data-sources/${encodeURIComponent(sourceId)}`, {
    method: 'PATCH',
    body: JSON.stringify(input),
  });
}

export async function getDataSource(
  tenantId: string,
  sourceId: string,
  options: {
    scan_entry_limit?: number;
    scan_entry_offset?: number;
    scan_entry_outcome?: string;
  } = {},
) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  if (options.scan_entry_limit) {
    params.set('scan_entry_limit', String(options.scan_entry_limit));
  }
  if (options.scan_entry_offset) {
    params.set('scan_entry_offset', String(options.scan_entry_offset));
  }
  if (options.scan_entry_outcome) {
    params.set('scan_entry_outcome', options.scan_entry_outcome);
  }
  return request<DataSourceDetailResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}?${params.toString()}`,
  );
}

export function dataSourceScanEntriesExportUrl(
  tenantId: string,
  sourceId: string,
  options: { outcome?: string } = {},
) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  if (options.outcome) {
    params.set('outcome', options.outcome);
  }
  return `${apiBase}/v1/data-sources/${encodeURIComponent(sourceId)}/scan-entries.csv?${params.toString()}`;
}

export async function archiveDataSource(
  tenantId: string,
  sourceId: string,
  options: { delete_documents?: boolean } = {},
) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  if (options.delete_documents) {
    params.set('delete_documents', 'true');
  }
  return request<DataSourceResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}?${params.toString()}`,
    { method: 'DELETE' },
  );
}

export async function scanDataSource(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceScanResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/scan?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function cancelDataSourceScan(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceScanResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/scan/cancel?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function preflightDataSource(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceScanResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/preflight?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function planDataSource(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceScanResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/plan?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function reindexDataSource(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceReindexResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/reindex?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function retryFailedDataSourceDocuments(tenantId: string, sourceId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DataSourceReindexResponse>(
    `/v1/data-sources/${encodeURIComponent(sourceId)}/retry-failed-documents?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function getDocument(tenantId: string, documentId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<DocumentDetailResponse>(
    `/v1/documents/${encodeURIComponent(documentId)}?${params.toString()}`,
  );
}

export async function downloadDocument(tenantId: string, documentId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return requestBlob(
    `/v1/documents/${encodeURIComponent(documentId)}/download?${params.toString()}`,
  );
}

export async function retryDocument(tenantId: string, documentId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<RegisterDocumentResponse>(
    `/v1/documents/${encodeURIComponent(documentId)}/retry?${params.toString()}`,
    { method: 'POST' },
  );
}

export async function deleteDocument(tenantId: string, documentId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<{ document: RegisteredDocument }>(
    `/v1/documents/${encodeURIComponent(documentId)}?${params.toString()}`,
    { method: 'DELETE' },
  );
}

export async function listJobs(tenantId: string, limit = 25) {
  const params = new URLSearchParams({ tenant_id: tenantId, limit: String(limit) });
  return request<ListJobsResponse>(`/v1/jobs?${params.toString()}`);
}

export async function listAuditEvents(tenantId: string, limit = 50) {
  const params = new URLSearchParams({ tenant_id: tenantId, limit: String(limit) });
  return request<ListAuditEventsResponse>(`/v1/audit-events?${params.toString()}`);
}

export async function registerDocument(input: DocumentRegistration) {
  return request<RegisterDocumentResponse>('/v1/documents/register', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function uploadDocument(input: {
  tenant_id: string;
  file: File;
}) {
  const form = new FormData();
  form.set('tenant_id', input.tenant_id);
  form.set('file', input.file);

  return requestForm<RegisterDocumentResponse>('/v1/documents/upload', {
    method: 'POST',
    body: form,
  });
}

export async function searchDocuments(input: {
  tenant_id: string;
  document_id?: string;
  query: string;
  limit: number;
}) {
  return request<SearchDocumentsResponse>('/v1/search', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function askConversation(input: AskConversationInput) {
  return request<AskConversationResponse>('/v1/conversations/ask', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function askConversationStream(
  input: AskConversationInput,
  handlers: AskConversationStreamHandlers = {},
) {
  const streamInactivityTimeoutMs = 180000;
  const controller = new AbortController();
  let timedOut = false;
  const abortOnTimeout = () => {
    timedOut = true;
    controller.abort();
  };
  let timeoutID = window.setTimeout(abortOnTimeout, streamInactivityTimeoutMs);
  const resetTimeout = () => {
    window.clearTimeout(timeoutID);
    timeoutID = window.setTimeout(abortOnTimeout, streamInactivityTimeoutMs);
  };
  const abortFromSignal = () => controller.abort();
  if (handlers.signal?.aborted) {
    controller.abort();
  } else {
    handlers.signal?.addEventListener('abort', abortFromSignal, { once: true });
  }

  try {
    handlers.onStatus?.('Connecting');
    const response = await fetch(`${apiBaseUrl}/v1/conversations/ask/stream`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(input),
      signal: controller.signal,
    });

    if (!response.ok) {
      throw await errorFromResponse(response);
    }
    if (!response.body) {
      throw new Error('Streaming is not supported by this browser');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let streamError: Error | null = null;

    function consumeEvent(rawEvent: string) {
      const lines = rawEvent.split(/\r?\n/);
      let event = 'message';
      const data: string[] = [];

      for (const line of lines) {
        if (line.startsWith('event:')) {
          event = line.slice('event:'.length).trim();
        }
        if (line.startsWith('data:')) {
          data.push(line.slice('data:'.length).trimStart());
        }
      }
      if (data.length === 0) {
        return;
      }

      let payload: unknown;
      try {
        payload = JSON.parse(data.join('\n'));
      } catch {
        streamError = new Error('Invalid stream event');
        return;
      }

      if (event === 'status' && isRecord(payload) && typeof payload.message === 'string') {
        handlers.onStatus?.(payload.message);
        return;
      }
      if (event === 'delta' && isRecord(payload) && typeof payload.content === 'string') {
        handlers.onDelta?.(payload.content);
        return;
      }
      if (event === 'done') {
        handlers.onDone?.(payload as AskConversationResponse);
        return;
      }
      if (event === 'error') {
        const message =
          isRecord(payload) && typeof payload.message === 'string'
            ? payload.message
            : 'Stream failed';
        streamError = new Error(message);
      }
    }

    while (true) {
      const { value, done } = await reader.read();
      resetTimeout();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });

      let boundary = buffer.indexOf('\n\n');
      while (boundary >= 0) {
        consumeEvent(buffer.slice(0, boundary));
        if (streamError) {
          throw streamError;
        }
        buffer = buffer.slice(boundary + 2);
        boundary = buffer.indexOf('\n\n');
      }
    }

    buffer += decoder.decode();
    if (buffer.trim() !== '') {
      consumeEvent(buffer);
    }
    if (streamError) {
      throw streamError;
    }
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      if (!timedOut && handlers.signal?.aborted) {
        throw new Error('Request canceled.');
      }
      throw new Error('Model gateway timed out. Check Settings, then test the target.');
    }
    throw err;
  } finally {
    window.clearTimeout(timeoutID);
    handlers.signal?.removeEventListener('abort', abortFromSignal);
  }
}

export async function listConversations(tenantId: string, limit = 25) {
  const params = new URLSearchParams({ tenant_id: tenantId, limit: String(limit) });
  return request<ListConversationsResponse>(`/v1/conversations?${params.toString()}`);
}

export async function deleteConversation(tenantId: string, conversationId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<{ conversation: Conversation }>(
    `/v1/conversations/${encodeURIComponent(conversationId)}?${params.toString()}`,
    { method: 'DELETE' },
  );
}

export async function listConversationMessages(tenantId: string, conversationId: string) {
  const params = new URLSearchParams({ tenant_id: tenantId });
  return request<ListConversationMessagesResponse>(
    `/v1/conversations/${encodeURIComponent(conversationId)}/messages?${params.toString()}`,
  );
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
    throw await errorFromResponse(response);
  }

  return response.json() as Promise<T>;
}

async function requestForm<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, init);

  if (!response.ok) {
    throw await errorFromResponse(response);
  }

  return response.json() as Promise<T>;
}

async function requestBlob(path: string, init?: RequestInit): Promise<Blob> {
  const response = await fetch(`${apiBaseUrl}${path}`, init);

  if (!response.ok) {
    throw await errorFromResponse(response);
  }

  return response.blob();
}

async function errorFromResponse(response: Response) {
  const body = await response.text();
  if (body) {
    try {
      const parsed: unknown = JSON.parse(body);
      if (isRecord(parsed) && typeof parsed.error === 'string') {
        return new Error(parsed.error);
      }
    } catch {
      return new Error(body);
    }
    return new Error(body);
  }
  return new Error(`API returned ${response.status}`);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
