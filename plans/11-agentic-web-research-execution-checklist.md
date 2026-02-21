# Agentic Web Research Execution Checklist

This is the implementation playbook for `plans/10-agentic-web-research-orchestration-plan.md`.

It defines:

- implementation order,
- concrete checklist items,
- "what good looks like" per step,
- required tests and verification.

## Scope and Non-Negotiables

- Applies to both response modes: `chat` and `deep_research`.
- `deep_research` must be more thorough by configuration and observed telemetry.
- Preserve existing product invariants (grounding default ON, deep research timeout target 150s, auth rollout sequencing unchanged).
- Keep fallback path available until parity is proven.

## Milestone Order

1. **M1 - Contracts and skeletons**
2. **M2 - Orchestrator core loop**
3. **M3 - Planner and sufficiency decisions**
4. **M4 - Source reader pipeline with security controls**
5. **M5 - Evidence ranking and synthesis wiring**
6. **M6 - Handler integration for chat + deep research**
7. **M7 - SSE/OpenAPI/frontend updates**
8. **M8 - Hardening, metrics, and rollout flags**

Complete each milestone before moving to the next.

## M1 - Contracts and skeletons

### Checklist

- [ ] Add research orchestrator types and interfaces in `backend/internal/research/types.go`.
- [ ] Add mode profiles in `backend/internal/research/profiles.go`.
- [ ] Add orchestrator shell in `backend/internal/research/orchestrator.go`.
- [ ] Add config fields and env parsing in `backend/internal/config/config.go`.
- [ ] Add env docs in `backend/.env.example`, `backend/README.md`, and `docs/local-development.md`.

### Files to touch

- `backend/internal/research/types.go` (new)
- `backend/internal/research/profiles.go` (new)
- `backend/internal/research/orchestrator.go` (new)
- `backend/internal/config/config.go`
- `backend/.env.example`
- `backend/README.md`
- `docs/local-development.md`

### What good looks like

- A single shared config shape exists for chat and deep profiles.
- Budget fields are explicit (`maxLoops`, `maxSourcesRead`, `maxQueries`, timeout, citation limits).
- No behavior changes yet in handlers.

### Tests

- Unit tests for config defaults and bounds validation in `backend/internal/config/config_test.go`.
- Unit tests for profile resolution in `backend/internal/research/profiles_test.go`.

## M2 - Orchestrator core loop

### Checklist

- [ ] Implement loop state machine: `plan -> search -> read -> evaluate -> loop/finalize` in `orchestrator.go`.
- [ ] Enforce hard budgets for loops, queries, reads, and elapsed time.
- [ ] Add stop reasons (`sufficient`, `budget_exhausted`, `timeout`, `error`).
- [ ] Emit internal progress events with loop metadata.
- [ ] Keep existing `runner.go` available for fallback.

### Files to touch

- `backend/internal/research/orchestrator.go`
- `backend/internal/research/types.go`
- `backend/internal/research/runner.go` (fallback bridging only, no removal)

### What good looks like

- Orchestrator can run with stubbed Planner/Searcher/Reader and produce deterministic outcomes.
- Every run exits with a defined stop reason.
- Budget limits are honored even when dependencies misbehave.

### Tests

- `backend/internal/research/orchestrator_test.go` (new):
  - loop terminates at `maxLoops`
  - query/read caps are enforced
  - timeout exits with `timeout`
  - errors convert to warnings when recoverable

## M3 - Planner and sufficiency decisions

### Checklist

- [ ] Add planner module (`planner.go`) using strict JSON outputs.
- [ ] Validate planner JSON schema in Go before use.
- [ ] Add heuristic fallback when planner output is invalid/empty.
- [ ] Add prompt templates for initial planning and sufficiency check.
- [ ] Ensure planner works with both mode profiles.

### Files to touch

- `backend/internal/research/planner.go` (new)
- `backend/internal/research/prompts.go` (new)
- `backend/internal/research/types.go`

### What good looks like

- Invalid planner payloads never crash the request.
- Planner can ask for more evidence and provide refined queries.
- Deep profile tends to request additional passes more often than chat profile under the same prompt.

### Tests

- `backend/internal/research/planner_test.go` (new):
  - valid JSON path
  - malformed JSON fallback path
  - empty query fallback path
  - `finalize` action honored

## M4 - Source reader pipeline with security controls

### Checklist

- [ ] Add source reader (`reader.go`) for URL fetch + extraction.
- [ ] Enforce URL/scheme checks and SSRF protections.
- [ ] Add redirect, size, and timeout limits.
- [ ] Extract content for `html`, `plain`, `markdown`, `json`, `csv`, `pdf`.
- [ ] Normalize and truncate extracted text to bounded windows.

### Files to touch

- `backend/internal/research/reader.go` (new)
- `backend/internal/research/url_security.go` (new)
- `backend/internal/research/extract.go` (new)
- `backend/internal/httpapi/files.go` (optional: share extraction helpers)

### What good looks like

- Reader rejects internal/private network targets.
- Reader survives noisy pages and unsupported content types gracefully.
- Fetch failures are returned as structured warnings, not panics.

### Tests

- `backend/internal/research/reader_test.go` (new):
  - scheme allow/deny
  - local/private IP block tests
  - body size cap behavior
  - timeout behavior
  - extraction smoke tests per content type

