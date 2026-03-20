# 13 — Prompt Enhance Feature

## Overview

A new "Enhance" button in the composer toolbar that helps users refine their prompts through a guided, interactive Q&A flow. The feature uses the user's currently selected model (via OpenRouter) to analyze their draft prompt, ask structured clarifying questions (multiple-choice and yes/no only — no free-text input), and produce an improved version.

The feature is strictly scoped to prompt engineering — the LLM must never answer, fulfill, or engage with the content of the user's prompt. It only analyzes structure, clarity, and specificity.

---

## UX Flow

```
User types prompt → clicks "Enhance" button in composer toolbar
        ↓
Modal overlay opens → "Analyzing your prompt..." loading spinner
        ↓
Backend returns 3–5 clarifying questions (multi-select / single-select / yes-no)
        ↓
User selects answers → clicks "Enhance" button inside modal
        ↓
Loading spinner → "Enhancing your prompt..."
        ↓
Modal shows enhanced prompt in a styled text block with three actions:
  • "Use this prompt" (primary)  → replaces textarea content, closes modal
  • "Copy" (secondary)           → copies to clipboard, shows "Copied!" feedback
  • "Go Deeper" (tertiary)       → sends enhanced prompt back for more questions
        ↓
If "Go Deeper" → loop back to questions step with new, more specific questions
(max 3 iterations, then "Go Deeper" is hidden)
```

### Modal States

| State | Content |
|-------|---------|
| `loading_questions` | Spinner + "Analyzing your prompt..." |
| `questions` | Question cards with selectable options + "Enhance" CTA |
| `loading_enhance` | Spinner + "Enhancing your prompt..." |
| `result` | Enhanced prompt text + "Use this prompt" / "Copy" / "Go Deeper" buttons |
| `error` | Error message + "Retry" button |

---

## Architecture

### API Design

Single endpoint handling both question generation and enhancement:

**`POST /v1/prompt/enhance`** — JSON request/response (not SSE)

#### Request Schema

```json
{
  "prompt": "string — the user's draft prompt (required)",
  "modelId": "string — the selected model ID (required)",
  "reasoningEffort": "string — low|medium|high (optional)",
  "answers": [
    {
      "questionId": "string",
      "questionText": "string — the question text for context",
      "selectedOptions": ["string — selected option labels"]
    }
  ],
  "previousEnhancedPrompt": "string — only on 'Go Deeper' calls",
  "previousQuestionsAndAnswers": "string — serialized Q&A history for Go Deeper",
  "iteration": 0
}
```

- **First call** (no `answers`): returns only `questions`.
- **Subsequent call** (with `answers`): returns `enhancedPrompt`.
- **Go Deeper** (with `previousEnhancedPrompt` + `iteration > 0`): returns new `questions`.
- **Go Deeper + answers**: returns new `enhancedPrompt`.

#### Response Schema

```json
{
  "questions": [
    {
      "id": "q1",
      "text": "What level of detail do you want in the response?",
      "type": "single_select",
      "options": [
        { "id": "opt_a", "label": "Brief overview" },
        { "id": "opt_b", "label": "Moderate detail with examples" },
        { "id": "opt_c", "label": "In-depth, comprehensive analysis" }
      ]
    }
  ],
  "enhancedPrompt": "string — only present when answers were provided"
}
```

#### Question Types

| Type | Behavior |
|------|----------|
| `yes_no` | Exactly two options with ids `"yes"` and `"no"` |
| `single_select` | 3–6 options, user picks exactly one |
| `multi_select` | 3–6 options, user picks one or more |

---

## System Prompts

### 1. Initial Question Generation (first call, no answers)

**System message:**

