# 14 - Agent Mode Council Fusion Plan

## Goal

Replace the current single-model async agent mode with a council-style multi-model workflow where:

1. the user selects up to 5 source models,
2. each selected source model independently answers the same prompt,
3. the user can inspect each individual source model response,
4. the user selects one fusion model,
5. the fusion model analyzes all source responses across explicit comparison categories,
6. the same fusion model then produces the final fused answer.

The user experience should expose three explicit steps inside the assistant message:

1. `Sources`
2. `Analysis`
3. `Result`

This should feel similar to OpenRouter Labs model fusion, but implemented on top of the existing conversation, persistence, grounding, and async run architecture in this repository.

---

## Product Decisions and Recommended Defaults

These settings and behaviors are the recommended defaults for the first implementation.

### Council composition

- Minimum source models: `1`
- Maximum source models: `5`
- Fusion model: exactly `1`
- Fusion model may also be one of the selected source models
- Council mode remains an async workflow backed by the existing agent run system

### Grounding behavior

- Grounding can be turned on or off for a council run
- If grounding is on, every source model gets its own research pass and produces its own citations
- Every grounded source model should target at least `15 readable web sources`
- "Readable web sources" means successfully fetched and extracted pages, not just Brave snippets
- If a source model cannot reach 15 readable sources within budget, it should complete as `degraded`, not fail automatically

### Brave search strategy

- Brave search requests should be globally serialized across the entire council run
- Minimum spacing between Brave requests: `1100ms`
- Each Brave request should ask for `15` search results
- A grounded source model should continue issuing follow-up Brave requests until it either:
  - reaches 15 readable web sources,
  - hits its per-model query cap,
  - or the run times out

### Recommended council grounding budgets

- `COUNCIL_TARGET_READABLE_SOURCES_PER_MODEL=15`
- `COUNCIL_SEARCH_RESULTS_PER_QUERY=15`
- `COUNCIL_MAX_SEARCH_QUERIES_PER_MODEL=3`
- `COUNCIL_MAX_BRAVE_SEARCHES_IN_FLIGHT=1`
- `COUNCIL_MAX_PAGE_READS_IN_FLIGHT=6`
- `COUNCIL_TIMEOUT_SECONDS=1200`
- `COUNCIL_REQUIRE_AT_LEAST_ONE_SUCCESSFUL_SOURCE_MODEL=true`

### Failure and degradation behavior

- One source model failing should not kill the whole run
- Fusion should proceed if at least one source model completes successfully
- A source model that returns an answer with fewer than 15 readable sources should be marked `degraded`
- If all source models fail, the run should fail and fusion should not start
- The analysis stage should explicitly know which source models were complete, degraded, or failed

### Live UX behavior

- Keep async worker execution
- Add live in-session updates for council runs so the user can watch `Sources -> Analysis -> Result` update without refreshing the conversation
- Use polling for council run status as the recommended first implementation, rather than trying to keep a single browser stream attached to the background worker for the full run lifetime

---

## Important Baseline Notes About the Current Code

Current agent mode is async, but it is still fundamentally single-model.

Today:

- request enters `POST /v1/chat/messages`
- if `mode=agent`, backend queues an async run
- the run performs grounded research
- the same selected model runs a role-based debate
- the same selected model synthesizes the final answer

Important current grounding details:

- Brave search requests for deep research and agent paths are already spaced at about `1100ms`
- this spacing is enforced around Brave search requests, not page reads
- the current Brave client already supports requesting multiple results in one request with the `count` parameter
- current agent mode does not guarantee 15 readable web sources per model because it is not a per-model council workflow and the orchestrator currently optimizes for bounded evidence, not a hard per-model readable-source target

That means the new council mode can and should reuse the existing Brave pacing pattern, but it needs a new orchestration layer to guarantee the council-specific source collection behavior.

---

## Current Baseline Architecture

Current key files:

