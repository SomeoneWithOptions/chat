# Backend

Go API service for auth, chat orchestration, model metadata, and SSE streaming.

## Current implementation

- `GET /health` (recommended for Cloud Run)
- `GET /healthz` (local compatibility)
- Final auth rollout: `POST /v1/auth/google`
- Final auth rollout: `GET /v1/auth/me`
- Final auth rollout: `POST /v1/auth/logout`
- `GET /v1/models`
- `PUT /v1/models/preferences`
- `PUT /v1/models/favorites`
- `POST /v1/files` (multipart upload for `.txt`, `.md`, `.pdf`, `.csv`, `.json`, max 25 MB)
- `POST /v1/conversations`
- `GET /v1/conversations`
- `DELETE /v1/conversations`
- `DELETE /v1/conversations/{id}`
- `GET /v1/conversations/{id}/messages`
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
- `OPENROUTER_API_KEY` (required for `POST /v1/chat/messages` streaming)
- `BRAVE_API_KEY` (required for grounding citations in chat responses)
- `GCS_UPLOAD_BUCKET` (required for attachment uploads)

Auth sequencing:

- Configure `GOOGLE_CLIENT_ID` and `AUTH_REQUIRED=true` during the final auth rollout phase.

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
- `GET /v1/models` syncs available models from OpenRouter into the local `models` cache and returns all/curated lists with user favorites/preferences.
- Grounding is enabled by default per message; Brave search failures are surfaced as non-fatal warnings in the SSE stream.
- Attachments are stored in GCS (`GCS_UPLOAD_BUCKET`) and linked to chat messages through `fileIds`.