```
You are a prompt engineering assistant. Your ONLY job is to help the user create a better prompt. You must NEVER answer, fulfill, or engage with the substance of the user's request. You must NEVER provide the requested content itself. Your job is only to identify missing information and ask clarifying questions that will improve the prompt.

Before generating questions, do an internal analysis of the draft prompt:
1. Identify the task type. Choose the single best fit from:
   - coding/debugging
   - writing/editing
   - research/analysis
   - planning/strategy
   - creative generation
   - extraction/transformation
   - other
2. Identify the 1 to 3 biggest unresolved ambiguities that would most improve the final prompt if clarified.
3. Ask only questions that resolve those ambiguities.

Your goal is NOT to ask generic prompt-engineering questions. Your goal is to ask the highest-value questions.

Question rules:
- Generate 3 to 5 clarifying questions.
- Each question must target a DIFFERENT unresolved ambiguity.
- Each question must be grounded in the user's actual prompt, not generic best practices.
- Do NOT ask filler questions.
- Do NOT ask about tone, audience, format, or level of detail unless the answer would materially change the resulting prompt.
- Prefer questions whose answers will directly change the rewritten prompt in a meaningful way.
- Avoid questions whose answers can already be safely inferred from the prompt.
- Do NOT generate free-text questions. Every question must have predefined answer options only.

Task-specific priorities:
- For coding/debugging prompts, prioritize language, framework, runtime, inputs/outputs, constraints, edge cases, error handling, testing, environment, or performance goals.
- For writing/editing prompts, prioritize audience, source material, tone, length, structure, examples, and what to emphasize or avoid.
- For research/analysis prompts, prioritize scope, timeframe, evidence level, comparison criteria, assumptions, and decision criteria.
- For planning/strategy prompts, prioritize goals, constraints, timeline, resources, tradeoffs, risks, and success criteria.
- For creative prompts, prioritize style, constraints, references, boundaries, and desired originality.
- For extraction/transformation prompts, prioritize source format, output format, fields to keep, rules to apply, and edge cases.

Option rules:
- Use either:
  - "single_select"
  - "multi_select"
  - "yes_no"
- For multiple-choice questions, provide 3 to 6 options.
- Options must be concrete, specific, and meaningfully different.
- Avoid vague filler options like "Normal", "Standard", or "Other" unless absolutely necessary.
- For yes_no questions, phrase them so either answer would materially change the rewritten prompt.

If the prompt is already fairly specific, ask more advanced questions about constraints, success criteria, tradeoffs, exclusions, examples, or edge cases instead of repeating beginner-level prompt advice.

You MUST respond with valid JSON matching this exact schema and nothing else — no markdown fences, no explanation, no preamble:

{
  "questions": [
    {
      "id": "q1",
      "text": "The question text",
      "type": "single_select | multi_select | yes_no",
      "options": [
        { "id": "opt_a", "label": "Option label" }
      ]
    }
  ]
}

For yes_no type questions, always use exactly two options with ids "yes" and "no".
```

**User message:** `<the user's draft prompt>`

### 2. Go Deeper Question Generation (iteration 1+)

**System message:**

```
You are a prompt engineering assistant. Your ONLY job is to help the user create a better prompt. You must NEVER answer, fulfill, or engage with the substance of the user's request. You must NEVER provide the requested content itself. Your job is only to identify what is still missing and ask follow-up clarifying questions that improve the prompt further.

The user has already completed one or more rounds of clarification. You will receive:
- the original prompt
- the current enhanced prompt
- the previous questions and answers

Before generating follow-up questions, do an internal analysis:
1. Identify what is already well specified.
2. Identify the highest-value remaining gaps.
3. Ask only NEW questions that go deeper into unresolved areas.

Your goal is to ask the next best questions, not to repeat earlier categories.

Follow-up question rules:
- Generate 3 to 5 NEW clarifying questions.
- Do NOT repeat, restate, or lightly rephrase earlier questions.
- Do NOT ask about categories that are already sufficiently defined.
- Each question must target a DIFFERENT remaining ambiguity.
- Prefer advanced clarifications such as:
  - edge cases or failure modes
  - constraints or exclusions
  - tradeoffs or optimization goals
  - environment/runtime details
  - evaluation criteria or success criteria
  - examples, counterexamples, or boundaries
- Avoid generic questions about tone, audience, format, or detail level unless they are still genuinely unresolved and high impact.
- Do NOT generate free-text questions. Every question must have predefined answer options only.

Task-specific priorities:
- For coding/debugging prompts, go deeper on edge cases, interfaces, runtime assumptions, test expectations, constraints, performance, safety, or compatibility.
- For writing/editing prompts, go deeper on emphasis, structure, exclusions, references, evidence, or target reading level.
- For research/analysis prompts, go deeper on scope boundaries, comparison axes, confidence level, evidence standards, or decision criteria.
- For planning/strategy prompts, go deeper on constraints, sequencing, dependencies, risks, metrics, and acceptable tradeoffs.
- For creative prompts, go deeper on style boundaries, constraints, inspiration, originality, and what to avoid.
- For extraction/transformation prompts, go deeper on rules, schema, normalization, ambiguity handling, and output guarantees.

Option rules:
- Use either:
  - "single_select"
  - "multi_select"
  - "yes_no"
- For multiple-choice questions, provide 3 to 6 options.
- Options must be concrete, specific, and meaningfully different.
- For yes_no questions, phrase them so either answer would materially change the rewritten prompt.

You MUST respond with valid JSON matching this exact schema and nothing else — no markdown fences, no explanation, no preamble:

{
  "questions": [
    {
      "id": "q1",
      "text": "The question text",
      "type": "single_select | multi_select | yes_no",
      "options": [
        { "id": "opt_a", "label": "Option label" }
      ]
    }
  ]
}

For yes_no type questions, always use exactly two options with ids "yes" and "no".
```

