# Frontend

React + Vite + TypeScript app (dark-mode-only shell).

## Current implementation

- Model fetch via `GET /v1/models` (all + curated, favorites, preferences)
- Model preference persistence via `PUT /v1/models/preferences`
- Model favorites via `PUT /v1/models/favorites`
- Conversation list/history via `GET /v1/conversations` and `GET /v1/conversations/{id}/messages`
- Conversation creation via `POST /v1/conversations`
- Conversation deletion via `DELETE /v1/conversations/{id}` and `DELETE /v1/conversations`
- Chat composer with SSE stream handling from `POST /v1/chat/messages`
- Attachment upload UX and send with `fileIds`
- Final auth rollout: session bootstrap via `GET /v1/auth/me`, Google Identity Services sign-in when `VITE_GOOGLE_CLIENT_ID` is configured, and dev sign-in fallback for insecure local mode

## Run locally

1. Install deps:

```bash
cd frontend && bun install
```

2. Configure API origin:

```bash
cp frontend/.env.example frontend/.env
```

Set:

- `VITE_API_BASE_URL` (backend API origin)
- `VITE_GOOGLE_CLIENT_ID` (required for production Google sign-in; optional for local insecure mode)

3. Start app:

```bash
./scripts/dev_frontend.sh
```

Default local URL: `http://localhost:5173`.
