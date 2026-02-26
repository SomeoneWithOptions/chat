# Local Development

## Prerequisites

- Go 1.22+
- Bun 1.0+
- Turso CLI (`turso`) authenticated with an account that can create DBs

## 1) Turso database connection

We use the production database for local development.

Get connection values:

```bash
turso db show chat --url
turso db tokens create chat
```

## 2) Configure backend

```bash
cp backend/.env.example backend/.env
```

Set at minimum:

- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN` (required for `libsql://` URL)
- `OPENROUTER_API_KEY` (required to stream chat responses)
- `BRAVE_API_KEY` (required for grounding citations; requests still run without it)
- `GCS_UPLOAD_BUCKET` (required to upload attachments)
- `MODEL_SYNC_BEARER_TOKEN` (required for `POST /v1/models/sync`; send it as `Authorization: Bearer <token>`)
- `DEEP_RESEARCH_TIMEOUT_SECONDS` (optional, default `150`; applies to deep-research requests only)
- `AGENTIC_RESEARCH_CHAT_ENABLED` / `AGENTIC_RESEARCH_DEEP_ENABLED` (optional; default `true`)
- `CHAT_RESEARCH_MAX_LOOPS`, `CHAT_RESEARCH_MAX_SOURCES_READ`, `CHAT_RESEARCH_MAX_SEARCH_QUERIES`, `CHAT_RESEARCH_TIMEOUT_SECONDS` (optional chat budgets)
- `DEEP_RESEARCH_MAX_LOOPS`, `DEEP_RESEARCH_MAX_SOURCES_READ`, `DEEP_RESEARCH_MAX_SEARCH_QUERIES` (optional deep budgets)
- `RESEARCH_SOURCE_FETCH_TIMEOUT_SECONDS`, `RESEARCH_SOURCE_MAX_BYTES` (optional source-read limits; defaults: `12` seconds and `1500000` bytes)
- `RESEARCH_MAX_CITATIONS_CHAT`, `RESEARCH_MAX_CITATIONS_DEEP` (optional citation caps)
- `DEFAULT_CHAT_REASONING_EFFORT` (optional, default `medium`)
- `DEFAULT_DEEP_RESEARCH_REASONING_EFFORT` (optional, default `high`)
- `SESSION_TTL_HOURS` (optional, default `720` for 30-day reauthentication)

Auth sequencing:

- Add `GOOGLE_CLIENT_ID` and enable `AUTH_REQUIRED=true` in the final auth rollout phase.

For temporary auth bypass while testing app flows:

- `AUTH_REQUIRED=false`

For local dev-only login flow:

- `AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true`

## 3) Run backend and frontend

```bash
./scripts/dev_backend.sh
./scripts/dev_frontend.sh
```

Frontend env setup:

```bash
cp frontend/.env.example frontend/.env
```

- Set `VITE_API_BASE_URL=http://localhost:8080`
- Set `VITE_GOOGLE_CLIENT_ID=<google-client-id>` when testing real Google sign-in

- Backend default: `http://localhost:8080`
- Frontend default: `http://localhost:5173`

## 4) Validate health

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```

## Deep Research Behavior (Local)

- Chat and deep research can run the shared agentic loop (`plan -> search -> read -> evaluate -> iterate/finalize`) when enabled.
- Planner/decision model calls in that loop use the selected model for the request (same model used for final response generation).
- Deep research uses larger default budgets than chat and remains bounded by `DEEP_RESEARCH_TIMEOUT_SECONDS`.
- SSE progress events include phases: `planning`, `searching`, `reading`, `evaluating`, `iterating`, `synthesizing`, `finalizing`.
- Progress is rendered in each assistant message as a compact thinking summary; expand it to view full phase-by-phase trace details.
- Raw provider reasoning text is available as an opt-in nested section within the message thinking panel when streamed by the model.
- Timeout and cancellation are enforced server-side via `DEEP_RESEARCH_TIMEOUT_SECONDS`.
- When Brave search partially fails, the request continues with available evidence and warning events.