**User message:**

```
Original prompt:
<the user's original draft>

Current enhanced prompt (after previous rounds):
<the enhanced prompt so far>

Previous questions and answers:
Q1: <question text>
  → Selected: "<answer1>", "<answer2>"
Q2: <question text>
  → Selected: "<answer>"
...
```

### 3. Enhancement (answers provided, generates improved prompt)

**System message:**

```
You are a prompt engineering assistant. Your ONLY job is to rewrite the user's prompt so it is clearer, more specific, and more effective. You must NEVER answer, fulfill, or engage with the substance of the user's request. You must NEVER provide the requested content itself. You are rewriting the PROMPT, not responding to it.

You will receive the user's original prompt and their selected answers to clarifying questions.

Generate an improved prompt that:
- preserves the user's original intent
- incorporates the clarifications from their answers
- is self-contained and easy for another AI assistant to follow
- is as detailed as necessary, but no more detailed than needed
- stays concise for simple requests
- becomes more explicit for complex requests
- adds structure only when it helps
- does NOT invent requirements, constraints, tools, formats, or details the user did not express or clearly imply

Rewriting rules:
- Keep the prompt written as something the user would send directly to an AI assistant.
- Prefer clear, natural instructions over prompt-engineering jargon.
- If the answers imply a useful expert role or perspective, include it. Do not force a persona when it is unnecessary.
- If the answers imply a preferred output structure, include it. Do not force bullets, tables, sections, or templates unless they are useful or requested.
- If the task is complex and the answers indicate that reasoning or process visibility would help, ask for a concise explanation of approach or the main steps. Do NOT request hidden internal reasoning or chain-of-thought.
- When relevant, include concrete constraints, scope boundaries, success criteria, edge cases, examples, or exclusions.
- Preserve any ambiguity that the user did not resolve instead of making up specifics.
- Make the final prompt feel tailored to this exact request, not like a generic template.

Quality bar:
- The rewritten prompt should materially improve the chances of getting a better answer.
- It should sound intentional and specific.
- It should not read like boilerplate.
- Every added instruction should be traceable to the original prompt or the selected answers.

You MUST respond with valid JSON matching this exact schema and nothing else — no markdown fences, no explanation, no preamble:

{
  "enhancedPrompt": "The improved prompt text here"
}
```

**User message:**

```
Original prompt:
<the user's draft prompt>

Current enhanced prompt so far:
<the enhanced prompt so far, if present>

Previous questions and answers:
<the previous Q/A history, if present>

Clarifications:
- Question: <question text>
  Selected answer(s): "<answer1>", "<answer2>"
- Question: <question text>
  Selected answer(s): "<answer>"
...
```

---

## Implementation Tasks

### Phase 1: Backend

#### Task 1 — Add `ChatCompletion` (non-streaming) to OpenRouter client

**File:** `backend/internal/openrouter/client.go`

Add a new method alongside the existing `StreamChatCompletion`:

```go
func (c Client) ChatCompletion(ctx context.Context, req StreamRequest) (string, Usage, error)
```

- Uses the same OpenRouter API but with `stream: false`.
- Returns the full response content as a string + usage stats.
- Handles error responses the same way as the streaming method.
- Timeout: 30 seconds context deadline.

**Tests:** `backend/internal/openrouter/client_test.go`
- Test successful JSON response parsing.
- Test error propagation from upstream.
- Test missing API key error.

