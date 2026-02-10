# Infra and Deployment Plan (Vercel + Cloud Run + Turso + GCP)

## Environments

1. `dev`
2. `staging`
3. `prod`

Each env has isolated API keys and DB tokens.

## Frontend (Vercel)

- Build command: `bun run build`
- Output: Vite static bundle
- Production app domain: `chat.sanetomore.com`
- Env vars:
  - `VITE_API_BASE_URL`
  - `VITE_GOOGLE_CLIENT_ID`
  - `VITE_APP_ENV`

## Backend (Cloud Run)

- Containerized Go API
- Public HTTPS endpoint with CORS restricted to allowed frontend origins
- Recommended backend API domain: `api.chat.sanetomore.com`
- Auth-related env vars can be deferred until the final auth rollout phase.
- Env vars:
  - `OPENROUTER_API_KEY`
  - `BRAVE_API_KEY`
  - `TURSO_DATABASE_URL`
  - `TURSO_AUTH_TOKEN`
  - `GOOGLE_CLIENT_ID`
  - `ALLOWED_GOOGLE_EMAILS` (comma-separated)
  - `SESSION_COOKIE_DOMAIN`
  - `SESSION_COOKIE_SECURE=true`
  - `SESSION_TTL_HOURS=168`
  - `OPENROUTER_FREE_TIER_DEFAULT_MODEL=openrouter/free`
  - `DEFAULT_CHAT_REASONING_EFFORT=medium`
  - `DEFAULT_DEEP_RESEARCH_REASONING_EFFORT=high`
  - `CORS_ALLOWED_ORIGINS`
  - `LOCAL_UPLOAD_DIR`
  - `APP_ENV`

## Domains and Local Development

- Production frontend: `https://chat.sanetomore.com`
- Production backend: `https://api.chat.sanetomore.com` (recommended)
- Local development is supported with localhost:
  - frontend: `http://localhost:5173` (Vite default)
  - backend: `http://localhost:8080` (recommended)
- Example `CORS_ALLOWED_ORIGINS`:
  - `https://chat.sanetomore.com,http://localhost:5173`
- Recommended `SESSION_COOKIE_DOMAIN`:
  - `.sanetomore.com` in production
  - unset in localhost development

## Turso

- Separate DBs per environment or branch-based DB strategy
- Schema SQL and versioned SQL changes tracked in `/db`
- Daily backup policy

## Attachment Storage (MVP and Future)

- MVP: local processing path configured via `LOCAL_UPLOAD_DIR` (ephemeral on Cloud Run) plus extracted text in Turso
- Future: GCS bucket for durable object storage and signed URL flows
- Migration path from local->GCS documented before multi-user scale
- Recommended `LOCAL_UPLOAD_DIR` default: `/tmp/chat-uploads`

## CI/CD Outline (Deferred)

1. On PR:
   - lint
   - unit tests
   - build checks
   - chat/attachments integration tests
   - reasoning-capability + preset API tests
2. On merge to main:
   - deploy backend to Cloud Run
   - deploy frontend to Vercel
   - apply DB SQL change scripts if schema updates are needed
3. Final auth rollout gate:
   - enable auth env vars and `AUTH_REQUIRED=true`
   - run auth flow integration tests

## Acceptance Criteria

- One-command deploy path per environment
- Secrets only from platform secret stores
- Rollback path documented for frontend and backend
- Final rollout gate: auth config supports the two initial allowed emails and future expansion without code changes
- Reasoning default env vars are configured and consistent across `dev`, `staging`, and `prod`
