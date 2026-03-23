# AGENTS.md

## Stack and Runtime

- Package manager/runtime: Bun.
- Database: Turso (LibSQL/SQLite).
- Database name: `chat-prod` (production). We use the production database for local development.
- Local DB operations: Use `turso` CLI (already authenticated on this machine).
- GCP operations: Use the local `gcloud` CLI on the project chat-486915 (already authenticated) to look up information, create resources, and manage deployments on GCP infrastructure.
- Frontend domain: `https://chat.sanetomore.com`.
- Backend domain: `https://api.chat.sanetomore.com`.

## Delivery Standards

- Keep changes scoped to the relevant folder ownership.
- Use the frontend-design skill at `/SKILL.md` for any frontend work.
- Prefer simple, explicit implementations over framework-heavy abstractions.
- Keep planning and docs synchronized with code changes:
  - update `/plans` when scope, architecture, or implementation order changes
  - update `/docs` when behavior, operations, or runbooks change
  - update `AGENTS.md` when repo rules/invariants/tooling standards change
  - include these doc updates in the same PR/commit as the code change when possible
- Add/adjust tests for new logic and regressions.


dont deploy the frontend and backend yourself, tell the user to do so, you are free to run migrations and operations on the database for deployments