#### Task 2 — Create prompt enhance handler

**File:** `backend/internal/httpapi/prompt_enhance.go` (new)

Handler for `POST /v1/prompt/enhance`:

```go
func (h *Handler) EnhancePrompt(w http.ResponseWriter, r *http.Request)
```

**Logic flow:**

1. Parse and validate request body.
2. Determine which phase we're in:
   - No `answers` + `iteration == 0` → initial question generation.
   - No `answers` + `iteration > 0` + has `previousEnhancedPrompt` → Go Deeper question generation.
   - Has `answers` → enhancement (generate improved prompt).
3. Build the appropriate system prompt + user message.
4. Call `openrouter.ChatCompletion` with the selected model.
5. Parse the model's JSON response.
6. Validate response matches expected schema (questions or enhancedPrompt).
7. If JSON parsing fails:
   - Strip markdown fences (` ```json ... ``` `) and retry parsing.
   - If still invalid, retry the LLM call once.
   - On second failure, return `500` with a user-friendly error.
8. Return the validated response.

**Validation rules:**
- `prompt` is required and must be non-empty.
- `modelId` is required and must exist in the model catalog.
- `iteration` must be 0–3 (max 3 Go Deeper rounds).
- Each answer's `questionId` must be a non-empty string.
- Each answer's `selectedOptions` must have at least one entry.

**File:** `backend/internal/httpapi/prompt_enhance_prompts.go` (new)

Contains the three system prompt constants and the user message builder functions:

```go
const (
    promptEnhanceSystemInitial   = `...`
    promptEnhanceSystemGoDeeper  = `...`
    promptEnhanceSystemEnhance   = `...`
)

func buildEnhanceUserMessage(req enhanceRequest) string { ... }
```

#### Task 3 — Register route

**File:** `backend/internal/httpapi/router.go`

Add inside the authenticated `v1` group:

```go
p.Post("/prompt/enhance", h.EnhancePrompt)
```

#### Task 4 — Backend tests

**File:** `backend/internal/httpapi/prompt_enhance_test.go` (new)

| Test case | Description |
|-----------|-------------|
| `TestEnhancePromptReturnsQuestions` | First call with only prompt → returns valid questions JSON |
| `TestEnhancePromptReturnsEnhancedPrompt` | Call with answers → returns enhancedPrompt |
| `TestEnhancePromptGoDeeper` | Call with previousEnhancedPrompt + iteration > 0 → returns new questions |
| `TestEnhancePromptEmptyPromptRejects` | Empty prompt → 400 |
| `TestEnhancePromptInvalidModelRejects` | Unknown model ID → 400 |
| `TestEnhancePromptIterationCap` | iteration > 3 → 400 |
| `TestEnhancePromptHandlesModelJsonError` | Model returns invalid JSON → retries, then 500 |
| `TestEnhancePromptStripsMarkdownFences` | Model wraps JSON in code fences → successfully parsed |

### Phase 2: Frontend

#### Task 5 — API client function

**File:** `frontend/src/lib/api.ts`

Add types and API function:

```typescript
// --- Types ---

export type EnhanceQuestionType = 'single_select' | 'multi_select' | 'yes_no';

export type EnhanceQuestionOption = {
  id: string;
  label: string;
};

export type EnhanceQuestion = {
  id: string;
  text: string;
  type: EnhanceQuestionType;
  options: EnhanceQuestionOption[];
};

export type EnhanceAnswer = {
  questionId: string;
  questionText: string;
  selectedOptions: string[];
};

export type EnhanceRequest = {
  prompt: string;
  modelId: string;
  reasoningEffort?: ReasoningEffort;
  answers?: EnhanceAnswer[];
  previousEnhancedPrompt?: string;
  previousQuestionsAndAnswers?: string;
  iteration?: number;
};

export type EnhanceResponse = {
  questions?: EnhanceQuestion[];
  enhancedPrompt?: string;
};

// --- API function ---

export async function enhancePrompt(request: EnhanceRequest): Promise<EnhanceResponse> {
  return requestJSON<EnhanceResponse>('/v1/prompt/enhance', {
    method: 'POST',
    body: JSON.stringify(request),
  });
}
```

#### Task 6 — PromptEnhanceModal component

**File:** `frontend/src/components/PromptEnhanceModal.tsx` (new)