- backend request entry: `backend/internal/httpapi/handler.go`
- async execution: `backend/internal/httpapi/agent_mode.go`
- research orchestration: `backend/internal/httpapi/research_orchestration.go`
- research loop: `backend/internal/research/orchestrator.go`
- Brave client: `backend/internal/brave/client.go`
- OpenRouter client: `backend/internal/openrouter/client.go`
- frontend app state: `frontend/src/App.tsx`
- frontend API types: `frontend/src/lib/api.ts`
- composer UI: `frontend/src/components/Composer.tsx`
- message rendering: `frontend/src/components/ChatMessage.tsx`
- persistence schema: `db/schema.sql`

Important behaviors to preserve:

- async queued agent runs
- assistant placeholder messages
- conversation persistence
- message citations
- graceful warning handling
- existing auth, model selection, and reasoning preset systems

Important current limitations to remove:

- `agent_runs` is centered around one `model_id`
- frontend state assumes one agent model
- `agent_summaries_json` stores role summaries, not per-model outputs
- the current async worker writes results later, but the browser does not have a dedicated public council run status channel

---

## Scope

## In Scope

- New council configuration for agent mode
- Up to 5 source models
- One fusion model
- Individual source model answers visible to the user
- Structured fusion analysis
- Final fused result from the same fusion model
- Grounded and ungrounded council runs
- Persistence of council state into conversation history
- Live council progress updates in the chat UI
- Backward-compatible rendering of old agent messages
- Tests for council success, degradation, and failure paths

## Out of Scope

- Exposing raw chain-of-thought
- Arbitrary tool-use agent swarms beyond the existing research and completion flow
- Changing normal chat or deep research semantics beyond required shared code reuse
- Deploying frontend or backend as part of this planning work

---

## Terminology

To avoid confusion, this plan uses the following terms consistently:

- `source model`: one of the user-selected LLMs that independently answers the prompt
- `fusion model`: the model that performs structured comparison and then writes the final answer
- `readable web source`: a successfully fetched and extracted web page or document used for grounding
- `source card`: the UI card for one source model in Step 1/3
- `council run`: the full async workflow for one council-mode assistant response

---

## UX Requirements

## Step 1/3 - Sources

The first section shows all selected source models.

Each source card should display:

- model name
- run status: `queued`, `running`, `complete`, `degraded`, or `failed`
- token count
- elapsed time
- grounding status
- readable source count
- whether the target of 15 readable sources was reached

Each source card should be expandable and show:

- the full model response
- the model's citations
- model-specific warnings
- model-specific usage
- optional model reasoning text behind explicit user expansion

Recommended UX behavior:

- source cards update independently as source models finish
- users can open a completed source card while other source models are still running
- failed or degraded cards remain visible; they are not hidden or collapsed away
- source cards should be sorted in the same order the user selected the models

## Step 2/3 - Analysis

The second section appears once fusion analysis begins.

It must show five categories:

1. `Agreement`
2. `Key Differences`
3. `Partial Coverage`
4. `Unique Insights`
5. `Blind Spots`

Recommended rendering behavior:

- each category is an expandable section
- content is bullet-based and concise
- source-model badges should be shown where useful
- `Key Differences` should support grouped topics with per-model positions
- `Blind Spots` should focus on what the overall council still missed relative to the user prompt

## Step 3/3 - Result

The third section shows the final fused answer.

It should display:

- the fusion model name
- a visible `Fused` indicator
- the final answer content
- final citations
- optional usage info
- optional reasoning text behind explicit expansion

Recommended behavior:

- Step 3 is the primary answer surface
- Steps 1 and 2 remain accessible after completion
- the user should always be able to inspect source model outputs after the final answer arrives

---

## High-Level Architecture

## Target Flow

```text
User prompt
  -> create council run
  -> create assistant placeholder message
  -> launch N source-model jobs
  -> collect source responses and citations
  -> run fusion analysis on fusion model
  -> run final fused answer on same fusion model
  -> persist full council payload to assistant message
  -> keep UI updated through run-status polling
```

