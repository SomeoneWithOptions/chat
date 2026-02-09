# Backend Plan (Go + OpenRouter)

## Suggested Stack

- Go 1.22+
- Router: Chi (or Gin, but Chi keeps dependencies small)
- HTTP streaming: SSE for token streaming
- Config: env-based with typed config loader

## API Contract Standard

- Use OpenAPI 3.1 as the canonical API contract.
- Document REST JSON endpoints and the streaming endpoint in the same spec.
- For streaming responses, define `text/event-stream` on `POST /v1/chat/messages`.
- Keep server implementation SSE-based (recommended standard for one-way token streaming to browser clients).

## API Endpoints (MVP)

1. `GET /healthz`
2. `POST /v1/auth/google` (exchange Google ID token -> session)
3. `GET /v1/auth/me` (session check)
4. `POST /v1/auth/logout`
5. `GET /v1/models`
6. `POST /v1/files` (upload metadata/start)
7. `POST /v1/chat/messages` (streaming)
8. `GET /v1/conversations`
9. `POST /v1/conversations`
10. `GET /v1/conversations/{id}/messages`
11. `DELETE /v1/conversations/{id}` (delete one chat)
12. `DELETE /v1/conversations` (delete all chats for current user)

## Authentication (Google, Email Allowlist)

- Verify Google ID token server-side (issuer, audience, signature, expiry).
- Enforce allowlist via config (`ALLOWED_GOOGLE_EMAILS` comma-separated).
- Initial allowlist values:
  - `acastesol@gmail.com`
  - `obzen.black@gmail.com`
- Create/update local `users` row and issue secure HTTP-only session cookie.
- Session TTL: 7 days (168 hours).
- Apply auth middleware to all non-auth routes.

## OpenRouter Integration

- Use OpenRouter chat completion endpoint with streaming mode.
- Sync available models from OpenRouter:
  - Primary: cache models from OpenRouter API
  - UI behavior: curated list first, optional show-all list
  - Fallback: manual curated list via env/file can be empty initially
  - First-run default model: `openrouter/free`
- Track model metadata in Turso:
  - id, provider, context window, pricing snapshot
- Store per-user model preferences:
  - favorite models
  - last-used normal-chat model
  - last-used deep-research model

## Request Orchestration

1. Validate request + limits
2. Authorize session and user ownership
3. Load conversation context window
4. Resolve attachments into text snippets
5. Execute grounding workflow (if enabled)
6. Select prompt template by mode (`chat` or `deep_research`)
7. Resolve model for mode:
   - normal: last-used model, else `openrouter/free`
   - deep research: user-selected or default to last-used normal model
8. Call OpenRouter and stream response chunks
9. Persist assistant output and citations

## Deletion Semantics

- `DELETE /v1/conversations/{id}` hard-deletes the conversation and dependent rows.
- `DELETE /v1/conversations` hard-deletes all user conversations and dependent rows.
- If related attachments are in GCS, delete those objects as part of the same workflow.
- If attachments were locally sourced, do not attempt deletion of original local user files.

## Guardrails

- Max input token estimate per request
- Max attachment count/size (25 MB per file)
- Timeout + cancellation propagation
- Provider error mapping with retry policy (idempotent-safe only)
- Session cookie security flags and CSRF strategy for auth endpoints

## Acceptance Criteria

- Streaming response works reliably via Cloud Run
- Invalid or non-allowlisted Google accounts cannot access chat APIs
- Model list endpoint returns usable options from OpenRouter
- Manual model fallback works when provider list fetch fails
- Deep research respects 120s timeout and model-selection behavior
- Chat deletion fully removes DB records and cleans up GCS-backed attachments
- Errors return consistent JSON envelope
