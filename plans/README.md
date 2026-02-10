# Implementation Plans Index

This folder contains separated planning files so implementation can happen in parallel and in phases.

Recommended execution order:

1. `plans/00-repository-structure.md`
2. `plans/01-product-scope-and-mvp.md`
3. `plans/02-system-architecture.md`
4. `plans/03-frontend-react-vite-plan.md`
5. `plans/04-backend-go-openrouter-plan.md`
6. `plans/05-data-files-storage-plan.md`
7. `plans/06-grounding-and-deep-research-plan.md`
8. `plans/07-infra-deployment-plan.md`
9. `plans/08-testing-security-observability-plan.md`
10. `plans/09-implementation-roadmap.md`

Core stack assumptions used in these plans:

- Frontend: React + Vite, deployed on Vercel
- JS package manager/runtime: Bun
- Backend API: Go, deployed on Cloud Run (GCP)
- Database: Turso (LibSQL/SQLite)
- DB change strategy: schema SQL + versioned SQL scripts in `/db` (no migration framework initially)
- Auth: Google sign-in with configurable email allowlist (initially 2 users), enabled as the final rollout gate
- LLM provider gateway: OpenRouter
- Web grounding: Brave Search API (Data for AI)
- File storage: local processing path in MVP, GCS later for durable storage

Additional implementation scope now included across all plan files:

- Per-model reasoning-effort presets (thinking-level presets) for both `chat` and `deep_research`
- User-selectable reasoning effort on each response with persistence by model + mode
- Model capability sync from OpenRouter (`supported_parameters`) to gate reasoning controls in UI
