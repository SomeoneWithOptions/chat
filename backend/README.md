# Backend

Go API service for auth, chat orchestration, model metadata, and SSE streaming.

## Current implementation

- `GET /healthz`
- `POST /v1/auth/google`
- `GET /v1/auth/me`
- `POST /v1/auth/logout`
- `GET /v1/models`
- `POST /v1/chat/messages` (SSE stream bridged from OpenRouter)

OpenAPI 3.1 contract: `backend/openapi/openapi.yaml`.

## Run locally

1. Copy env template:

```bash
cp backend/.env.example backend/.env
```

2. Fill required values in `backend/.env`:

- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN` (if using `libsql://...` URL)
- `GOOGLE_CLIENT_ID` (required only when `AUTH_REQUIRED=true`)
- `OPENROUTER_API_KEY` (required for `POST /v1/chat/messages` streaming)

For local auth testing without Google verification, set:

- `AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true`

For temporary anonymous testing, set:

- `AUTH_REQUIRED=false`

3. Start server:

```bash
./scripts/dev_backend.sh
```

## Notes

- Session cookie defaults to 7 days (`SESSION_TTL_HOURS=168`).
- Email allowlist is env-configurable (`ALLOWED_GOOGLE_EMAILS`).
- Cookie is HTTP-only and same-site constrained; set `COOKIE_SECURE=true` outside local HTTP.
