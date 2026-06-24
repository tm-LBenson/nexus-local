import { FormEvent, useEffect, useState } from 'react';
import {
  DocumentRegistration,
  Health,
  ModelTarget,
  Readiness,
  RegisterDocumentResponse,
  apiBase,
  getHealth,
  getModelTargets,
  getReadiness,
  registerDocument,
} from './api';

const initialDocument: DocumentRegistration = {
  tenant_id: 'tenant_1',
  owner_id: 'user_1',
  name: 'Handbook.md',
  storage_key: 'tenants/tenant_1/documents/source.md',
  size_bytes: 42,
};

export function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [targets, setTargets] = useState<ModelTarget[]>([]);
  const [documentForm, setDocumentForm] = useState<DocumentRegistration>(initialDocument);
  const [registration, setRegistration] = useState<RegisterDocumentResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    Promise.all([getHealth(), getReadiness(), getModelTargets()])
      .then(([healthResult, readinessResult, targetsResult]) => {
        setHealth(healthResult);
        setReadiness(readinessResult);
        setTargets(targetsResult.targets);
      })
      .catch((err: unknown) => setError(messageFromError(err)));
  }, []);

  async function submitDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const result = await registerDocument(documentForm);
      setRegistration(result);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSubmitting(false);
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
                <dt>Version</dt>
                <dd>{health?.version ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Storage</dt>
                <dd>{readiness?.persistence_backend ?? 'unknown'}</dd>
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
            <h2>Register Document</h2>
            <form className="documentForm" onSubmit={submitDocument}>
              <label>
                Tenant
                <input
                  value={documentForm.tenant_id}
                  onChange={(event) =>
                    setDocumentForm((current) => ({ ...current, tenant_id: event.target.value }))
                  }
                />
              </label>
              <label>
                Owner
                <input
                  value={documentForm.owner_id}
                  onChange={(event) =>
                    setDocumentForm((current) => ({ ...current, owner_id: event.target.value }))
                  }
                />
              </label>
              <label>
                Name
                <input
                  value={documentForm.name}
                  onChange={(event) =>
                    setDocumentForm((current) => ({ ...current, name: event.target.value }))
                  }
                />
              </label>
              <label>
                Size
                <input
                  min={0}
                  type="number"
                  value={documentForm.size_bytes}
                  onChange={(event) =>
                    setDocumentForm((current) => ({
                      ...current,
                      size_bytes: Number(event.target.value),
                    }))
                  }
                />
              </label>
              <label className="spanAll">
                Storage key
                <input
                  value={documentForm.storage_key}
                  onChange={(event) =>
                    setDocumentForm((current) => ({ ...current, storage_key: event.target.value }))
                  }
                />
              </label>
              <button disabled={submitting} type="submit">
                {submitting ? 'Registering' : 'Register'}
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
        </div>
      </section>
    </main>
  );
}

function messageFromError(err: unknown) {
  return err instanceof Error ? err.message : 'Unknown API error';
}