## Recommended execution phases

### Phase A - Setup

- validate source model count and uniqueness
- validate fusion model exists
- resolve reasoning effort defaults for each selected model
- persist council configuration
- create assistant placeholder message
- create council run record
- return message metadata and public council run id to the frontend

### Phase B - Source generation

- launch one source-model execution task per selected source model
- each source-model task can run inference in parallel
- grounded source-model tasks must request Brave search access through a global council run coordinator
- persist each source result incrementally as it completes

### Phase C - Fusion analysis

- after all source tasks finish or fail, build the fusion analysis input
- call the fusion model with strict JSON output instructions
- persist structured analysis results

### Phase D - Final fused answer

- call the same fusion model again with:
  - original prompt
  - successful source responses
  - structured analysis
  - evidence and citations
  - warnings and degradation metadata
- persist final answer to assistant message content
- mark run complete

---

## Recommended Public API Changes

## 1) Extend `POST /v1/chat/messages`

The current request shape only supports one agent model. Extend it when `mode=agent`.

### Recommended request body for council mode

```json
{
  "conversationId": "string",
  "message": "string",
  "mode": "agent",
  "grounding": true,
  "sourceModels": [
    { "modelId": "provider/model-a", "reasoningEffort": "high" },
    { "modelId": "provider/model-b", "reasoningEffort": "medium" }
  ],
  "fusionModel": {
    "modelId": "provider/model-c",
    "reasoningEffort": "high"
  },
  "fileIds": []
}
```

### Validation rules

- `sourceModels.length` must be between `1` and `5`
- all source model ids must be unique
- fusion model must be present
- each selected model must exist in the model catalog
- reasoning effort values must be valid if provided

### Backward compatibility

For the first implementation, support both:

- old single-model agent requests
- new council-mode agent requests

Recommended compatibility behavior:

- if old single-model agent shape is received, route it through the legacy agent path
- if `sourceModels` and `fusionModel` are present, route through the new council path

## 2) Add a public run status endpoint

Recommended new endpoint:

- `GET /v1/agent-runs/{id}`

This endpoint should return:

- top-level council run status
- current step (`sources`, `analysis`, `result`)
- per-source model status and payload
- analysis payload if available
- final result payload if available
- warnings
- timestamps

Recommended frontend behavior:

- the initial `POST /v1/chat/messages` still creates the assistant placeholder
- after receiving metadata, the client polls `GET /v1/agent-runs/{id}` every `1-2s` while the run is active
- polling stops when the run reaches a terminal state

This is the recommended first implementation because it is simpler and more reliable than trying to preserve one browser-to-worker stream for the full async run.

## 3) OpenAPI updates

Update `backend/openapi/openapi.yaml` to document:

- council-mode request body
- council run status endpoint
- council-specific response payload types
- backward compatibility rules for legacy agent mode

---

## Data Model Changes

## 1) Extend `agent_runs`

The current `agent_runs` table is too single-model for council fusion.

Add fields for:

- `workflow_type` or `mode_variant` with value `council_fusion`
- `source_model_ids_json`
- `fusion_model_id`
- `grounding_enabled`
- `council_config_json`
- `source_results_json`
- `fusion_analysis_json`
- `fusion_result_json`
- `completed_sources`
- `degraded_sources`
- `failed_sources`
- `public_status_json` or `run_summary_json`

Recommended first implementation approach:

- keep one row per council run
- store council state in structured JSON columns
- avoid a fully normalized run-step schema until product or analytics needs justify it

## 2) Extend `messages`

The assistant message must persist the council payload required for conversation replay.

Add fields or structured JSON payloads for:

- `agent_sources_json`
- `agent_analysis_json`
- `agent_result_model_id`
- `agent_result_usage_json`
- `agent_run_id`

Recommended behavior:

- keep the final fused answer in `messages.content`
- keep the council UI data in structured JSON fields
- allow the conversation reload path to reconstruct the entire council UI from persisted message data alone

