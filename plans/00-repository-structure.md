# Repository Structure Plan

## Target Monorepo Layout

```text
/frontend   # React + Vite app
/backend    # Go API service
/db         # schema SQL, versioned SQL change scripts, Turso notes
/infra      # deployment configs and infrastructure docs
/docs       # product/technical docs beyond implementation plans
/scripts    # local dev, CI helper scripts
/plans      # planning files (this folder)
```

## Why These Folders

- `/frontend`: isolate UI build/tooling and deployment concerns.
- `/backend`: isolate API code, integrations, streaming logic, and final-rollout auth.
- `/db`: keep schema and SQL evolution changes versioned and reviewable.
- `/infra`: separate runtime/deploy config from app code.
- `/docs`: keep stable docs separate from active implementation plans.
- `/scripts`: keep repeatable automation commands in one place.

## Immediate Scaffold (Planning Phase)

1. Create `/frontend`, `/backend`, `/db`.
2. Create optional support folders (`/infra`, `/docs`, `/scripts`).
3. Add README placeholders in each folder with intended ownership.

## Ownership and Boundaries

- Backend owns OpenRouter, Brave, DB writes, and final-rollout auth verification.
- Frontend owns UX, model/mode controls, and streaming rendering.
- DB folder is the single source of truth for schema/SQL changes.
