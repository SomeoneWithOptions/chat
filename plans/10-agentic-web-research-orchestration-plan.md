# Agentic Web Research Orchestration Plan (Chat + Deep Research)

## Goal

Implement an agentic research loop for both `chat` and `deep_research` paths where the system:

1. understands the user request,
2. plans what to search,
3. searches for candidate sources,
4. reads source content (not only snippets),
5. decides whether evidence is sufficient,
6. repeats as needed within budget,
7. returns a grounded final answer with citations.

Deep research should run with more loops, more source reads, and more reasoning time than normal chat.

Execution details and implementation checklist live in `plans/11-agentic-web-research-execution-checklist.md`.

## Current Baseline (What Exists Today)

- Normal chat grounding:
  - one-pass Brave search + snippet citations in `backend/internal/httpapi/handler.go`
  - no iterative sufficiency loop
- Deep research:
  - multi-pass query heuristics in `backend/internal/research/runner.go`
  - single synthesis call after search in `backend/internal/httpapi/deep_research.go`
- Frontend:
  - research timeline supports phases `planning`, `searching`, `synthesizing`, `finalizing`
  - consumes SSE events from `POST /v1/chat/messages`

## Target Behavior

### Shared Orchestration Contract

For both modes, run a loop:

1. **Plan**: identify missing facts, define search intents.
2. **Search**: execute web queries and collect candidate URLs.
3. **Read**: fetch/extract source content from top candidates.
4. **Evaluate**: decide if evidence is sufficient and trustworthy.
5. **Loop or finalize**: if not sufficient, generate refined queries and continue.

### Mode Profiles

- **Chat profile (fast)**
  - fewer loops and source reads
  - aims for low latency while still agentic
  - conservative fallback to current single-pass behavior on errors
- **Deep research profile (thorough)**
  - more loops and more sources read
  - stronger evidence requirements and contradiction checks
  - must still complete/fail gracefully within 150s timeout target

## Implementation Design

## 1) New Research Orchestrator Package

Create/expand `backend/internal/research` into an orchestration layer.

Proposed files:

- `orchestrator.go`: core loop state machine
- `profiles.go`: mode profiles and default budgets
- `planner.go`: LLM structured planning/evaluation logic
- `searcher.go`: search adapter abstraction (Brave-backed)
- `reader.go`: source fetch + extraction pipeline
- `evidence.go`: evidence store, scoring, dedupe, confidence tracking
- `types.go`: shared structs for steps/results/events

Core interfaces:

- `Planner` (LLM-based): returns strict structured actions
- `Searcher`: returns candidate URLs (Brave)
- `Reader`: fetches URL and returns extracted text + metadata

Core result object should include:

- loop count and per-step stats
- final evidence set and ranked citations
- warning list (partial failures)
- reason for stop (`sufficient`, `budget_exhausted`, `timeout`, `error`)

## 2) Structured Planner/Evaluator Prompts

Add planner outputs as strict JSON (schema validated in Go):

- `nextAction`: `search_more | finalize`
- `queries`: list of refined queries
- `coverageGaps`: unresolved facts/questions
- `targetSourceTypes`: optional hints (`official docs`, `gov`, `news`, `standards`)
- `confidence`: float 0-1
- `reason`: short explanation for decision

Planner must be used at least:

- initial planning
- after each read batch for sufficiency check

Fallback behavior if planner output is invalid/unavailable:

- fallback to heuristic query builder (existing logic)
- continue with safe bounded loop

## 3) Source Reading Pipeline (Critical)

Implement source fetch/extract pipeline used by both modes.

Security requirements:

- allow only `http`/`https`
- block local/private/link-local/loopback IP ranges (SSRF guard)
- reject non-standard ports by policy or strict allowlist
- cap redirects and body size
- per-request timeout and total read budget

Extraction requirements:

- support at least:
  - `text/html` (main content extraction)
  - `text/plain`, `text/markdown`, `application/json`, `text/csv`
  - `application/pdf` (reuse PDF extraction approach)
- normalize whitespace and trim aggressively
- keep bounded excerpt windows for prompt injection safety and token control
- store source metadata: final URL, title, content type, fetch status, fetched_at

## 4) Evidence Model and Ranking

Maintain an in-memory evidence pool per request:

- dedupe by canonical URL
- keep best available text/snippet per source
- compute evidence quality score from:
  - query relevance,
  - source quality (official domains/docs),
  - recency signals,
  - content completeness,
  - cross-source corroboration,
  - contradiction flags.

For final synthesis:

- pass only top-N evidence items per mode budget
- preserve stable citation IDs `[1]...[N]`
- reorder persisted citations to match claim order when possible (existing behavior)

## 5) Handler Integration (`/v1/chat/messages`)

Replace direct grounding/deep-runner calls with orchestrator invocation.

In `backend/internal/httpapi/handler.go` and `backend/internal/httpapi/deep_research.go`:

- route both modes through new orchestrator with different profiles
- keep current message persistence and SSE framing
- preserve graceful warning behavior (non-fatal search/read failures)
- keep deep research timeout as hard request budget

Normal chat path:

- keep response streaming UX
- if orchestrator exceeds chat budget, finalize with available evidence or fallback quickly

Deep research path:

- allow larger evidence set and more loops
- preserve existing deep-research structured answer requirement

## 6) SSE and OpenAPI Contract Updates

Update `backend/openapi/openapi.yaml` and frontend event types.

Option A (minimal change):

- keep phase enum unchanged
- encode loop/read/eval details in `progress.message`