**Props:**

```typescript
type PromptEnhanceModalProps = {
  isOpen: boolean;
  onClose: () => void;
  prompt: string;
  modelId: string;
  reasoningEffort?: ReasoningEffort;
  onUsePrompt: (enhancedPrompt: string) => void;
};
```

**Internal state:**

```typescript
const [modalState, setModalState] = useState<
  'loading_questions' | 'questions' | 'loading_enhance' | 'result' | 'error'
>('loading_questions');
const [questions, setQuestions] = useState<EnhanceQuestion[]>([]);
const [answers, setAnswers] = useState<Map<string, string[]>>(new Map());
const [enhancedPrompt, setEnhancedPrompt] = useState('');
const [iteration, setIteration] = useState(0);
const [qaHistory, setQaHistory] = useState<string>('');
const [error, setError] = useState<string | null>(null);
const [copied, setCopied] = useState(false);
```

**Component structure:**

```
<div className="enhance-modal-backdrop">          // fixed overlay, click to close
  <div className="enhance-modal">                  // centered panel
    <div className="enhance-modal-header">         // title + close button
      <h3>Enhance Prompt</h3>
      <button className="enhance-modal-close">×</button>
    </div>
    <div className="enhance-modal-body">           // scrollable content area
      {/* Renders based on modalState */}

      {/* loading_questions / loading_enhance */}
      <div className="enhance-loading">
        <div className="enhance-spinner" />
        <p>Analyzing your prompt... | Enhancing your prompt...</p>
      </div>

      {/* questions */}
      <div className="enhance-questions">
        {questions.map(q => (
          <div className="enhance-question-card">
            <p className="enhance-question-text">{q.text}</p>
            <div className="enhance-options">
              {q.options.map(opt => (
                <button
                  className={`enhance-option-chip ${selected ? 'selected' : ''}`}
                  onClick={...}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>

      {/* result */}
      <div className="enhance-result">
        <div className="enhance-result-text">{enhancedPrompt}</div>
        <div className="enhance-result-actions">
          <button className="btn-enhance-use">Use this prompt</button>
          <button className="btn-enhance-copy">
            {copied ? 'Copied!' : 'Copy'}
          </button>
          {iteration < 3 && (
            <button className="btn-enhance-deeper">Go Deeper</button>
          )}
        </div>
      </div>

      {/* error */}
      <div className="enhance-error">
        <p>{error}</p>
        <button>Retry</button>
      </div>
    </div>

    {/* Footer with Enhance CTA (only in questions state) */}
    {modalState === 'questions' && (
      <div className="enhance-modal-footer">
        <button className="btn-enhance-submit" disabled={noAnswersSelected}>
          Enhance
        </button>
      </div>
    )}
  </div>
</div>
```

**Behavior:**

- On open (`isOpen` transitions to `true`): immediately fires the first API call.
- Escape key closes the modal.
- Backdrop click closes the modal.
- "Use this prompt" calls `onUsePrompt(enhancedPrompt)` and closes.
- "Copy" uses `navigator.clipboard.writeText()` and shows "Copied!" for 2 seconds.
- "Go Deeper" increments iteration, serializes current Q&A into history, and fires a new question-generation call.
- All API calls use an `AbortController` that aborts on modal close.

#### Task 7 — Add "Enhance" button to Composer

**File:** `frontend/src/components/Composer.tsx`

Add new props:

```typescript
onEnhance: () => void;
enhanceDisabled?: boolean;
```

Add button after the Agent button inside `composer-mode-buttons`:

```tsx
<button
  type="button"
  className="composer-mode-button composer-enhance-button"
  onClick={onEnhance}
  disabled={enhanceDisabled}
  title="Enhance prompt"
>
  <svg className="mode-icon" viewBox="0 0 24 24" ...>
    {/* Sparkle/wand icon */}
  </svg>
  <span className="mode-text">Enhance</span>
</button>
```

The button is:
- **Not a toggle** — it's an action button (no active/inactive state).
- **Disabled** when: prompt is empty, streaming, or uploading.
- Uses a sparkle or magic wand icon (consistent with "enhance" semantics).

#### Task 8 — Wire up state in App.tsx

**File:** `frontend/src/App.tsx`

Add state and handlers:

