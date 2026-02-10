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
2. `GET /v1/models`
3. `PUT /v1/models/reasoning-presets`
4. `POST /v1/files` (upload metadata/start)
5. `POST /v1/chat/messages` (streaming)
6. `GET /v1/conversations`
7. `POST /v1/conversations`
8. `GET /v1/conversations/{id}/messages`
9. `DELETE /v1/conversations/{id}` (delete one chat)
10. `DELETE /v1/conversations` (delete all chats for current user)
11. Final rollout auth endpoints:
    - `POST /v1/auth/google` (exchange Google ID token -> session)
    - `GET /v1/auth/me` (session check)
    - `POST /v1/auth/logout`

## Authentication (Google, Email Allowlist, Final Rollout Gate)

- Verify Google ID token server-side (issuer, audience, signature, expiry).
- Enforce allowlist via config (`ALLOWED_GOOGLE_EMAILS` comma-separated).
- Initial allowlist values:
  - `acastesol@gmail.com`
  - `obzen.black@gmail.com`
- Create/update local `users` row and issue secure HTTP-only session cookie.
- Session TTL: 7 days (168 hours).
- Apply auth middleware to all non-auth routes in the final rollout phase.

## OpenRouter Integration

- Use OpenRouter chat completion endpoint with streaming mode.
- Sync available models from OpenRouter:
  - Primary: cache models from OpenRouter API
  - Persist model capability metadata from `supported_parameters` (at minimum whether reasoning controls are supported)
  - UI behavior: curated list first, optional show-all list
  - Fallback: manual curated list via env/file can be empty initially
  - First-run default model: `openrouter/free`
- Track model metadata in Turso:
  - id, provider, context window, pricing snapshot
- Store per-user model preferences:
  - favorite models
  - last-used normal-chat model
  - last-used deep-research model
  - reasoning effort preset per `(user_id, model_id, mode)`
- Request payload to OpenRouter:
  - include `reasoning.effort` when model supports reasoning and an effort value is selected/resolved
  - omit reasoning fields for models without reasoning support

## Request Orchestration

1. Validate request + limits
2. Authorize session and user ownership (enforced as final rollout gate)
3. Load conversation context window
4. Resolve attachments into text snippets
5. Execute grounding workflow (if enabled)
6. Select prompt template by mode (`chat` or `deep_research`)
7. Resolve model for mode:
   - normal: last-used model, else `openrouter/free`
   - deep research: user-selected or default to last-used normal model
8. Resolve reasoning effort:
   - explicit request override, else per-model+mode preset, else backend default
   - validate against supported model capability and normalize value
9. Call OpenRouter and stream response chunks
10. Persist assistant output and citations

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
- Model list endpoint returns usable options from OpenRouter
- Manual model fallback works when provider list fetch fails
- Reasoning preset endpoint persists and returns per-model + per-mode values
- Chat/deep-research requests apply reasoning effort correctly when model supports it
- Deep research respects 150s timeout and model-selection behavior
- Chat deletion fully removes DB records and cleans up GCS-backed attachments
- Errors return consistent JSON envelope
- Final rollout gate: invalid or non-allowlisted Google accounts cannot access chat APIs
