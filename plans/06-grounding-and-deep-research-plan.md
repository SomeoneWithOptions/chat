# Grounding and Deep Research Plan

## Grounding Defaults

- `grounding_enabled = true` by default for all messages (new and existing chats).
- User can toggle OFF per-message.
- Backend uses Brave results and prompt guidance without custom source blocklists.

## Normal Chat (Grounded)

- Uses a single grounding pass with up to 6 Brave results.
- Includes grounded context in system prompt when available.
- Persists up to 8 citations on the assistant message.
- On grounding errors, streams a warning event and continues response generation.

## Deep Research (Implemented)

- Dedicated multi-pass orchestration path (separate from normal chat).
- Query planning + iterative Brave search across 3-6 passes.
- URL dedupe and evidence ranking with confidence filtering.
- High-confidence citations are used for synthesis and persistence.
- Deep-research prompt template requires structured sections:
  1. Direct Answer
  2. Key Evidence
  3. Conflicting Signals
  4. Recommendations
  5. Source List
- Inline source references use `[n]` markers mapped to provided evidence.
- Final persisted citations are reordered to match claim/reference order when possible.

## Research Controls

- Pass count: min 3, max 6.
- Results per pass: 6.
- Max deep-research citations persisted/streamed: 10.
- Runtime timeout: `DEEP_RESEARCH_TIMEOUT_SECONDS` (default 120s), enforced with request-scoped context timeout.
- User-visible progress events streamed over SSE:
  - `planning`
  - `searching`
  - `synthesizing`
  - `finalizing`

## Acceptance Criteria

- Normal mode stays low-latency with grounding enabled
- Deep research outputs are materially more detailed than normal mode
- Citations are present for factual claims in both modes
- Deep research requests complete or fail gracefully within 120s
