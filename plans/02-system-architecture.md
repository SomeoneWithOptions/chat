# System Architecture

## High-Level Components

1. Vite React frontend on Vercel
2. Google Identity Services (frontend sign-in)
3. Go API service on Cloud Run
4. Turso DB for users/sessions/chats/messages/models/preferences/files metadata
5. OpenRouter for model inference and model catalog
6. Brave Search API for grounding
7. Local attachment processing path (MVP) with optional future GCS bucket

## Authentication Flow (Final Rollout Phase)

Implementation timing note:

- This flow is enforced in the final rollout phase after core chat, attachments, grounding, and deep-research stabilization.

1. Frontend gets Google ID token from Google Identity Services.
2. Frontend sends token to backend `POST /v1/auth/google`.
3. Backend verifies token signature and claims.
4. Backend enforces email allowlist (initially `acastesol@gmail.com` and `obzen.black@gmail.com`).
5. Backend issues HTTP-only secure session cookie (7-day session).
6. All `/v1/*` application routes require valid session.

## Data Flow (Standard Chat)

1. Frontend sends `POST /v1/chat/messages` with:
   - conversation id
   - user message
   - model id
   - flags (`grounding=true`, `deep_research=false`)
   - attached file ids
2. Go backend validates payload and resolves context.
3. If grounding ON, backend performs search workflow and prepares citations.
4. Backend calls OpenRouter with:
   - system prompt
   - recent conversation
   - grounded snippets (if enabled)
   - file extracted text context
5. Backend streams tokens to frontend via SSE.
6. Backend persists final assistant message and metadata.

## Data Flow (Deep Research)

1. Request enters research orchestrator with selected deep-research model (default: user's last normal-chat model).
2. Orchestrator runs multiple query-expansion + search passes.
3. Builds evidence set with deduped citations.
4. Runs synthesis prompt to produce a structured answer.
5. Streams periodic progress events + final output.

## Service Boundaries

- Frontend handles UI state, sign-in UX, and streaming rendering.
- Backend owns provider logic, web search logic, prompts, citations, cost controls, and final-rollout auth verification/session management.
- DB stores canonical state and indexes.
- Storage layer handles temporary local file processing and extracted text persistence.
- Deletion workflow performs hard delete in DB and removes GCS objects when storage provider is `gcs`.

## Scaling Notes

- Cloud Run concurrency tuned for streaming workloads.
- Long deep-research requests need strict timeout + cancellation.
- Add async job mode later if deep-research duration outgrows request lifecycle.
- Session store can remain DB-backed for MVP and move to Redis later if needed.
- Cloud Run local disk is ephemeral, so local attachments are processing-only until GCS is added.
