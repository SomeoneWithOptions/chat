# AGENTS.md

Implementation guidance for this repository. Keep this file stable and high-signal.

## Purpose

- Ensure consistent implementation decisions across contributors and agent runs.
- Capture non-negotiable technical and product rules.
- Keep sprint/task details in `/plans`, not here.

## Repository Layout

- `/frontend`: React + Vite app (dark-mode-only UI).
- `/backend`: Go API (auth, chat orchestration, search, streaming).
- `/db`: Turso schema SQL and versioned SQL change scripts.
- `/infra`: deployment/runtime docs and config notes.
- `/docs`: long-lived architecture and operational docs.
- `/scripts`: local developer helpers.
- `/plans`: implementation plans and roadmap.

## Stack and Runtime

- Frontend: React + Vite + TypeScript.
- Package manager/runtime: Bun.
- Backend: Go 1.22+.
- Database: Turso (LibSQL/SQLite).
- LLM gateway: OpenRouter.
- Web grounding: Brave Data for AI.
- Frontend domain: `https://chat.sanetomore.com`.
- Backend domain: `https://api.chat.sanetomore.com`.

## Product Invariants

- Google authentication is required before app access.
- Email allowlist is config-driven and extensible.
- Initial allowlist:
  - `acastesol@gmail.com`
  - `obzen.black@gmail.com`
- Session TTL: 7 days.
- Grounding defaults to ON for every message.
- Deep research timeout target: 120 seconds.
- Deep research model is user-selectable.
- Default model behavior:
  - normal chat uses last-used model
  - first-run fallback is `openrouter/free`
  - deep research defaults to last-used normal model

## Model UX Rules

- Curated model list may be empty initially.
- UI must support "show all models".
- UI must support favorites.
- Show model pricing and context window in UI.

## File and Data Rules

- Supported attachment types (MVP):
  - `.txt`, `.md`, `.pdf`, `.csv`, `.json`
- Max file size: 25 MB.
- Local attachment processing path for MVP: `LOCAL_UPLOAD_DIR` (Cloud Run default `/tmp/chat-uploads`).
- GCS is optional for later durable blob storage.

## Deletion Semantics

- User can delete one chat or all chats.
- Chat deletes are hard deletes in DB (no soft-delete behavior).
- Deleting chat data removes dependent rows (messages, citations, files metadata).
- If attachments are stored in GCS, delete corresponding objects.
- Do not delete original local user files from their machine.

## API and Streaming Contract

- Use OpenAPI 3.1 as the canonical API contract.
- Use SSE (`text/event-stream`) for token streaming responses.
- Keep endpoint behavior and contract docs in sync.

## Security and Logging

- Never log API keys or sensitive token values.
- Session cookies must be HTTP-only, secure, and same-site constrained.
- Verify Google ID tokens server-side on login.
- Restrict CORS to approved origins (prod + localhost dev origins).

## Delivery Standards

- Keep changes scoped to the relevant folder ownership.
- Prefer simple, explicit implementations over framework-heavy abstractions.
- Keep planning and docs synchronized with code changes:
  - update `/plans` when scope, architecture, or implementation order changes
  - update `/docs` when behavior, operations, or runbooks change
  - update `AGENTS.md` when repo rules/invariants/tooling standards change
  - include these doc updates in the same PR/commit as the code change when possible
- Add/adjust tests for new logic and regressions.
- Do not introduce breaking behavior without documenting it.
