import { FormEvent, useEffect, useState } from 'react';
import {
  Health,
  ModelTarget,
  Readiness,
  RegisterDocumentResponse,
  apiBase,
  getHealth,
  getModelTargets,
  getReadiness,
  uploadDocument,
} from './api';

const initialUpload = {
  tenant_id: 'tenant_1',
  owner_id: 'user_1',
};

export function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [targets, setTargets] = useState<ModelTarget[]>([]);
  const [uploadForm, setUploadForm] = useState(initialUpload);
  const [file, setFile] = useState<File | null>(null);
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
              <div>
                <dt>Objects</dt>
                <dd>{readiness?.object_storage_backend ?? 'unknown'}</dd>
              </div>
              <div>
                <dt>Vectors</dt>
                <dd>{readiness?.vector_backend ?? 'unknown'}</dd>
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
                Owner
                <input
                  value={uploadForm.owner_id}
                  onChange={(event) =>
                    setUploadForm((current) => ({ ...current, owner_id: event.target.value }))
                  }
                />
              </label>
              <label className="spanAll">
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
        </div>
      </section>
    </main>
  );
}

function messageFromError(err: unknown) {
  return err instanceof Error ? err.message : 'Unknown API error';
}
