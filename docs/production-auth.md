# Production Auth Profile

Nexus Local supports a production-oriented `trusted-header` auth mode for deployments behind an identity-aware reverse proxy. In this mode, the API does not accept a dev user fallback. It trusts identity headers only from the private proxy path.

The included profile uses Caddy as the public entrypoint and expects an Authelia-compatible forward-auth service.

```text
browser -> Caddy -> Authelia forward-auth
              |-> web
              |-> /api -> Nexus Local API
```

## What This Profile Does

- Exposes only Caddy on the host.
- Keeps API, web, Postgres, MinIO, Qdrant, NATS, and Valkey internal to Compose.
- Builds the web app with `WEB_API_BASE_URL=/api`.
- Runs the API with `AUTH_MODE=trusted-header`.
- Strips client-supplied identity headers before auth.
- Copies successful auth headers from the auth gateway:
  - `Remote-User` -> `X-User-ID`
  - `Remote-Email` -> `X-User-Email`

The API maps these headers through:

```env
TRUSTED_USER_ID_HEADER=X-User-ID
TRUSTED_EMAIL_HEADER=X-User-Email
```

## Required Auth Gateway Contract

Your forward-auth service must:

- Redirect unauthenticated users to login.
- Return a 2xx response to Caddy after authentication.
- Include stable `Remote-User` and `Remote-Email` response headers.

Authelia's Caddy integration uses Caddy's `forward_auth` directive and `/api/authz/forward-auth` authorization endpoint. Authentik or another gateway can also work if it provides the same forward-auth response headers.

## Start The Profile

Create a local environment file:

```powershell
Copy-Item .env.example .env
```

Edit `.env` for your deployment:

```env
NEXUS_PUBLIC_URL=https://nexus.example.com
NEXUS_SITE_ADDRESS=nexus.example.com
NEXUS_HTTP_PORT=80
NEXUS_HTTPS_PORT=443
AUTHELIA_INTERNAL_URL=http://authelia:9091
POSTGRES_PASSWORD=replace-me
OBJECT_STORAGE_ACCESS_KEY=replace-me
OBJECT_STORAGE_SECRET_KEY=replace-me
MODEL_GATEWAY_BASE_URL=http://gpu-host:8000/v1
```

Start:

```powershell
docker compose -f deploy\compose\compose.prod-auth.yml --env-file .env up -d --build
```

For a local proxy test without public TLS:

```powershell
docker compose -f deploy\compose\compose.prod-auth.yml up -d --build
```

Then open:

```text
http://localhost:8088
```

That local command still requires a reachable forward-auth gateway at `AUTHELIA_INTERNAL_URL`. Without it, Caddy should fail closed rather than serving the app.

## First User Bootstrap

1. Sign in through the auth gateway.
2. Open Nexus Local.
3. The first authenticated user lands in Settings with no workspace.
4. Create the first workspace.
5. Use Settings -> Members to add additional users by the same user ID and email emitted by the gateway.

## Security Notes

- Do not expose the API container port directly in production.
- Do not run `AUTH_MODE=dev` outside local development.
- Use long random values for Postgres, MinIO, Authelia, and session secrets.
- Keep the auth gateway, Caddy, and API on a trusted private network.
- Validate that direct client-supplied `X-User-ID` and `X-User-Email` headers do not reach the API.

References:

- Caddy `forward_auth`: https://caddyserver.com/docs/caddyfile/directives/forward_auth
- Authelia Caddy integration: https://www.authelia.com/integration/proxies/caddy/