## 3) Legacy payload handling

Current `agent_summaries_json` should be treated as legacy data.

Recommended behavior:

- preserve support for old agent messages
- do not overwrite or reinterpret old role-summary payloads as council data
- render council UI only when the new council payload exists

## 4) Preferences changes

Current model preferences only store one last-used agent model.

Add preference fields for:

- `last_used_agent_source_model_ids_json`
- `last_used_agent_fusion_model_id`

Reasoning presets should continue using the existing per-user, per-model, per-mode approach.

---

## Backend Type Design

Introduce explicit council types rather than overloading the current role-debate types.

### Recommended types

```go
type CouncilSourceSpec struct {
    ModelID         string `json:"modelId"`
    ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

type CouncilRunConfig struct {
    SourceModels []CouncilSourceSpec `json:"sourceModels"`
    FusionModel  CouncilSourceSpec   `json:"fusionModel"`
    Grounding    bool                `json:"grounding"`
}

type CouncilSourceResult struct {
    ModelID          string             `json:"modelId"`
    Status           string             `json:"status"`
    Response         string             `json:"response,omitempty"`
    ReasoningContent string             `json:"reasoningContent,omitempty"`
    Citations        []citationResponse `json:"citations,omitempty"`
    Usage            *messageUsage      `json:"usage,omitempty"`
    DurationMs       int64              `json:"durationMs,omitempty"`
    SearchQueries    int                `json:"searchQueries,omitempty"`
    ReadableSources  int                `json:"readableSources,omitempty"`
    Warnings         []string           `json:"warnings,omitempty"`
    Error            string             `json:"error,omitempty"`
}

type CouncilAnalysisItem struct {
    Point        string   `json:"point"`
    SourceModels []string `json:"sourceModels,omitempty"`
}

type CouncilDifferencePosition struct {
    SourceModel string `json:"sourceModel"`
    Summary     string `json:"summary"`
}

type CouncilDifferenceGroup struct {
    Topic     string                      `json:"topic"`
    Positions []CouncilDifferencePosition `json:"positions"`
}

type CouncilAnalysis struct {
    Agreement       []CouncilAnalysisItem    `json:"agreement"`
    KeyDifferences  []CouncilDifferenceGroup `json:"keyDifferences"`
    PartialCoverage []CouncilAnalysisItem    `json:"partialCoverage"`
    UniqueInsights  []CouncilAnalysisItem    `json:"uniqueInsights"`
    BlindSpots      []CouncilAnalysisItem    `json:"blindSpots"`
}

type CouncilFinalResult struct {
    ModelID          string        `json:"modelId"`
    Response         string        `json:"response"`
    ReasoningContent string        `json:"reasoningContent,omitempty"`
    Usage            *messageUsage `json:"usage,omitempty"`
}
```

Recommended behavior:

- treat the current `agentSummary` as legacy-only for old agent mode
- all new council runs should use the new council result types

---

## Grounding and Source Collection Design

This is the highest-risk and most important architectural area.

## Key distinction

Brave rate limits apply to Brave search requests, not page reads.

That means the council grounding pipeline must be split into two independent sub-systems:

1. `Search stage`
   - uses Brave
   - globally serialized
   - roughly one request every `1100ms`

2. `Read stage`
   - fetches the returned URLs directly
   - bounded by a page-read concurrency cap
   - counts successfully extracted pages toward the 15-source target

## Recommended grounded source-model algorithm

For each grounded source model:

1. generate one or more Brave queries appropriate to the prompt
2. request Brave results with `count=15`
3. dedupe and rank candidate URLs
4. fetch pages with bounded page-read concurrency
5. count only successfully extracted pages as readable web sources
6. if readable web sources are fewer than 15:
   - request another Brave query after the global 1100ms search interval
   - continue until:
     - 15 readable web sources are reached,
     - 3 Brave queries have been used for that source model,
     - or the run hits timeout

