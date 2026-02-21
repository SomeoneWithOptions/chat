# Backend

Go API service for auth, chat orchestration, model metadata, and SSE streaming.

## Current implementation

- `GET /health` (recommended for Cloud Run)
- `GET /healthz` (local compatibility)
- Final auth rollout: `POST /v1/auth/google`
- Final auth rollout: `GET /v1/auth/me`
- Final auth rollout: `POST /v1/auth/logout`
- `GET /v1/models`
- `POST /v1/models/sync`
- `PUT /v1/models/preferences`
- `PUT /v1/models/favorites`
- `PUT /v1/models/reasoning-presets`
- `POST /v1/files` (multipart upload for `.txt`, `.md`, `.pdf`, `.csv`, `.json`, max 25 MB)
- `POST /v1/conversations`
- `GET /v1/conversations`
- `DELETE /v1/conversations`
- `DELETE /v1/conversations/{id}`
- `GET /v1/conversations/{id}/messages`
- `POST /v1/chat/messages` (SSE stream bridged from OpenRouter, including usage metrics when available)

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
- `MODEL_SYNC_BEARER_TOKEN` (required for `POST /v1/models/sync`)
- `DEFAULT_CHAT_REASONING_EFFORT` (optional: `low`, `medium`, `high`; default `medium`)
- `DEFAULT_DEEP_RESEARCH_REASONING_EFFORT` (optional: `low`, `medium`, `high`; default `high`)
- `AGENTIC_RESEARCH_CHAT_ENABLED` / `AGENTIC_RESEARCH_DEEP_ENABLED` (optional; default `true`)
- `CHAT_RESEARCH_MAX_LOOPS`, `CHAT_RESEARCH_MAX_SOURCES_READ`, `CHAT_RESEARCH_MAX_SEARCH_QUERIES`, `CHAT_RESEARCH_TIMEOUT_SECONDS` (optional chat budgets)
- `DEEP_RESEARCH_MAX_LOOPS`, `DEEP_RESEARCH_MAX_SOURCES_READ`, `DEEP_RESEARCH_MAX_SEARCH_QUERIES` (optional deep budgets)
- `RESEARCH_SOURCE_FETCH_TIMEOUT_SECONDS`, `RESEARCH_SOURCE_MAX_BYTES` (optional source-read safety limits)
- `RESEARCH_MAX_CITATIONS_CHAT`, `RESEARCH_MAX_CITATIONS_DEEP` (optional citation caps)

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

- Session cookie defaults to 30 days (`SESSION_TTL_HOURS=720`).
- Email allowlist is env-configurable (`ALLOWED_GOOGLE_EMAILS`).
- Cookie is HTTP-only and same-site constrained; set `COOKIE_SECURE=true` outside local HTTP.
- `GET /v1/models` returns cached models from the local `models` table (no provider sync in request path).
- `GET /v1/models` returns model capability metadata (`supportsReasoning`) and user reasoning presets.
- `POST /v1/models/sync` performs an on-demand OpenRouter sync into the local `models` cache and returns the synced row count.
  - Requires `Authorization: Bearer <MODEL_SYNC_BEARER_TOKEN>`.
- `PUT /v1/models/reasoning-presets` updates per-model reasoning effort presets for `chat` or `deep_research`.
- Grounding is enabled by default per message; Brave search failures are surfaced as non-fatal warnings in the SSE stream.
- Chat and deep research both support iterative agentic web research loops behind independent feature flags.
- Deep research uses larger loop/query/read budgets than normal chat and still respects `DEEP_RESEARCH_TIMEOUT_SECONDS`.
- Attachments are stored in GCS (`GCS_UPLOAD_BUCKET`) and linked to chat messages through `fileIds`.