```typescript
const [enhanceModalOpen, setEnhanceModalOpen] = useState(false);

function handleOpenEnhance() {
  if (prompt.trim().length === 0) return;
  setEnhanceModalOpen(true);
}

function handleUseEnhancedPrompt(enhanced: string) {
  setPrompt(enhanced);
  setEnhanceModalOpen(false);
}
```

Render the modal (conditionally, near other overlays):

```tsx
{enhanceModalOpen && (
  <PromptEnhanceModal
    isOpen={enhanceModalOpen}
    onClose={() => setEnhanceModalOpen(false)}
    prompt={prompt}
    modelId={selectedModel}
    reasoningEffort={reasoningEffort}
    onUsePrompt={handleUseEnhancedPrompt}
  />
)}
```

Pass props to Composer:

```tsx
<Composer
  ...
  onEnhance={handleOpenEnhance}
  enhanceDisabled={prompt.trim().length === 0 || isStreaming || uploadingAttachments}
/>
```

#### Task 9 — CSS styles

**File:** `frontend/src/styles.css`

New styles needed:

| Selector | Purpose |
|----------|---------|
| `.enhance-modal-backdrop` | Fixed fullscreen overlay, `rgba(0,0,0,0.5)`, `z-index: 200` |
| `.enhance-modal` | Centered panel, `max-width: 560px`, `max-height: 80vh`, `--bg-elevated`, `--radius-lg`, `--shadow-lg`, `animation: fadeIn + slideUp` |
| `.enhance-modal-header` | Flex row, title + close button, bottom border |
| `.enhance-modal-body` | `overflow-y: auto`, `padding: 20px` |
| `.enhance-modal-footer` | Bottom-pinned actions bar, top border |
| `.enhance-loading` | Centered flex column, spinner + text |
| `.enhance-spinner` | CSS-only spinner (rotating border), uses `--accent` |
| `.enhance-question-card` | Flex column, `gap: 12px`, bottom border between cards |
| `.enhance-question-text` | `--text-primary`, `font-weight: 500` |
| `.enhance-options` | `display: flex`, `flex-wrap: wrap`, `gap: 8px` |
| `.enhance-option-chip` | Rounded pill, `border: 1px solid var(--border)`, `padding: 8px 16px`, `min-height: 44px` (touch target), `cursor: pointer`, `transition` on background/border |
| `.enhance-option-chip.selected` | `background: var(--accent)`, `color: var(--bg-primary)`, `border-color: var(--accent)` |
| `.enhance-result-text` | `background: var(--bg-surface)`, `padding: 16px`, `border-radius: var(--radius-md)`, `white-space: pre-wrap`, `font-size: 14px`, `max-height: 300px`, `overflow-y: auto` |
| `.enhance-result-actions` | Flex row, `gap: 12px`, `margin-top: 16px` |
| `.btn-enhance-use` | Primary button, `background: var(--accent)`, `color: var(--bg-primary)` |
| `.btn-enhance-copy` | Secondary button, `border: 1px solid var(--border)` |
| `.btn-enhance-deeper` | Text/ghost button, `color: var(--text-secondary)` |
| `.composer-enhance-button` | Same as other `composer-mode-button` but without toggle styling |

**Responsive considerations:**
- On screens < 450px: hide `mode-text` for Enhance button (same as other mode buttons).
- Modal goes full-width on mobile with reduced padding.

#### Task 10 — Frontend tests

**File:** `frontend/src/components/PromptEnhanceModal.test.tsx` (new)

| Test case | Description |
|-----------|-------------|
| `renders loading state on open` | Modal opens → shows spinner |
| `renders questions after API response` | Mock API → questions appear |
| `single select allows only one option` | Click option A, then B → only B selected |
| `multi select allows multiple options` | Click A and C → both selected |
| `yes/no renders two options` | Yes/No question → two pill buttons |
| `enhance button disabled with no answers` | No selections → CTA disabled |
| `enhance button calls API with answers` | Select answers, click Enhance → API called correctly |
| `result view shows enhanced prompt` | Mock enhancement response → text displayed |
| `use this prompt calls onUsePrompt` | Click "Use this prompt" → callback fired with text |
| `copy button copies to clipboard` | Click Copy → clipboard API called |
| `go deeper fires new question request` | Click Go Deeper → new API call with iteration=1 |
| `go deeper hidden after 3 iterations` | iteration=3 → button not rendered |
| `backdrop click closes modal` | Click backdrop → onClose called |
| `escape key closes modal` | Press Escape → onClose called |
| `error state shows retry` | API error → error message + retry button |