## Recommended status semantics

### `complete`

- source model produced an answer
- source model reached at least 15 readable web sources when grounding was enabled

### `degraded`

- source model produced an answer
- but it ended with fewer than 15 readable web sources because of:
  - fetch failures,
  - extraction failures,
  - query budget exhaustion,
  - timeout,
  - or sparse relevant results

### `failed`

- source model could not produce a usable answer at all

## Recommended coordinator behavior

Build a run-level grounding coordinator responsible for:

- granting Brave request slots one at a time
- tracking per-source search counts
- tracking total Brave queries used across the run
- tracking per-source readable web source counts
- exposing status metrics for the UI and logs

This should live as council-specific orchestration code rather than being hidden inside a single-model research loop.

---

## Fusion Analysis Design

After source model generation completes, the fusion model performs structured comparison.

## Required analysis output categories

- `agreement`
- `keyDifferences`
- `partialCoverage`
- `uniqueInsights`
- `blindSpots`

## Recommended analysis input

The fusion model should receive:

- the original prompt
- all successful source model responses
- source model labels and names
- per-source warnings
- degradation metadata
- per-source citations
- a reminder that some source models may have incomplete evidence

## Required output format

Use strict JSON.

### Recommended schema

```json
{
  "agreement": [
    {
      "point": "string",
      "sourceModels": ["model-a", "model-b"]
    }
  ],
  "keyDifferences": [
    {
      "topic": "string",
      "positions": [
        {
          "sourceModel": "model-a",
          "summary": "string"
        }
      ]
    }
  ],
  "partialCoverage": [
    {
      "point": "string",
      "sourceModels": ["model-c"]
    }
  ],
  "uniqueInsights": [
    {
      "point": "string",
      "sourceModels": ["model-d"]
    }
  ],
  "blindSpots": [
    {
      "point": "string"
    }
  ]
}
```

## Recommended prompt rules

- compare substance, not writing style
- identify genuine agreement, not superficial wording overlap
- treat degraded source-model outputs with caution
- identify unresolved contradictions explicitly
- identify blind spots relative to the user prompt, not only relative to source disagreements
- do not fabricate differences to fill categories

---

## Final Fused Answer Design

The same fusion model used for analysis must also produce the final answer.

## Required input

The fusion model should receive:

- original prompt
- successful source-model responses
- structured analysis
- source citations and warnings
- degradation and failure metadata

## Required behavior

- produce the strongest final answer possible
- preserve important consensus points
- incorporate valuable unique insights where justified
- mention unresolved differences when they matter
- preserve citation discipline when grounding is enabled
- surface uncertainty honestly when the evidence is incomplete

## Important rule

The fusion model used for Step 2/3 and Step 3/3 must be the same model for a given run.

---

## Backend Implementation Plan

## 1) Request parsing and mode routing

Update `backend/internal/httpapi/handler.go`:

- extend the chat request type to accept `sourceModels` and `fusionModel`
- validate council-mode input
- preserve backward compatibility for legacy agent requests
- persist council-specific model selection preferences
- include the new council run id in the initial metadata written to the placeholder message

Detailed steps:

1. add new request structs for council source model selection
2. add validation helpers for count, uniqueness, and model existence
3. resolve reasoning efforts per selected model
4. branch council requests to a new council queue path
5. keep legacy agent path available until council rollout is complete

## 2) Council run execution layer

Refactor `backend/internal/httpapi/agent_mode.go`:

- keep the async queue pattern
- add a dedicated execution path for council runs
- stop treating one `model_id` as the entire run definition

Detailed steps:

1. add council run config and result structs
2. create a council run placeholder message path
3. persist council config to `agent_runs`
4. implement source-model fan-out execution
5. add incremental persistence after each source-model completion
6. add fusion analysis stage
7. add final result stage
8. mark final run status and message payloads on completion or failure

## 3) Grounding coordinator

