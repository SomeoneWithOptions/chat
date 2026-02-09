# Implementation Roadmap

## Phase 0: Foundation

1. Create mono-repo/app layout:
   - `/frontend`, `/backend`, `/db`
   - support folders: `/infra`, `/docs`, `/scripts`
2. Configure local env and secrets management
3. Add local lint/test/build scripts (CI workflows can be added later)
4. Configure Google OAuth app + email allowlist (`acastesol@gmail.com`, `obzen.black@gmail.com`)
5. Configure 7-day session TTL
6. Configure domains and CORS:
   - frontend `chat.sanetomore.com`
   - backend `api.chat.sanetomore.com` (recommended)
   - localhost origins for dev
7. Set `OPENROUTER_FREE_TIER_DEFAULT_MODEL=openrouter/free`
8. Create OpenAPI 3.1 baseline contract with `text/event-stream` for chat streaming endpoint

Exit criteria:

- Local frontend and backend run together
- Local quality commands run successfully
- Login can issue and validate local session

## Phase 1: Core Chat Loop

1. Users/sessions/conversation/message DB schema + SQL bootstrap/change scripts
2. Backend auth endpoints + auth middleware
3. Backend `/chat/messages` with OpenRouter streaming
4. Frontend chat UI with model selector and auth gate

Exit criteria:

- Allowed user can login and send message with streaming response

## Phase 2: Model Catalog and Persistence

1. Integrate OpenRouter model fetch and cache
2. Add curated list + show-all behavior (curated can start empty)
3. Add favorites and per-user model preferences
4. Persist conversations and history UI

Exit criteria:

- Models selectable and stable even when provider list is unavailable
- Pricing and context-window metadata visible in UI

## Phase 3: File Attachments

1. Upload endpoint + file metadata
2. Text extraction for MVP file types
3. Include extracted context in prompts
4. Add delete one/delete all chat operations and UI

Exit criteria:

- File-backed questions produce context-aware responses
- 25 MB file limit enforced
- Delete operations hard-delete DB data; GCS files are removed when storage backend is GCS

## Phase 4: Grounding (Default ON)

1. Brave API integration
2. Citation storage/rendering
3. Toggle behavior + graceful fallback on errors

Exit criteria:

- Grounded results with citations are default behavior

## Phase 5: Deep Research Mode

1. Multi-pass search orchestration
2. Deep research prompt templates + structured outputs
3. Progress streaming events in UI

Exit criteria:

- Deep research mode reliably provides richer analysis than normal mode

## Phase 6: Production Hardening

1. Observability and alerts
2. Security/rate limits
3. Staging load and failure tests
4. Plan and test migration from local attachment path to GCS

Exit criteria:

- Stable production deployment on Vercel + Cloud Run
