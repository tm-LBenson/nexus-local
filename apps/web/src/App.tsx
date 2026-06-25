import { FormEvent, useEffect, useMemo, useState } from 'react';
import {
  AskConversationResponse,
  CurrentUserResponse,
  Health,
  ListConversationMessagesResponse,
  ListConversationsResponse,
  ListDocumentsResponse,
  ListJobsResponse,
  ModelTarget,
  Readiness,
  RegisterDocumentResponse,
  SearchDocumentsResponse,
  askConversation,
  apiBase,
  createTenant,
  deleteDocument,
  getCurrentUser,
  getHealth,
  getModelTargets,
  getReadiness,
  listConversationMessages,
  listConversations,
  listDocuments,
  listJobs,
  searchDocuments,
  uploadDocument,
} from './api';

type View = 'ask' | 'documents' | 'search' | 'activity' | 'history' | 'status' | 'settings';

const fallbackTenantID = 'tenant_1';

const initialAsk = {
  conversation_id: '',
  model_target: 'general',
  question: '',
  limit: 5,
};

export function App() {
  const [activeView, setActiveView] = useState<View>('ask');
  const [health, setHealth] = useState<Health | null>(null);
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [currentUser, setCurrentUser] = useState<CurrentUserResponse | null>(null);
  const [targets, setTargets] = useState<ModelTarget[]>([]);
  const [tenantID, setTenantID] = useState(fallbackTenantID);
  const [tenantName, setTenantName] = useState('Personal Workspace');
  const [file, setFile] = useState<File | null>(null);
  const [registration, setRegistration] = useState<RegisterDocumentResponse | null>(null);
  const [documents, setDocuments] = useState<ListDocumentsResponse | null>(null);
  const [jobs, setJobs] = useState<ListJobsResponse | null>(null);
  const [conversations, setConversations] = useState<ListConversationsResponse | null>(null);
  const [conversationMessages, setConversationMessages] =
    useState<ListConversationMessagesResponse | null>(null);
  const [selectedConversationID, setSelectedConversationID] = useState('');
  const [searchForm, setSearchForm] = useState({ query: '', limit: 5 });
  const [searchResult, setSearchResult] = useState<SearchDocumentsResponse | null>(null);
  const [askForm, setAskForm] = useState(initialAsk);
  const [askResult, setAskResult] = useState<AskConversationResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [creatingTenant, setCreatingTenant] = useState(false);
  const [deletingDocumentID, setDeletingDocumentID] = useState('');
  const [searching, setSearching] = useState(false);
  const [asking, setAsking] = useState(false);

  const selectedTenant = useMemo(
    () => currentUser?.memberships.find((membership) => membership.tenant.id === tenantID),
    [currentUser, tenantID],
  );
  const activeJobCount = useMemo(
    () =>
      jobs?.jobs.filter((job) => ['queued', 'running', 'retrying'].includes(job.state)).length ?? 0,
    [jobs],
  );

  useEffect(() => {
    Promise.all([getHealth(), getReadiness(), getModelTargets(), getCurrentUser()])
      .then(async ([healthResult, readinessResult, targetsResult, currentUserResult]) => {
        setHealth(healthResult);
        setReadiness(readinessResult);
        setTargets(targetsResult.targets);
        setCurrentUser(currentUserResult);
        const initialTenantID = currentUserResult.memberships[0]?.tenant.id ?? fallbackTenantID;
        setTenantID(initialTenantID);
        const [documentsResult, jobsResult, conversationsResult] = await Promise.all([
          listDocuments(initialTenantID),
          listJobs(initialTenantID),
          listConversations(initialTenantID),
        ]);
        setDocuments(documentsResult);
        setJobs(jobsResult);
        setConversations(conversationsResult);
      })
      .catch((err: unknown) => setError(messageFromError(err)));
  }, []);

  async function refreshDocuments(nextTenantID = tenantID) {
    setError(null);
    try {
      setDocuments(await listDocuments(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshJobs(nextTenantID = tenantID) {
    setError(null);
    try {
      setJobs(await listJobs(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshConversations(nextTenantID = tenantID) {
    setError(null);
    try {
      setConversations(await listConversations(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshRuntime() {
    setError(null);
    try {
      const [healthResult, readinessResult, targetsResult, jobsResult] = await Promise.all([
        getHealth(),
        getReadiness(),
        getModelTargets(),
        listJobs(tenantID),
      ]);
      setHealth(healthResult);
      setReadiness(readinessResult);
      setTargets(targetsResult.targets);
      setJobs(jobsResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function switchTenant(nextTenantID: string) {
    setTenantID(nextTenantID);
    setSelectedConversationID('');
    setConversationMessages(null);
    setError(null);
    try {
      const [documentsResult, jobsResult, conversationsResult] = await Promise.all([
        listDocuments(nextTenantID),
        listJobs(nextTenantID),
        listConversations(nextTenantID),
      ]);
      setDocuments(documentsResult);
      setJobs(jobsResult);
      setConversations(conversationsResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function submitUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      setError('Choose a file');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const result = await uploadDocument({ tenant_id: tenantID, file });
      setRegistration(result);
      await Promise.all([refreshDocuments(tenantID), refreshJobs(tenantID)]);
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
      setCurrentUser(await getCurrentUser());
      await switchTenant(created.tenant.id);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCreatingTenant(false);
    }
  }

  async function openConversation(conversation: ListConversationsResponse['conversations'][number]) {
    setSelectedConversationID(conversation.id);
    setAskForm((current) => ({
      ...current,
      conversation_id: conversation.id,
      model_target: conversation.model_target,
    }));
    setError(null);
    try {
      setConversationMessages(await listConversationMessages(tenantID, conversation.id));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  function resumeConversation(conversation: ListConversationsResponse['conversations'][number]) {
    setAskForm((current) => ({
      ...current,
      conversation_id: conversation.id,
      model_target: conversation.model_target,
      question: '',
    }));
    setActiveView('ask');
  }

  async function removeDocument(documentID: string, name: string) {
    if (!window.confirm(`Delete ${name}?`)) {
      return;
    }
    setDeletingDocumentID(documentID);
    setError(null);
    try {
      await deleteDocument(tenantID, documentID);
      await Promise.all([refreshDocuments(tenantID), refreshJobs(tenantID)]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingDocumentID('');
    }
  }

  async function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!searchForm.query.trim()) {
      setError('Enter a query');
      return;
    }
    setSearching(true);
    setError(null);
    try {
      setSearchResult(
        await searchDocuments({
          tenant_id: tenantID,
          query: searchForm.query,
          limit: Number(searchForm.limit),
        }),
      );
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
        tenant_id: tenantID,
        conversation_id: askForm.conversation_id || undefined,
        model_target: askForm.model_target,
        question: askForm.question,
        limit: Number(askForm.limit),
      });
      setAskResult(result);
      setAskForm((current) => ({
        ...current,
        conversation_id: result.conversation.id,
        question: '',
      }));
      setSelectedConversationID(result.conversation.id);
      await refreshConversations(tenantID);
      if (conversationMessages && selectedConversationID === result.conversation.id) {
        setConversationMessages(await listConversationMessages(tenantID, result.conversation.id));
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setAsking(false);
    }
  }

  return (
    <main className="shell">
      <header className="topbar">
        <div className="brandBlock">
          <p className="eyebrow">Nexus Local</p>
          <h1>Workspace</h1>
        </div>
        <nav className="tabs" aria-label="Workspace">
          {(['ask', 'documents', 'search', 'activity', 'history', 'status', 'settings'] as View[]).map((view) => (
            <button
              aria-pressed={activeView === view}
              className={activeView === view ? 'tab tabActive' : 'tab'}
              key={view}
              onClick={() => setActiveView(view)}
              type="button"
            >
              {titleCase(view)}
            </button>
          ))}
        </nav>
        <div className="topActions">
          <select
            aria-label="Tenant"
            className="tenantSelect"
            onChange={(event) => void switchTenant(event.target.value)}
            value={tenantID}
          >
            {currentUser?.memberships.map((membership) => (
              <option key={membership.tenant.id} value={membership.tenant.id}>
                {membership.tenant.name}
              </option>
            ))}
            {(!currentUser || currentUser.memberships.length === 0) && (
              <option value={fallbackTenantID}>{fallbackTenantID}</option>
            )}
          </select>
          <span className={health ? 'status statusReady' : 'status'}>
            {health ? 'Online' : 'Connecting'}
          </span>
        </div>
      </header>

      {error && <div className="toast">{error}</div>}

      <section className="workspace">
        {activeView === 'ask' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Ask</h2>
              <span>{selectedTenant?.tenant.name ?? tenantID}</span>
            </div>
            <form className="askComposer" onSubmit={submitAsk}>
              <textarea
                aria-label="Question"
                onChange={(event) =>
                  setAskForm((current) => ({ ...current, question: event.target.value }))
                }
                placeholder="Ask a question"
                value={askForm.question}
              />
              <div className="composerActions">
                <details className="menuPanel">
                  <summary>Options</summary>
                  <div className="menuFields">
                    <label>
                      Target
                      <select
                        value={askForm.model_target}
                        onChange={(event) =>
                          setAskForm((current) => ({
                            ...current,
                            model_target: event.target.value,
                          }))
                        }
                      >
                        {targets.length === 0 && <option value="general">general</option>}
                        {targets.map((target) => (
                          <option key={target.name} value={target.name}>
                            {target.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Limit
                      <input
                        max="20"
                        min="1"
                        type="number"
                        value={askForm.limit}
                        onChange={(event) =>
                          setAskForm((current) => ({
                            ...current,
                            limit: Number(event.target.value),
                          }))
                        }
                      />
                    </label>
                    <label>
                      Conversation
                      <input
                        value={askForm.conversation_id}
                        onChange={(event) =>
                          setAskForm((current) => ({
                            ...current,
                            conversation_id: event.target.value,
                          }))
                        }
                      />
                    </label>
                  </div>
                </details>
                <button disabled={asking} type="submit">
                  {asking ? 'Asking' : 'Ask'}
                </button>
              </div>
            </form>

            {askResult && (
              <div className="answerBox">
                <div className="answerMeta">
                  <strong>{askResult.conversation.title || askResult.conversation.id}</strong>
                  <span>{askResult.completion.model || askResult.conversation.model_target}</span>
                </div>
                <p>{askResult.assistant_message.content}</p>
                <details className="inlineDetails">
                  <summary>Sources</summary>
                  <div className="resultStack">
                    {askResult.hits.map((hit) => (
                      <ResultHit hit={hit} key={`${hit.document_id}:${hit.chunk_id}`} />
                    ))}
                    {askResult.hits.length === 0 && <p className="muted">No sources</p>}
                  </div>
                </details>
              </div>
            )}
          </div>
        )}

        {activeView === 'documents' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Documents</h2>
              <button onClick={() => void refreshDocuments()} type="button">
                Refresh
              </button>
            </div>

            <form className="uploadBar" onSubmit={submitUpload}>
              <input
                aria-label="Document"
                type="file"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <button disabled={submitting} type="submit">
                {submitting ? 'Uploading' : 'Upload'}
              </button>
            </form>

            {registration && (
              <div className="resultBand">
                <span>{registration.document.name}</span>
                <strong>{registration.job.type}</strong>
                <em>{registration.job.state}</em>
              </div>
            )}

            <div className="tableList">
              {documents?.documents.map((document) => (
                <div className="documentRow" key={document.id}>
                  <strong>{document.name}</strong>
                  <span>{document.status}</span>
                  <em>{document.id}</em>
                  <small>{formatBytes(document.size_bytes)}</small>
                  <details className="rowMenu">
                    <summary>More</summary>
                    <button
                      disabled={deletingDocumentID === document.id}
                      onClick={() => void removeDocument(document.id, document.name)}
                      type="button"
                    >
                      {deletingDocumentID === document.id ? 'Deleting' : 'Delete'}
                    </button>
                  </details>
                </div>
              ))}
              {documents && documents.documents.length === 0 && <p className="muted">No documents</p>}
            </div>
          </div>
        )}

        {activeView === 'activity' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Activity</h2>
              <button onClick={() => void refreshJobs()} type="button">
                Refresh
              </button>
            </div>

            <div className="tableList">
              {jobs?.jobs.map((job) => (
                <div className="jobRow" key={job.id}>
                  <strong>{job.type}</strong>
                  <span>{job.state}</span>
                  <em>{job.resource_id || job.id}</em>
                  <small>{job.attempts} tries</small>
                  <time dateTime={job.updated_at}>{formatDateTime(job.updated_at)}</time>
                </div>
              ))}
              {jobs && jobs.jobs.length === 0 && <p className="muted">No activity</p>}
            </div>
          </div>
        )}

        {activeView === 'history' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>History</h2>
              <button onClick={() => void refreshConversations()} type="button">
                Refresh
              </button>
            </div>

            <div className="tableList">
              {conversations?.conversations.map((conversation) => (
                <div
                  className={
                    selectedConversationID === conversation.id
                      ? 'conversationRow selectedRow'
                      : 'conversationRow'
                  }
                  key={conversation.id}
                >
                  <button onClick={() => void openConversation(conversation)} type="button">
                    <strong>{conversation.title || conversation.id}</strong>
                  </button>
                  <span>{conversation.model_target}</span>
                  <time dateTime={conversation.updated_at}>
                    {formatDateTime(conversation.updated_at)}
                  </time>
                  <button onClick={() => resumeConversation(conversation)} type="button">
                    Resume
                  </button>
                </div>
              ))}
              {conversations && conversations.conversations.length === 0 && (
                <p className="muted">No conversations</p>
              )}
            </div>

            {conversationMessages && (
              <div className="messageStack">
                {conversationMessages.messages.map((message) => (
                  <div className={`messageBubble ${message.role}`} key={message.id}>
                    <span>{message.role}</span>
                    <p>{message.content}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {activeView === 'search' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Search</h2>
              <span>{selectedTenant?.tenant.name ?? tenantID}</span>
            </div>
            <form className="searchBar" onSubmit={submitSearch}>
              <input
                aria-label="Search"
                onChange={(event) =>
                  setSearchForm((current) => ({ ...current, query: event.target.value }))
                }
                placeholder="Search documents"
                value={searchForm.query}
              />
              <details className="menuPanel compactMenu">
                <summary>Options</summary>
                <label>
                  Limit
                  <input
                    max="20"
                    min="1"
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
              </details>
              <button disabled={searching} type="submit">
                {searching ? 'Searching' : 'Search'}
              </button>
            </form>
            {searchResult && (
              <div className="resultStack">
                {searchResult.hits.map((hit) => (
                  <ResultHit hit={hit} key={`${hit.document_id}:${hit.chunk_id}`} />
                ))}
                {searchResult.hits.length === 0 && <p className="muted">No matches</p>}
              </div>
            )}
          </div>
        )}

        {activeView === 'status' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Status</h2>
              <button onClick={() => void refreshRuntime()} type="button">
                Refresh
              </button>
            </div>

            <div className="statusGrid">
              <StatusTile
                detail={health?.version ?? apiBase()}
                label="API"
                ready={health?.status === 'ok'}
                value={health?.status ?? 'offline'}
              />
              <StatusTile
                detail={readiness?.run_migrations ? 'migrations on' : 'migrations off'}
                label="Database"
                ready={readiness?.status === 'ready'}
                value={readiness?.persistence_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.object_store_configured ? 'configured' : 'local profile'}
                label="Objects"
                ready={readiness?.status === 'ready'}
                value={readiness?.object_storage_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.vector_collection ?? 'documents'}
                label="Vectors"
                ready={readiness?.status === 'ready'}
                value={readiness?.vector_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.embedding_model ?? 'unknown'}
                label="Embeddings"
                ready={readiness?.status === 'ready'}
                value={readiness?.embedding_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.model_gateway_auth ? 'auth configured' : 'no gateway auth'}
                label="Model Gateway"
                ready={readiness?.status === 'ready'}
                value={readiness?.model_gateway ?? 'unknown'}
              />
              <StatusTile
                detail={`${jobs?.jobs.length ?? 0} recent jobs`}
                label="Worker"
                ready={Boolean(jobs)}
                value={activeJobCount > 0 ? `${activeJobCount} active` : 'idle'}
              />
              <StatusTile
                detail={`${targets.length} configured`}
                label="Targets"
                ready={targets.length > 0}
                value={targets[0]?.name ?? 'none'}
              />
            </div>

            <details className="inlineDetails">
              <summary>Endpoints</summary>
              <dl className="runtimeList">
                <div>
                  <dt>API</dt>
                  <dd>{apiBase()}</dd>
                </div>
                <div>
                  <dt>Queue</dt>
                  <dd>{readiness?.queue_backend ?? 'unknown'}</dd>
                </div>
                <div>
                  <dt>Models</dt>
                  <dd>{readiness?.model_gateway ?? 'unknown'}</dd>
                </div>
                <div>
                  <dt>Embeddings</dt>
                  <dd>{readiness?.embedding_gateway ?? 'unknown'}</dd>
                </div>
              </dl>
            </details>
          </div>
        )}

        {activeView === 'settings' && (
          <div className="settingsGrid">
            <section className="workSurface">
              <div className="surfaceHeader">
                <h2>Tenants</h2>
                <span>{currentUser?.user.email ?? 'Unknown user'}</span>
              </div>
              <form className="inlineForm" onSubmit={submitTenant}>
                <input
                  aria-label="Tenant name"
                  onChange={(event) => setTenantName(event.target.value)}
                  value={tenantName}
                />
                <button disabled={creatingTenant} type="submit">
                  {creatingTenant ? 'Creating' : 'Create'}
                </button>
              </form>
              <div className="tableList">
                {currentUser?.memberships.map((membership) => (
                  <button
                    className="tenantRow"
                    key={membership.tenant.id}
                    onClick={() => void switchTenant(membership.tenant.id)}
                    type="button"
                  >
                    <strong>{membership.tenant.name}</strong>
                    <span>{membership.role}</span>
                    <em>{membership.tenant.id}</em>
                  </button>
                ))}
                {currentUser && currentUser.memberships.length === 0 && (
                  <p className="muted">No tenants</p>
                )}
              </div>
            </section>

            <section className="workSurface">
              <div className="surfaceHeader">
                <h2>Runtime</h2>
                <span>{readiness?.auth_mode ?? 'unknown'}</span>
              </div>
              <dl className="runtimeList">
                <div>
                  <dt>API</dt>
                  <dd>{apiBase()}</dd>
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
              <details className="inlineDetails">
                <summary>Model Targets</summary>
                <div className="tableList">
                  {targets.map((target) => (
                    <div className="targetRow" key={target.name}>
                      <span>{target.name}</span>
                      <strong>{target.model}</strong>
                    </div>
                  ))}
                  {targets.length === 0 && <p className="muted">No targets</p>}
                </div>
              </details>
            </section>
          </div>
        )}
      </section>
    </main>
  );
}

function ResultHit({ hit }: { hit: SearchDocumentsResponse['hits'][number] }) {
  return (
    <div className="searchHit">
      <div className="answerMeta">
        <strong>{hit.document_id}</strong>
        <span>{hit.score.toFixed(3)}</span>
      </div>
      <p>{hit.text}</p>
      <em>{hit.chunk_id}</em>
    </div>
  );
}

function StatusTile({
  detail,
  label,
  ready,
  value,
}: {
  detail: string;
  label: string;
  ready: boolean;
  value: string;
}) {
  return (
    <div className={ready ? 'statusTile statusTileReady' : 'statusTile'}>
      <span>{label}</span>
      <strong>{value}</strong>
      <em>{detail}</em>
    </div>
  );
}

function titleCase(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
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

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));
}