Add council-specific coordination code, likely in `backend/internal/httpapi/agent_mode.go` or extracted helper files.

Detailed steps:

1. implement a run-scoped Brave slot coordinator
2. enforce one Brave request every ~1100ms globally for the run
3. set Brave `count=15`
4. track per-source-model search query count
5. track per-source-model readable web source count
6. track page-read concurrency separately from Brave search concurrency
7. expose these counters in the run status payload

## 4) Research reuse strategy

Do not try to force the existing single-model orchestrator to own the whole council run.

Recommended approach:

- reuse pieces of the research stack where helpful:
  - Brave search client
  - reader
  - extraction
  - evidence ranking
- introduce a council-specific orchestration layer above them
- avoid bending the existing single-result orchestrator into a multi-model controller if that creates excessive complexity

## 5) Public run status endpoint

Add `GET /v1/agent-runs/{id}` in `backend/internal/httpapi/router.go` and `backend/internal/httpapi/handler.go`.

Detailed steps:

1. validate user ownership of the run
2. return council config, step state, source results, analysis, result, warnings, and timestamps
3. keep the payload stable enough for repeated polling
4. do not expose sensitive internal-only details

## 6) Persistence helpers

Add JSON encode/decode helpers for:

- council config
- source results
- analysis
- final result

Recommended behavior:

- use tolerant decoders for backward compatibility
- avoid breaking old message reads

---

## Frontend Implementation Plan

## 1) API types and polling support

Update `frontend/src/lib/api.ts`.

Detailed steps:

1. add council request types
2. add council payload types for source results, analysis, and final result
3. add agent run status response types
4. add client method for `GET /v1/agent-runs/{id}`
5. keep existing legacy agent message types compatible during rollout

## 2) App state changes

Update `frontend/src/App.tsx`.

Detailed steps:

1. add state for selected source models
2. add state for selected fusion model
3. add state for active council run ids per placeholder assistant message
4. start polling when a council run is queued
5. merge polled status into the correct assistant message
6. stop polling when the run completes or fails
7. restore council data on conversation reload

## 3) Composer UX

Update `frontend/src/components/Composer.tsx` and likely `frontend/src/components/ModelSelector.tsx`.

Detailed steps:

1. expose a council configuration tray, modal, or drawer when agent mode is active
2. allow selecting up to 5 source models
3. allow selecting one fusion model
4. clearly label the two roles: `Sources` and `Fuse with`
5. show selected model chips and counts
6. prevent duplicate source model selection
7. preserve current reasoning control behavior where possible

## 4) Message rendering

Update `frontend/src/components/ChatMessage.tsx`.

Detailed steps:

1. detect new council payloads
2. render Step 1/3 source model cards
3. render Step 2/3 analysis sections
4. render Step 3/3 fused final result
5. preserve citations, usage, and optional reasoning expansions
6. preserve old agent summary rendering for old messages until migration is complete

## 5) Styling

Update `frontend/src/styles.css`.

Detailed steps:

1. style source model cards for success, degraded, and failed states
2. style analysis categories as expandable sections
3. style fused result card distinctly but consistently with the existing app
4. make sure the layout works on narrow mobile widths
5. avoid large layout shifts as sections update over time

---

## Prompting Plan

## 1) Source model prompt

Each source model should receive:

- the original prompt
- recent conversation context
- attachment-expanded prompt content if applicable
- evidence context if grounding is enabled
- instructions to answer independently and not assume access to peer model outputs

Recommended rules:

- source models should not see each other
- source models should write their best direct answer
- source models should preserve citations where grounding is enabled

## 2) Analysis prompt

The fusion model should receive:

- original prompt
- all successful source responses
- degradation and failure metadata
- source citations
- strict JSON schema for the five analysis categories

## 3) Final synthesis prompt

The same fusion model should receive:

- original prompt
- all successful source responses
- structured analysis JSON
- evidence bundle and citations
- instructions to write the final answer with citation discipline and honest uncertainty handling