## M5 - Evidence ranking and synthesis wiring

### Checklist

- [ ] Implement evidence pool (`evidence.go`) with canonical URL dedupe.
- [ ] Add scoring heuristics (relevance, recency, source quality, corroboration).
- [ ] Add contradiction flags for conflicting claims.
- [ ] Build synthesis input from top-ranked evidence by profile budget.
- [ ] Preserve stable citation indexing and claim-order post-processing.

### Files to touch

- `backend/internal/research/evidence.go` (new)
- `backend/internal/httpapi/deep_research.go`
- `backend/internal/httpapi/handler.go`

### What good looks like

- Duplicate URLs collapse to one citation source.
- Low-value/no-content sources are down-ranked or removed.
- Final answer includes citations aligned with evidence list.

### Tests

- `backend/internal/research/evidence_test.go` (new):
  - dedupe
  - ranking stability
  - contradiction detection basics
- Extend `backend/internal/httpapi/handler_conversations_test.go`:
  - persisted citations remain ordered by claims where possible

## M6 - Handler integration for chat + deep research

### Checklist

- [ ] Inject orchestrator into handler construction.
- [ ] Route normal chat grounding path through orchestrator (chat profile).
- [ ] Route deep research path through orchestrator (deep profile).
- [ ] Keep fallback to old logic behind feature flags.
- [ ] Preserve persistence contract for messages, citations, warnings, and usage.

### Files to touch

- `backend/internal/httpapi/handler.go`
- `backend/internal/httpapi/deep_research.go`
- `backend/internal/httpapi/router.go`
- `backend/internal/httpapi/handler_conversations_test.go`

### What good looks like

- `/v1/chat/messages` behavior remains API-compatible.
- Chat path is still responsive and does not inherit deep-research latency profile.
- Deep research uses higher budgets and more loops than chat.

### Tests

- Update/extend integration tests in `backend/internal/httpapi/handler_conversations_test.go`:
  - chat emits progress and citations with iterative pipeline
  - deep research emits multi-loop progress in order
  - partial failures produce warning + final output
  - timeout still emits error + done

## M7 - SSE/OpenAPI/frontend updates

### Checklist

- [ ] Update `backend/openapi/openapi.yaml` progress schema.
- [ ] Update frontend stream event types.
- [ ] Update research timeline UI for new phases/metadata.
- [ ] Ensure compatibility if backend sends old phase set during rollout.

### Files to touch

- `backend/openapi/openapi.yaml`
- `frontend/src/lib/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`

### What good looks like

- Frontend renders loop/read/evaluate activity cleanly.
- No regressions for existing warning/citation rendering.
- OpenAPI reflects actual wire payloads.

### Tests

- Frontend unit tests for stream event handling.
- API contract spot checks against sample SSE payloads.

## M8 - Hardening, metrics, and rollout flags

### Checklist

- [ ] Add structured metrics/logging for loop and fetch behavior.
- [ ] Add feature flags for chat/deep orchestrator enablement.
- [ ] Add rollback plan and default-safe values.
- [ ] Run performance and reliability sweeps.
- [ ] Remove dead code only after parity is confirmed.

### Files to touch

- `backend/internal/httpapi/handler.go`
- `backend/internal/httpapi/deep_research.go`
- `backend/internal/config/config.go`
- `docs/local-development.md`
- `plans/08-testing-security-observability-plan.md` (if observability details change)

### What good looks like

- Operators can disable chat or deep agentic loops independently.
- Logs provide enough detail to debug failures without leaking sensitive data.
- System behavior under failure is predictable and bounded.

### Tests

- Integration tests for feature-flag toggles.
- End-to-end smoke checks with flags on and off.

## Cross-Cutting Definition of Done

All items below must be true before full rollout:

- [ ] Chat and deep research both use iterative research when respective flags are enabled.
- [ ] Deep research demonstrates higher loop/read counts in logs or test fixtures.
- [ ] Citation persistence and streaming are correct.
- [ ] Timeout/cancellation behavior remains bounded.
- [ ] SSRF and fetch safety tests pass.
- [ ] OpenAPI, docs, and plan files are updated together.
- [ ] Existing tests pass plus new coverage for orchestrator, planner, reader, and evidence components.

## Suggested PR Breakdown

1. **PR-1:** M1 config/contracts skeletons + tests
2. **PR-2:** M2 orchestrator core + tests
3. **PR-3:** M3 planner + prompt templates + tests
4. **PR-4:** M4 reader security/extraction + tests
5. **PR-5:** M5 evidence ranking + synthesis wiring + tests
6. **PR-6:** M6 backend handler integration + integration tests
7. **PR-7:** M7 OpenAPI + frontend updates + frontend tests
8. **PR-8:** M8 hardening/metrics/flags + rollout docs

## Verification Command Set

Run before each merge:

- Backend tests: `go test ./...` (from `backend`)
- Frontend tests: `bun test` (from `frontend`)
- Frontend build: `bun run build` (from `frontend`)

Run before enabling flags in production:

- End-to-end chat smoke with flags ON/OFF
- End-to-end deep research smoke with flags ON/OFF
- Timeout and cancellation smoke tests
