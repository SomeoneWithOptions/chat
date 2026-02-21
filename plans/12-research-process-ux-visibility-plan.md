# Research Process UX Visibility Plan

## Goal

Improve UX by showing users concise, live process updates while the model researches and prepares a response.

Primary requirement from product:

- Show **up to 2 lines of text** about what the LLM is about to do / currently doing.
- For simple steps (for example: quick grounding/search fetch), show a **short one-line update**.

## Why this is needed

Current UX gap:

- In normal chat mode, users mostly see a long "thinking" state until answer tokens stream.
- Deep research has richer progress, but does not yet communicate concise user-facing intent for each step.

Desired outcome:

- Users understand progress, trust the system more, and feel less waiting friction.
- Messaging remains concise and readable, not verbose technical logs.

## Scope

In scope:

- Progress UX for both `chat` and `deep_research`.
- New user-facing progress copy generated from safe summaries.
- Two-line max display behavior with short variants for quick steps.

Out of scope:

- Exposing chain-of-thought or raw planner internals.
- Showing raw fetched page text or full query list dumps.

## UX Rules (Non-Negotiable)

1. Progress text length:
- Normal: max **2 short lines**.
- Quick/simple steps: **1 short line**.

2. Tone:
- Plain language, action-oriented, no internal jargon.
- Avoid uncertainty theater; be concrete and brief.

3. Safety:
- Never expose internal reasoning traces.
- Never leak secrets, API keys, or full source content.

4. Stability:
- Frontend must render gracefully if backend sends old progress payloads.

## Proposed UX Model

### A) Progress Strip (always visible while running)

For both chat and deep research:

- Line 1: current step label + short action.
- Line 2 (optional): quick detail such as loop/counter.

Examples:

- `Planning next search`  
  `Checking what evidence is still missing`

- `Searching trusted sources`

- `Reading top 3 pages`  
  `2 of 3 processed`

- `Evidence looks conflicting`  
  `Running one more pass`

### B) Expandable Detail Panel

- Keep timeline/phases for deep research.
- Add same panel for chat when grounding is on.
- Show compact counters:
  - loop `x/y`
  - sources considered/read
  - stop reason (if ended early)

## Backend Implementation Plan

## 1) Add user-facing progress summary fields to SSE

Update progress event payload to include optional display fields:

- `title` (short headline)
- `detail` (optional second line)
- `isQuickStep` (bool; frontend may show one-line style)
- `decision` (enum, optional): `search_more|finalize|fallback`

Files:

- `backend/internal/research/types.go`
- `backend/internal/research/orchestrator.go`
- `backend/internal/httpapi/handler.go`
- `backend/internal/httpapi/deep_research.go`
- `backend/openapi/openapi.yaml`

## 2) Emit meaningful summaries per phase

Map internal phases to concise user copy:

- `planning`: "Planning next step"
- `searching`: "Searching trusted sources"
- `reading`: "Reading selected sources"
- `evaluating`: "Checking evidence coverage"
- `iterating`: "Running another pass"
- `synthesizing`: "Drafting answer"
- `finalizing`: "Finalizing citations"

For quick phases (e.g. single grounding fetch), emit one-liner only.

## 3) Stream progress earlier in normal chat

Ensure chat sends progress SSE before final token stream starts, so users see active movement during pre-answer research.

## 4) Add summary builder helper

Create a backend helper that converts internal step state to safe display text:

- Inputs: phase, loop counters, source counters, decision signal, warning signal.
- Output: `title`, `detail`, `isQuickStep`.

No LLM-generated free-form prose for this layer; deterministic templates only.

## Frontend Implementation Plan

## 1) Update stream event types

Add optional fields to progress event type:

- `title?: string`
- `detail?: string`
- `isQuickStep?: boolean`
- `decision?: 'search_more' | 'finalize' | 'fallback'`

File:

- `frontend/src/lib/api.ts`

## 2) Render two-line progress text

In App timeline/progress strip:

- Prefer backend-provided `title` and `detail`.
- Enforce max two lines via CSS clamp.
- If `isQuickStep=true` or no detail, show single-line compact state.

File:

- `frontend/src/App.tsx`

## 3) Show progress for normal chat too

Do not gate progress UI only by deep research.

Behavior:

- If chat + grounding enabled and progress events exist, show compact strip.
- Keep deep research panel richer/expandable.

## 4) Visual design details

- Micro transitions between steps (fast fade/slide, 120-180ms).
- Avoid layout shift; fixed min height for strip.
- Maintain existing dark design system and style consistency.

## Copy Specification (Initial)

Use these as defaults in backend templates:

- Planning:
  - Title: `Planning next step`
  - Detail: `Checking what evidence is still missing`

- Searching:
  - Quick title: `Getting grounding results`
  - Non-quick detail: `Searching trusted sources for corroboration`

- Reading:
  - Title: `Reading selected sources`
  - Detail: `Using top-ranked pages to improve accuracy`

- Evaluating:
  - Title: `Checking evidence quality`
  - Detail: `Deciding whether we can answer confidently`

- Iterating:
  - Title: `Running another pass`
  - Detail: `Need one more search to close gaps`

- Synthesizing:
  - Title: `Drafting response`
  - Detail: `Grounding claims to collected sources`

- Finalizing:
  - Title: `Finalizing answer`
  - Detail: `Ordering citations and sending response`

## Telemetry and Debugging

Add structured logs for UX events (without sensitive content):

- `phase`, `loop`, `maxLoops`
- `sourcesRead`, `sourcesConsidered`
- `title`, `hasDetail`, `isQuickStep`

Purpose:

- Diagnose stuck states and tune copy quality.

## Testing Plan

Backend tests:

- Progress payload includes `title/detail/isQuickStep` where expected.
- Quick steps produce one-line payload.
- Backward-compatible behavior when new fields absent.

Frontend tests:

- Renders one-line quick state.
- Renders two-line state and clamps length.
- Chat mode shows compact progress strip when progress arrives.
- Deep research still shows full timeline with new summaries.

Files:

- `backend/internal/httpapi/handler_conversations_test.go`
- `frontend/src/App.test.tsx`

## Rollout Strategy

1. Phase 1 (safe):
- Add fields and frontend support, keep existing messages as fallback.

2. Phase 2:
- Enable deterministic summary templates for all progress events.

3. Phase 3:
- Tune copy and quick-step thresholds using real usage feedback.

## Acceptance Criteria

- Users always see meaningful progress before final answer tokens when research is running.
- Progress text is max two lines; quick steps are one line.
- Chat and deep research both expose process visibility.
- No chain-of-thought leakage.
- Existing SSE consumers remain compatible.