---

## Configuration Plan

Add council-specific config in `backend/internal/config/config.go` and document it in `backend/.env.example` and `backend/README.md`.

### Recommended env vars

- `COUNCIL_MODE_ENABLED=true`
- `COUNCIL_MAX_SOURCE_MODELS=5`
- `COUNCIL_TARGET_READABLE_SOURCES_PER_MODEL=15`
- `COUNCIL_SEARCH_RESULTS_PER_QUERY=15`
- `COUNCIL_MAX_SEARCH_QUERIES_PER_MODEL=3`
- `COUNCIL_MAX_BRAVE_SEARCHES_IN_FLIGHT=1`
- `COUNCIL_MAX_PAGE_READS_IN_FLIGHT=6`
- `COUNCIL_TIMEOUT_SECONDS=1200`
- `COUNCIL_REQUIRE_AT_LEAST_ONE_SUCCESSFUL_SOURCE_MODEL=true`

Recommended validation rules:

- `COUNCIL_MAX_SOURCE_MODELS` must be between `1` and `5`
- `COUNCIL_TARGET_READABLE_SOURCES_PER_MODEL` must be at least `15`
- `COUNCIL_SEARCH_RESULTS_PER_QUERY` should default to `15`
- `COUNCIL_MAX_BRAVE_SEARCHES_IN_FLIGHT` should default to `1`
- `COUNCIL_MAX_PAGE_READS_IN_FLIGHT` should be positive and modest

---

## Detailed Implementation Steps by Phase

## Phase 1 - Schema and contracts

1. add new migration for council run schema extensions
2. update `db/schema.sql`
3. extend `backend/openapi/openapi.yaml`
4. extend frontend and backend request/response types
5. add JSON encode/decode helpers

Exit criteria:

- council request shape is documented and parseable
- council payloads can be persisted and loaded

## Phase 2 - Queueing and run status plumbing

1. create council placeholder message path
2. create council run record with config
3. add public `GET /v1/agent-runs/{id}` endpoint
4. add frontend polling support

Exit criteria:

- user can queue a council run and see live run state updates

## Phase 3 - Source model execution

1. implement per-source-model run tasks
2. persist per-source result state incrementally
3. surface `queued`, `running`, `complete`, `degraded`, `failed`
4. attach citations and usage to each source result

Exit criteria:

- multiple source models can independently complete and render in the UI

## Phase 4 - Grounding coordinator and source target enforcement

1. add council run Brave coordinator
2. set Brave `count=15`
3. enforce global ~1100ms Brave request spacing
4. track readable web sources per source model
5. continue follow-up queries until 15 readable sources or cap/timeout
6. mark degraded source-model results when target is missed

Exit criteria:

- grounded source models target 15 readable web sources with correct pacing and degradation semantics

## Phase 5 - Fusion analysis

1. build structured analysis prompt
2. implement strict JSON parse and validation
3. persist analysis payload
4. render analysis categories in the UI

Exit criteria:

- user sees Agreement, Key Differences, Partial Coverage, Unique Insights, and Blind Spots

## Phase 6 - Final fused answer

1. build final synthesis prompt
2. ensure same fusion model performs synthesis
3. persist final answer in `messages.content`
4. persist supporting result payload
5. render fused answer card in the UI

Exit criteria:

- user sees a complete fused answer tied to the selected fusion model

## Phase 7 - Compatibility, polish, and cleanup

1. preserve rendering for old agent messages
2. refine mobile layout and loading states
3. document council mode in plan and backend docs
4. tune wording and degraded-state messaging

Exit criteria:

- old messages still render safely and the new council flow feels complete

---

## Testing Plan

## Backend tests

Add tests for:

- source model count validation
- duplicate source model rejection
- missing fusion model rejection
- council config persistence
- per-source result persistence
- degraded behavior when fewer than 15 readable web sources are obtained
- fusion proceeds when at least one source model succeeds
- fusion fails when all source models fail
- same fusion model used for both analysis and final answer
- Brave request serialization at about `1100ms`
- page-read concurrency cap
- conversation replay payload correctness

