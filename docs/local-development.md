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
- `DEFAULT_CHAT_REASONING_EFFORT` (optional, default `medium`)
- `DEFAULT_DEEP_RESEARCH_REASONING_EFFORT` (optional, default `high`)

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

- Deep research runs a dedicated multi-pass research pipeline (3-6 Brave search passes).
- SSE progress events are streamed with phases: `planning`, `searching`, `synthesizing`, `finalizing`.
- Timeout and cancellation are enforced server-side via `DEEP_RESEARCH_TIMEOUT_SECONDS`.
- When Brave search partially fails, the request continues with available evidence and warning events.
