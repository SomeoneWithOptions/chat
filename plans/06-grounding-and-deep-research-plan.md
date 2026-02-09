# Grounding and Deep Research Plan

## Grounding Defaults

- `grounding_enabled = true` by default for all messages (new and existing chats).
- User can toggle OFF per-message.
- Backend uses Brave results and prompt guidance without custom source blocklists.

## Brave API Integration

Use Brave "Data for AI" endpoints as the default retrieval tool for fresh web information.

Workflow:

1. Generate search query from user message and conversation context.
2. Call Brave API.
3. Normalize results (title, URL, summary, timestamp if available).
4. Rerank top N snippets for prompt inclusion.
5. Persist selected citations for final answer traceability.

## Modes

### Mode A: Normal Search Chat

- 1-2 query passes
- Max 5-8 citations
- Faster response and lighter synthesis

### Mode B: Deep Research

- Multi-pass query expansion (3-6 passes)
- Broader evidence collection and dedupe
- Structured synthesis with explicit sections
- Higher timeout and token budget
- Model is user-selectable per request
- Default deep-research model is user's last-used normal-chat model

## Prompt Templates (Draft)

### System Prompt: Normal Search

- Use attached context and provided web snippets.
- Prefer recent, reliable sources.
- Include concise citations for factual claims.
- If evidence is weak, state uncertainty clearly.

### System Prompt: Deep Research

- Build a thorough analysis before answering.
- Compare sources, note disagreements, and infer likely truth.
- Output sections:
  1. Direct Answer
  2. Key Evidence
  3. Conflicting Signals
  4. Practical Recommendations
  5. Source List

## Research Controls

- Max search calls per request
- Max runtime timeout: 120s
- User-visible progress events:
  - `planning`
  - `searching`
  - `synthesizing`
  - `finalizing`

## Acceptance Criteria

- Normal mode stays low-latency with grounding enabled
- Deep research outputs are materially more detailed than normal mode
- Citations are present for factual claims in both modes
- Deep research requests complete or fail gracefully within 120s