Recommended test files:

- `backend/internal/httpapi/handler_conversations_test.go`
- new tests alongside `backend/internal/httpapi/agent_mode.go`
- config tests in `backend/internal/config/config_test.go`

## Frontend tests

Add tests for:

- selecting up to 5 source models
- selecting a fusion model
- rendering mixed source statuses
- expanding an individual source model response
- rendering all five analysis categories
- rendering the final fused result
- showing degraded source-model results clearly
- preserving council UI after conversation reload
- compatibility with old agent messages

Recommended test files:

- `frontend/src/App.test.tsx`
- `frontend/src/components/ChatMessage.test.tsx`
- new tests for composer council interactions

---

## Observability Plan

Add structured logs and metrics for:

- council run start, completion, and failure
- number of source models selected
- grounded vs ungrounded runs
- per-source search query count
- per-source readable source count
- degraded source-model count
- failed source-model count
- fusion analysis latency
- final synthesis latency
- total Brave searches used by run
- total page reads attempted, succeeded, and failed
- per-model usage and cost

Do not log:

- full source-model responses
- full extracted page text
- secrets, tokens, or credentials

---

## Rollout Plan

### Phase 1 rollout

- ship schema and API changes behind `COUNCIL_MODE_ENABLED`
- keep legacy agent mode fully available

### Phase 2 rollout

- enable queueing and public run status for internal/dev use
- validate council message persistence and replay

### Phase 3 rollout

- enable source-model fan-out without grounding in controlled environments first
- verify usage and latency characteristics

### Phase 4 rollout

- enable grounded council runs with Brave pacing and 15-source targeting
- watch query usage carefully

### Phase 5 rollout

- enable full analysis and fused result UX for all users
- tune degraded-state messaging and quotas based on real runs

---

## Acceptance Criteria

This feature is complete when:

- user can select 1 to 5 source models and 1 fusion model
- user can inspect each source model's response individually
- user sees `Sources`, `Analysis`, and `Result` within one assistant message
- the analysis stage contains:
  - Agreement
  - Key Differences
  - Partial Coverage
  - Unique Insights
  - Blind Spots
- the same fusion model is used for both analysis and final answer
- grounded source models target 15 readable web sources each
- Brave search requests are globally serialized at about one request every 1100ms
- a source model that misses 15 readable sources is marked `degraded`, not silently treated as complete
- the final conversation history replay restores all council sections
- old agent messages still render safely

---

## File Change Map

### Backend

- `backend/internal/httpapi/handler.go`
- `backend/internal/httpapi/agent_mode.go`
- `backend/internal/httpapi/router.go`
- `backend/internal/httpapi/research_orchestration.go`
- `backend/internal/research/orchestrator.go`
- `backend/internal/brave/client.go`
- `backend/internal/openrouter/client.go`
- `backend/internal/config/config.go`
- `backend/openapi/openapi.yaml`

### Frontend

- `frontend/src/lib/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/components/Composer.tsx`
- `frontend/src/components/ModelSelector.tsx`
- `frontend/src/components/ChatMessage.tsx`
- `frontend/src/styles.css`

### Database and planning/docs

- `db/schema.sql`
- new migration in `db/migrations/`
- `plans/README.md`
- `backend/README.md`
- `docs/local-development.md`

---

## Recommended Implementation Order Summary

1. add council schema and API contracts
2. add council run queueing and public status polling
3. add source-model fan-out execution
4. add globally serialized Brave grounding coordinator
5. enforce 15 readable web sources target per grounded source model
6. add fusion analysis stage
7. add final fused answer stage
8. add full frontend Sources / Analysis / Result UI
9. add compatibility handling and test coverage

This ordering minimizes risk by getting persistence and run visibility stable before the heaviest orchestration and UI work lands.
