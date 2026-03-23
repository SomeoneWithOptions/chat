# 15 - Council Mode Stabilization Plan

## Goal

Ship a council mode that is actually correct in production:

1. council runs execute the council workflow, not the legacy single-agent workflow
2. source models can run sequentially
3. grounded source models aim for at least `15` readable sources each from a single Brave search pass
4. the user sees simple, minimal progress information
5. reloads and polling preserve council state correctly
6. the feature is covered by backend and frontend tests

---

## Recommended Execution Model

For the stabilization pass, use **sequential source-model execution**.

Why:

- it removes the current orchestration ambiguity
- it makes Brave pacing and per-model source accounting much easier to reason about
- it reduces race conditions in persistence and progress updates
- it is slower than fan-out, but much easier to make correct first

This means one council run should execute in this order:

1. initialize run
2. source model 1
3. source model 2
4. source model 3
5. source model 4
6. source model 5
7. fusion analysis
8. final fused result

Parallelism can be reconsidered only after this path is stable and tested.

---

## Product Rules

### Grounding

- Council grounding must respect the request flag.
- Ungrounded council runs must be allowed.
- Grounded source models must target `15` readable sources each.
- A grounded source model is `complete` only if it reaches `15` readable sources.
- A grounded source model is `degraded` if it returns an answer with fewer than `15` readable sources.
- If all grounded source models are `degraded` or `failed`, the final result should clearly warn that evidence quality was below target.

### Search strategy

- Brave requests stay globally serialized per council run.
- Each grounded source model gets exactly one Brave search pass with `count=15`.
- The returned results are read once, then the source model produces exactly one grounded response before the runner moves to the next selected model.
- If fewer than `15` readable sources are recovered from that single pass, the source model should complete as `degraded` rather than retrying with follow-up Brave searches.

### User-facing output

- Keep the council UI minimal by default.
- The user should see:
  - `Sources`
  - `Analysis`
  - `Result`
- Each source row/card should show only:
  - model name
  - status
  - readable source count
  - elapsed time
  - a small warning indicator if degraded or failed
- Full source answers, citations, and reasoning stay behind expansion.
- Analysis should be concise bullets, not large walls of text.
- The main result stays as the primary readable answer.

---

## Fix Order

### Phase 1 - Correct the execution path

Backend changes:

- branch async worker execution by `workflow_type`
- call `executeCouncilRun` for `council_fusion`
- keep legacy `single_model` behavior unchanged

Acceptance criteria:

- a queued council run never enters the legacy role-debate path
- a queued single-model agent run still works exactly as before

### Phase 2 - Make the council runner deterministic

Backend changes:

- change source-model execution from goroutines to a sequential loop
- keep a single mutable council run state object
- persist state after each source-model transition:
  - `queued`
  - `running`
  - `complete`
  - `degraded`
  - `failed`
- persist fusion analysis and final result separately

Acceptance criteria:

- source models finish in the same order the user selected them
- progress is stable and replayable
- no concurrent writes are needed for council state

### Phase 3 - Enforce the 15-readable-source single-pass grounding contract

Backend changes:

- introduce council-specific config values for:
  - target readable sources per model
  - search results per query
  - timeout
- make each grounded source-model pass perform one Brave search, one read pass, and one model response
- mark source runs as `degraded` when they answer below target

Recommended defaults:

- `COUNCIL_TARGET_READABLE_SOURCES_PER_MODEL=15`
- `COUNCIL_SEARCH_RESULTS_PER_QUERY=15`
- `COUNCIL_TIMEOUT_SECONDS=1200`

Acceptance criteria:

- a grounded `complete` source always has `readableSources >= 15`
- a grounded source below target is visibly marked `degraded`
- ungrounded council runs do not require search budget

### Phase 4 - Expose a real public council status payload

Backend changes:

- stop treating `agent_runs` as final-only storage for council mode
- persist public council state into `agent_runs` on every meaningful update
- return one public payload from `GET /v1/agent-runs/{id}` containing:
  - run status
  - source results
  - analysis
  - result
  - warnings
  - completed/degraded/failed counters

Suggested shape:

- `status`
- `sourceResults`
- `analysis`
- `result`
- `warnings`
- `completedSources`
- `degradedSources`
- `failedSources`

Acceptance criteria:

- polling shows partial source progress while the run is still active
- the endpoint remains correct after page refresh

### Phase 5 - Fix persistence and replay

Backend changes:

- extend message response loading to include council fields
- load and return:
  - `agentSources`
  - `agentAnalysis`
  - `agentResultModelId`
  - `agentResultUsage`
  - `agentRunId`
- keep old agent messages backward compatible

Frontend changes:

- restore council sections on conversation reload
- resume polling for active council runs

Acceptance criteria:

- reloading an active council conversation preserves the current council UI
- reloading a completed council conversation still shows `Sources`, `Analysis`, and `Result`

### Phase 6 - Make frontend polling terminate correctly

Frontend changes:

- react to terminal run states from `GET /v1/agent-runs/{id}`
- mark the thinking trace:
  - `done` on completion
  - `stopped` on failure
- stop polling completed or failed runs

Acceptance criteria:

- council runs do not stay visually `running` forever
- polling stops after completion or failure

### Phase 7 - Simplify council UX

Frontend changes:

- keep the council tray simple
- default to collapsed source details
- show minimal status chips and counts first
- avoid duplicate or noisy intermediate text
- keep analysis categories concise and skimmable

Acceptance criteria:

- the user can understand run state from one screen without opening every section
- the final answer remains the dominant element in the assistant message

### Phase 8 - Add missing tests

Backend tests:

- council run dispatches to the council executor
- grounded sequential council run with at least `15` readable sources per source model
- degraded source model below `15`
- partial failure with fusion still succeeding
- all-source failure
- ungrounded council run
- public polling payload updates during execution
- conversation reload returns persisted council fields

Frontend tests:

- composer sends `sourceModels` and `fusionModel`
- polling updates source cards progressively
- polling stops on completion/failure
- reloaded council messages render persisted sections
- minimal council summary renders correctly

Acceptance criteria:

- council behavior is explicitly covered end to end
- no release depends on manual inspection alone

---

## Implementation Notes

- Prefer one council state serializer instead of scattering JSON writes across message and run tables.
- The assistant message should remain the long-term replay source for the conversation UI.
- `agent_runs` should be the live status source for polling.
- If both are stored, define one write helper so the two copies cannot drift.

---

## Release Checklist

Before calling council mode ready:

1. sequential council execution is live behind the normal council request path
2. grounded runs enforce the `15`-source target for `complete`
3. ungrounded runs work
4. polling shows partial updates
5. reload restores council state
6. polling terminates correctly
7. backend council tests exist and pass
8. frontend council tests exist and pass

---

## Recommended Next Implementation Order

1. fix worker dispatch
2. switch council sources to sequential execution
3. persist public council status on every update
4. load council fields in conversation replay
5. fix frontend polling terminal-state handling
6. tighten UX to minimal output
7. add backend tests
8. add frontend tests
