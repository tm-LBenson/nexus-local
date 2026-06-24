import { useEffect, useState } from 'react';

type Health = {
  status: string;
  env: string;
  version: string;
};

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch(`${apiBaseUrl}/healthz`)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`API returned ${response.status}`);
        }
        return response.json();
      })
      .then(setHealth)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Unknown API error');
      });
  }, []);

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
                <dd>{apiBaseUrl}</dd>
              </div>
              <div>
                <dt>Status</dt>
                <dd>{health?.status ?? error ?? 'Checking health'}</dd>
              </div>
              <div>
                <dt>Version</dt>
                <dd>{health?.version ?? 'unknown'}</dd>
              </div>
            </dl>
          </article>

          <article className="panel">
            <h2>Architecture</h2>
            <ul>
              <li>OpenAI-compatible model gateway</li>
              <li>S3-compatible document storage</li>
              <li>Qdrant vector search</li>
              <li>NATS-backed jobs and events</li>
            </ul>
          </article>
        </div>
      </section>
    </main>
  );
}

