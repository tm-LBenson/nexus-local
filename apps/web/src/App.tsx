import { FormEvent, useEffect, useState } from 'react';
import {
  AskConversationResponse,
  CurrentUserResponse,
  Health,
  ListDocumentsResponse,
  ModelTarget,
  Readiness,
  RegisterDocumentResponse,
  SearchDocumentsResponse,
  askConversation,
  apiBase,
  createTenant,
  getHealth,
  getCurrentUser,
  getModelTargets,
  getReadiness,
  listDocuments,
  searchDocuments,
  uploadDocument,
} from './api';

const initialUpload = {
  tenant_id: 'tenant_1',
};

const initialSearch = {
  tenant_id: 'tenant_1',
  query: '',
  limit: 5,
};

const initialAsk = {
  tenant_id: 'tenant_1',
  conversation_id: '',
  model_target: 'general',
  question: '',
  limit: 5,
};

export function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [currentUser, setCurrentUser] = useState<CurrentUserResponse | null>(null);
  const [targets, setTargets] = useState<ModelTarget[]>([]);
  const [uploadForm, setUploadForm] = useState(initialUpload);
  const [documentTenant, setDocumentTenant] = useState(initialUpload.tenant_id);
  const [tenantName, setTenantName] = useState('Personal Workspace');
  const [searchForm, setSearchForm] = useState(initialSearch);
  const [askForm, setAskForm] = useState(initialAsk);
  const [file, setFile] = useState<File | null>(null);
  const [registration, setRegistration] = useState<RegisterDocumentResponse | null>(null);
  const [documents, setDocuments] = useState<ListDocumentsResponse | null>(null);
  const [searchResult, setSearchResult] = useState<SearchDocumentsResponse | null>(null);
  const [askResult, setAskResult] = useState<AskConversationResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [creatingTenant, setCreatingTenant] = useState(false);
  const [searching, setSearching] = useState(false);
  const [asking, setAsking] = useState(false);

  useEffect(() => {
    Promise.all([getHealth(), getReadiness(), getModelTargets(), getCurrentUser()])
      .then(async ([healthResult, readinessResult, targetsResult, currentUserResult]) => {
        setHealth(healthResult);
        setReadiness(readinessResult);
        setTargets(targetsResult.targets);
        setCurrentUser(currentUserResult);
        const tenantId = currentUserResult.memberships[0]?.tenant.id ?? initialUpload.tenant_id;
        applyTenant(tenantId);
        const documentsResult = await listDocuments(tenantId);
        setDocuments(documentsResult);
      })
      .catch((err: unknown) => setError(messageFromError(err)));
  }, []);

  function applyTenant(tenantId: string) {
    setDocumentTenant(tenantId);
    setUploadForm((current) => ({ ...current, tenant_id: tenantId }));
    setSearchForm((current) => ({ ...current, tenant_id: tenantId }));
    setAskForm((current) => ({ ...current, tenant_id: tenantId }));
  }

  async function refreshDocuments(tenantId = documentTenant) {
    setError(null);
    try {
      const result = await listDocuments(tenantId);
      setDocuments(result);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function submitUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      setError('Choose a file before uploading');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const result = await uploadDocument({ ...uploadForm, file });
      setRegistration(result);
      setDocumentTenant(uploadForm.tenant_id);
      await refreshDocuments(uploadForm.tenant_id);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function submitTenant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!tenantName.trim()) {
      setError('Enter a tenant name');
      return;
    }
    setCreatingTenant(true);
    setError(null);
    try {
      const created = await createTenant({ name: tenantName });
      const result = await getCurrentUser();
      setCurrentUser(result);
      applyTenant(created.tenant.id);
      await refreshDocuments(created.tenant.id);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCreatingTenant(false);
    }
  }

  async function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!searchForm.query.trim()) {
      setError('Enter a search query');
      return;
    }
    setSearching(true);
    setError(null);
    try {
      const result = await searchDocuments({
        ...searchForm,
        limit: Number(searchForm.limit),
      });
      setSearchResult(result);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSearching(false);
    }
  }

  async function submitAsk(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!askForm.question.trim()) {
      setError('Enter a question');
      return;
    }
    setAsking(true);
    setError(null);
    try {
      const result = await askConversation({
        ...askForm,
        conversation_id: askForm.conversation_id || undefined,
        limit: Number(askForm.limit),
      });
      setAskResult(result);
      setAskForm((current) => ({
        ...current,
        conversation_id: result.conversation.id,
        question: '',
      }));
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setAsking(false);
    }
  }

  return (
    <main className="shell">
      <section className="workspace">
        <div className="topbar">
          <div>
            <p className="eyebrow">Nexus Local</p>
            <h1>Portable AI workspace</h1>
          </div>
          <span className={health ? 'status statusReady' : 'status'}>
            {health ? 'API online' : 'Connecting'}
          </span>
        </div>

        <div className="grid">
          <article className="panel">
            <h2>Runtime</h2>
            <dl>
              <div>
                <dt>API</dt>
                <dd>{apiBase()}</dd>
              </div>
              <div>
                <dt>Status</dt>
                <dd>{health?.status ?? error ?? 'Checking health'}</dd>
              </div>
              <div>
                <dt>Auth</dt>
                <dd>{readiness?.auth_mode ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Version</dt>
                <dd>{health?.version ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Storage</dt>
                <dd>{readiness?.persistence_backend ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Objects</dt>
                <dd>{readiness?.object_storage_backend ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Vectors</dt>
                <dd>{readiness?.vector_backend ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Embeddings</dt>
                <dd>{readiness?.embedding_backend ?? 'unknown'}</dd>
              </div>
            </dl>
          </article>

          <article className="panel">
            <h2>Model Targets</h2>
            <div className="targetList">
              {targets.map((target) => (
                <div className="targetRow" key={target.name}>
                  <span>{target.name}</span>
                  <strong>{target.model}</strong>
                </div>
              ))}
              {targets.length === 0 && <p className="muted">No targets loaded</p>}
            </div>
          </article>

          <article className="panel panelWide">
            <h2>Upload Document</h2>
            <form className="documentForm" onSubmit={submitUpload}>
              <label>
                Tenant
                <input
                  value={uploadForm.tenant_id}
                  onChange={(event) =>
                    setUploadForm((current) => ({ ...current, tenant_id: event.target.value }))
                  }
                />
              </label>
              <label>
                File
                <input
                  type="file"
                  onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                />
              </label>
              <button disabled={submitting} type="submit">
                {submitting ? 'Uploading' : 'Upload'}
              </button>
            </form>

            {registration && (
              <div className="resultBand">
                <span>{registration.document.id}</span>
                <strong>{registration.job.type}</strong>
                <em>{registration.job.state}</em>
              </div>
            )}

            {error && <p className="errorText">{error}</p>}
          </article>

          <article className="panel panelWide">
            <h2>Tenant Access</h2>
            <div className="identityBar">
              <strong>{currentUser?.user.email ?? 'Unknown user'}</strong>
              <span>{currentUser?.user.id ?? 'No principal'}</span>
            </div>
            <form className="documentForm compactForm" onSubmit={submitTenant}>
              <label>
                Name
                <input value={tenantName} onChange={(event) => setTenantName(event.target.value)} />
              </label>
              <button disabled={creatingTenant} type="submit">
                {creatingTenant ? 'Creating' : 'Create Tenant'}
              </button>
            </form>
            <div className="documentList">
              {currentUser?.memberships.map((membership) => (
                <button
                  className="tenantRow"
                  key={membership.tenant.id}
                  onClick={() => {
                    applyTenant(membership.tenant.id);
                    void refreshDocuments(membership.tenant.id);
                  }}
                  type="button"
                >
                  <strong>{membership.tenant.name}</strong>
                  <span>{membership.role}</span>
                  <em>{membership.tenant.id}</em>
                </button>
              ))}
              {currentUser && currentUser.memberships.length === 0 && (
                <p className="muted">Create a tenant to start using membership-scoped access</p>
              )}
            </div>
          </article>

          <article className="panel panelWide">
            <h2>Document Library</h2>
            <form
              className="documentForm compactForm"
              onSubmit={(event) => {
                event.preventDefault();
                void refreshDocuments();
              }}
            >
              <label>
                Tenant
                <input
                  value={documentTenant}
                  onChange={(event) => setDocumentTenant(event.target.value)}
                />
              </label>
              <button type="submit">Refresh</button>
            </form>

            <div className="documentList">
              {documents?.documents.map((document) => (
                <div className="documentRow" key={document.id}>
                  <strong>{document.name}</strong>
                  <span>{document.status}</span>
                  <em>{document.id}</em>
                  <small>{formatBytes(document.size_bytes)}</small>
                </div>
              ))}
              {documents && documents.documents.length === 0 && (
                <p className="muted">No documents for this tenant</p>
              )}
            </div>
          </article>

          <article className="panel panelWide">
            <h2>Search Documents</h2>
            <form className="documentForm" onSubmit={submitSearch}>
              <label>
                Tenant
                <input
                  value={searchForm.tenant_id}
                  onChange={(event) =>
                    setSearchForm((current) => ({ ...current, tenant_id: event.target.value }))
                  }
                />
              </label>
              <label>
                Limit
                <input
                  min="1"
                  max="20"
                  type="number"
                  value={searchForm.limit}
                  onChange={(event) =>
                    setSearchForm((current) => ({
                      ...current,
                      limit: Number(event.target.value),
                    }))
                  }
                />
              </label>
              <label className="spanAll">
                Query
                <input
                  value={searchForm.query}
                  onChange={(event) =>
                    setSearchForm((current) => ({ ...current, query: event.target.value }))
                  }
                />
              </label>
              <button disabled={searching} type="submit">
                {searching ? 'Searching' : 'Search'}
              </button>
            </form>

            {searchResult && (
              <div className="searchResults">
                {searchResult.hits.map((hit) => (
                  <div className="searchHit" key={`${hit.document_id}:${hit.chunk_id}`}>
                    <div className="searchHitHeader">
                      <strong>{hit.document_id}</strong>
                      <span>{hit.score.toFixed(3)}</span>
                    </div>
                    <p>{hit.text}</p>
                    <em>{hit.chunk_id}</em>
                  </div>
                ))}
                {searchResult.hits.length === 0 && <p className="muted">No matching chunks</p>}
              </div>
            )}
          </article>

          <article className="panel panelWide">
            <h2>Ask Documents</h2>
            <form className="documentForm" onSubmit={submitAsk}>
              <label>
                Tenant
                <input
                  value={askForm.tenant_id}
                  onChange={(event) =>
                    setAskForm((current) => ({ ...current, tenant_id: event.target.value }))
                  }
                />
              </label>
              <label>
                Target
                <input
                  value={askForm.model_target}
                  onChange={(event) =>
                    setAskForm((current) => ({ ...current, model_target: event.target.value }))
                  }
                />
              </label>
              <label>
                Limit
                <input
                  min="1"
                  max="20"
                  type="number"
                  value={askForm.limit}
                  onChange={(event) =>
                    setAskForm((current) => ({ ...current, limit: Number(event.target.value) }))
                  }
                />
              </label>
              <label className="spanAll">
                Conversation
                <input
                  value={askForm.conversation_id}
                  onChange={(event) =>
                    setAskForm((current) => ({ ...current, conversation_id: event.target.value }))
                  }
                />
              </label>
              <label className="spanAll">
                Question
                <input
                  value={askForm.question}
                  onChange={(event) =>
                    setAskForm((current) => ({ ...current, question: event.target.value }))
                  }
                />
              </label>
              <button disabled={asking} type="submit">
                {asking ? 'Asking' : 'Ask'}
              </button>
            </form>

            {askResult && (
              <div className="answerBox">
                <div className="searchHitHeader">
                  <strong>{askResult.conversation.id}</strong>
                  <span>{askResult.completion.model || askResult.conversation.model_target}</span>
                </div>
                <p>{askResult.assistant_message.content}</p>
                <em>{askResult.hits.length} retrieved chunks</em>
              </div>
            )}
          </article>
        </div>
      </section>
    </main>
  );
}

function messageFromError(err: unknown) {
  return err instanceof Error ? err.message : 'Unknown API error';
}

function formatBytes(bytes: number) {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