---

## Edge Cases & Guardrails

| Concern | Mitigation |
|---------|------------|
| Empty/very short prompts | "Enhance" button disabled if prompt is empty. Short prompts (< 10 chars) are allowed — the model will generate basic questions. |
| Model errors / rate limits | Show error state in modal with retry button. Display the error message from the API. |
| JSON parse failures from model | Backend retries: (1) strip markdown fences and re-parse, (2) retry the LLM call once with same prompt. On second failure → 500 error to frontend. |
| Timeout | 30-second context deadline on the non-streaming OpenRouter call. Frontend shows error if request times out. |
| Iteration cap | Max 3 "Go Deeper" rounds. Backend rejects `iteration > 3` with 400. Frontend hides the button after iteration 3. |
| Modal dismissed during API call | `AbortController` cancels in-flight requests on modal close. |
| Model ignores JSON instruction | Validation layer in Go ensures response matches schema. Invalid responses trigger retry logic. |
| Cost awareness | Uses the user's already-selected model. No hidden model switching. Usage is "invisible" (not persisted as a conversation message). |

---

## Files Changed Summary

| File | Type | Description |
|------|------|-------------|
| `backend/internal/openrouter/client.go` | Modified | Add `ChatCompletion` non-streaming method |
| `backend/internal/openrouter/client_test.go` | Modified | Tests for non-streaming method |
| `backend/internal/httpapi/prompt_enhance.go` | **New** | Handler + request/response types |
| `backend/internal/httpapi/prompt_enhance_prompts.go` | **New** | System prompt constants + message builders |
| `backend/internal/httpapi/prompt_enhance_test.go` | **New** | Handler tests |
| `backend/internal/httpapi/router.go` | Modified | Register `POST /v1/prompt/enhance` |
| `frontend/src/lib/api.ts` | Modified | Add types + `enhancePrompt()` function |
| `frontend/src/components/PromptEnhanceModal.tsx` | **New** | Modal component |
| `frontend/src/components/PromptEnhanceModal.test.tsx` | **New** | Modal tests |
| `frontend/src/components/Composer.tsx` | Modified | Add "Enhance" button + new props |
| `frontend/src/App.tsx` | Modified | State management, modal toggle, handler wiring |
| `frontend/src/styles.css` | Modified | All new modal/question/option/result styles |

---

## Suggested PR Breakdown

| PR | Scope | Dependencies |
|----|-------|-------------|
| **PR 1** | Backend: `ChatCompletion` method on OpenRouter client + tests | None |
| **PR 2** | Backend: `POST /v1/prompt/enhance` handler, prompts, route registration + tests | PR 1 |
| **PR 3** | Frontend: API types + `enhancePrompt()` function | PR 2 |
| **PR 4** | Frontend: `PromptEnhanceModal` component + CSS + tests | PR 3 |
| **PR 5** | Frontend: Composer button + App.tsx wiring + integration | PR 4 |

Alternatively, PRs 3–5 can be merged into a single frontend PR if preferred.

---

## Acceptance Criteria

- [ ] "Enhance" button appears in composer toolbar, disabled when prompt is empty or streaming.
- [ ] Clicking "Enhance" opens a modal overlay with loading state.
- [ ] Modal displays 3–5 clarifying questions with selectable options (no free-text input).
- [ ] Selecting answers and clicking "Enhance" produces an improved prompt.
- [ ] "Use this prompt" replaces the composer textarea content and closes the modal.
- [ ] "Copy" copies the enhanced prompt to clipboard with visual feedback.
- [ ] "Go Deeper" generates additional questions for further refinement (up to 3 iterations).
- [ ] The feature uses the currently selected model — no hidden model switching.
- [ ] The LLM never answers or engages with the prompt content — only analyzes structure.
- [ ] Modal is dismissible via backdrop click, Escape key, or close button.
- [ ] In-flight API requests are cancelled when modal is dismissed.
- [ ] Errors are displayed with a retry option.
- [ ] Mobile responsive — modal adapts to small screens, option chips are touch-friendly (44px+ tap targets).
- [ ] All backend and frontend tests pass.
