# Implementation Roadmap

## Phase 0: Foundation

1. Create mono-repo/app layout:
   - `/frontend`, `/backend`, `/db`
   - support folders: `/infra`, `/docs`, `/scripts`
2. Configure local env and secrets management
3. Add local lint/test/build scripts (CI workflows can be added later)
4. Configure domains and CORS:
   - frontend `chat.sanetomore.com`
   - backend `api.chat.sanetomore.com` (recommended)
   - localhost origins for dev
5. Set `OPENROUTER_FREE_TIER_DEFAULT_MODEL=openrouter/free`
6. Create OpenAPI 3.1 baseline contract with `text/event-stream` for chat streaming endpoint

Exit criteria:

- Local frontend and backend run together
- Local quality commands run successfully

## Phase 1: Core Chat Loop

1. Users/sessions/conversation/message DB schema + SQL bootstrap/change scripts
2. Backend `/chat/messages` with OpenRouter streaming
3. Frontend chat UI with model selector

Exit criteria:

- User can send message with streaming response in non-auth rollout mode

## Phase 2: Model Catalog and Persistence

1. Integrate OpenRouter model fetch and cache
2. Persist model capability metadata from provider (`supported_parameters`)
3. Add curated list + show-all behavior (curated can start empty)
4. Add favorites and per-user model preferences
5. Persist conversations and history UI

Exit criteria:

- Models selectable and stable even when provider list is unavailable
- Pricing and context-window metadata visible in UI

## Phase 3: Reasoning Presets (Per Model + Mode)

1. Add DB schema for user model reasoning presets
2. Add backend API to persist presets (`PUT /v1/models/reasoning-presets`)
3. Extend model catalog response with reasoning capability flags and stored presets
4. Add frontend thinking-level selector for selected model
5. Include effort override in `POST /v1/chat/messages`
6. Resolve and apply reasoning effort for both normal chat and deep research

Exit criteria:

- User can set and persist reasoning effort per model for both modes
- Reasoning selector state follows selected model and mode correctly
- Backend sends reasoning parameters only for compatible models

## Phase 4: File Attachments

1. Upload endpoint + file metadata
2. Text extraction for MVP file types
3. Include extracted context in prompts
4. Add delete one/delete all chat operations and UI

Exit criteria:

- File-backed questions produce context-aware responses
- 25 MB file limit enforced
- Delete operations hard-delete DB data; GCS files are removed when storage backend is GCS

## Phase 5: Grounding (Default ON)

1. Brave API integration
2. Citation storage/rendering
3. Toggle behavior + graceful fallback on errors

Exit criteria:

- Grounded results with citations are default behavior

## Phase 6: Deep Research Mode

1. Multi-pass search orchestration
2. Deep research prompt templates + structured outputs
3. Progress streaming events in API/UI (`planning`, `searching`, `synthesizing`, `finalizing`)
4. Timeout/cancellation enforcement with `DEEP_RESEARCH_TIMEOUT_SECONDS`
5. Deep-research citation quality controls (dedupe, confidence filtering, claim-order alignment)
6. Backend + frontend streaming progress tests

Exit criteria:

- Deep research mode reliably provides richer analysis than normal mode
- Deep research requests fail gracefully within configured timeout bounds

## Phase 7: Production Hardening

1. Observability and alerts
2. Security/rate limits
3. Staging load and failure tests
4. Plan and test migration from local attachment path to GCS

Exit criteria:

- Stable production deployment on Vercel + Cloud Run

## Phase 8: Authentication Final Rollout (Last)

1. Configure Google OAuth app + email allowlist (`acastesol@gmail.com`, `obzen.black@gmail.com`)
2. Configure and enforce 7-day session TTL
3. Enable backend auth middleware and auth endpoints in production (`AUTH_REQUIRED=true`)
4. Enable frontend auth gate and sign-in bootstrap for production
5. Run auth-specific integration tests and production smoke checks

Exit criteria:

- Only allowlisted Google accounts can access chat APIs
- Session issue/validation/logout works end-to-end in production