Option B (recommended):

- extend `StreamEventProgress.phase` with:
  - `reading`
  - `evaluating`
  - `iterating`
- include optional fields:
  - `loop`
  - `maxLoops`
  - `sourcesConsidered`
  - `sourcesRead`

Frontend updates required in:

- `frontend/src/lib/api.ts` (event types)
- `frontend/src/App.tsx` (timeline rendering)
- tests in `frontend/src/App.test.tsx`

## 7) Configuration and Defaults

Add config for mode-specific budgets.

Suggested env vars (with safe defaults):

- `AGENTIC_RESEARCH_CHAT_ENABLED=true`
- `AGENTIC_RESEARCH_DEEP_ENABLED=true`
- `CHAT_RESEARCH_MAX_LOOPS=2`
- `CHAT_RESEARCH_MAX_SOURCES_READ=4`
- `CHAT_RESEARCH_MAX_SEARCH_QUERIES=4`
- `CHAT_RESEARCH_TIMEOUT_SECONDS=20`
- `DEEP_RESEARCH_MAX_LOOPS=6`
- `DEEP_RESEARCH_MAX_SOURCES_READ=16`
- `DEEP_RESEARCH_MAX_SEARCH_QUERIES=18`
- `DEEP_RESEARCH_TIMEOUT_SECONDS=150` (existing)
- `RESEARCH_SOURCE_FETCH_TIMEOUT_SECONDS=12`
- `RESEARCH_SOURCE_MAX_BYTES=1500000`
- `RESEARCH_MAX_CITATIONS_CHAT=8`
- `RESEARCH_MAX_CITATIONS_DEEP=12`

Update:

- `backend/internal/config/config.go`
- `backend/.env.example`
- `backend/README.md`
- `docs/local-development.md`

## 8) Prompting Strategy

Create explicit prompt templates for:

- planning (`what to search`)
- sufficiency evaluation (`is evidence enough`)
- final synthesis (`answer with citation discipline`)

Prompt requirements:

- never claim facts without supporting evidence IDs
- explicitly surface uncertainty and conflicts
- enforce recency caution for time-sensitive requests
- disallow fabricated citations and unknown claims

## 9) Observability and Safety

Add per-request metrics/log fields:

- loop count
- search query count
- source fetch attempts/success/failure by status
- evidence count before/after ranking
- stop reason
- latency per stage and total

Security/logging guardrails:

- never log full extracted page text
- never log API keys/tokens
- include only bounded debug snippets when needed

## 10) Testing Plan

### Unit Tests

- orchestrator loop termination and budget enforcement
- planner JSON parsing/validation + fallback path
- source reader SSRF protections and content-type handling
- evidence ranking, dedupe, contradiction handling

### Integration Tests (Backend)

- normal chat runs at least one plan/search/read/evaluate cycle
- deep research executes multi-loop cycle and emits ordered progress
- partial source-read failures still yield answer + warning
- timeout behavior remains bounded and user-visible
- citations persist and stream correctly

### Frontend Tests

- timeline renders new phases/loop metadata
- warning/error handling remains stable
- citations and usage ordering still correct

## 11) Delivery Phases

### Phase A: Core Engine (Backend only)

1. build orchestrator + profiles
2. wire existing Brave search and heuristic fallback
3. keep current SSE phase enum (minimal UI impact)

### Phase B: Source Reading + Sufficiency Loop

1. add reader pipeline and SSRF guards
2. enable plan/search/read/evaluate iterative loop
3. tune budgets for chat vs deep

### Phase C: Contract + UI Enhancements

1. extend progress event schema (optional but recommended)
2. update frontend timeline and tests
3. update OpenAPI and docs

### Phase D: Hardening

1. telemetry and performance tuning
2. source quality/ranking refinements
3. regression and load testing

## 12) Acceptance Criteria

- Both `chat` and `deep_research` use iterative agentic research loops.
- Deep research performs more loops/source reads than chat by config and in observed telemetry.
- System can fetch and read real source content, not only search snippets.
- Responses include citations for factual claims and handle uncertainty explicitly.
- Partial failures degrade gracefully with warnings; no hard failure unless unrecoverable.
- Normal chat remains responsive under configured chat budget.
- Deep research completes/fails gracefully within `DEEP_RESEARCH_TIMEOUT_SECONDS`.

## 13) Risks and Mitigations

- **Latency regression in normal chat**
  - Mitigation: strict chat budgets + fast fallback path.
- **SSRF/security exposure in URL reading**
  - Mitigation: network guards, scheme restrictions, size/time caps.
- **Planner JSON instability**
  - Mitigation: strict schema validation + deterministic fallback.
- **Token/cost increase**
  - Mitigation: bounded evidence windows, top-N filtering, profile budgets.
- **Noisy/low-quality web pages**
  - Mitigation: extraction quality checks + source ranking + contradiction detection.

## 14) Migration Notes

- Keep existing `resolveGroundingContext` and current deep runner code path behind fallback until parity is validated.
- Introduce feature flags for controlled rollout and quick rollback:
  - `AGENTIC_RESEARCH_CHAT_ENABLED`
  - `AGENTIC_RESEARCH_DEEP_ENABLED`
- Once stable, retire old heuristic-only path and simplify duplicated logic.

## 15) Out of Scope for This Plan

- Asynchronous research job queue UX.
- Multi-provider search federation beyond Brave.
- User-configurable research budgets in UI.
- Auth changes (auth remains final rollout gate per repository rules).
