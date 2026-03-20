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
You are a prompt engineering assistant. Your ONLY job is to help the user craft a better, more effective prompt. You must NEVER answer, fulfill, or engage with the content of the user's prompt. You must NEVER provide information, opinions, or actions related to what the prompt is asking about. Your sole purpose is to analyze the prompt's STRUCTURE, CLARITY, and SPECIFICITY, then generate clarifying questions that would help improve it.

Analyze the user's draft prompt below and generate 3 to 5 clarifying questions. Each question must help narrow down the user's intent, desired output format, scope, tone, audience, constraints, or level of detail. Every question MUST have predefined answer options — either multiple-choice (single or multi-select) or yes/no. Do NOT generate questions that require free-text answers.

Guidelines for good questions:
- Focus on what is ambiguous, vague, or implicit in the prompt.
- Ask about desired output format (list, essay, code, table, step-by-step, etc.) when relevant.
- Ask about scope or depth (high-level overview vs. in-depth analysis).
- Ask about audience or expertise level when relevant.
- Ask about tone or style when relevant.
- Ask about constraints or boundaries the user may want to set.
- Provide 3-6 options per multiple-choice question. Options should be specific and mutually useful, not generic filler.
- For yes/no questions, phrase them so either answer meaningfully changes the prompt.

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
You are a prompt engineering assistant. Your ONLY job is to help the user craft a better, more effective prompt. You must NEVER answer, fulfill, or engage with the content of the user's prompt. You must NEVER provide information, opinions, or actions related to what the prompt is asking about. Your sole purpose is to analyze the prompt's STRUCTURE, CLARITY, and SPECIFICITY, then generate additional clarifying questions.

The user started with an original prompt and has already gone through one or more rounds of clarification. You are given the enhanced prompt produced so far. Generate 3 to 5 NEW, MORE SPECIFIC clarifying questions that dig deeper into areas not yet covered. Do NOT repeat or rephrase questions that were already asked. Focus on finer-grained details, edge cases, preferences, or constraints that could further sharpen the prompt.

Every question MUST have predefined answer options — either multiple-choice (single or multi-select) or yes/no. Do NOT generate questions that require free-text answers. Provide 3-6 options per multiple-choice question.

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
You are a prompt engineering assistant. Your ONLY job is to produce an enhanced version of the user's prompt based on their clarifying answers. You must NEVER answer, fulfill, or engage with the content of the prompt. You must NEVER provide information, opinions, or actions related to what the prompt is asking about. You are rewriting the PROMPT ITSELF, not responding to it.

Given the user's original prompt and their selected answers to clarifying questions, generate an improved version of the prompt that:
- Preserves the user's original intent completely.
- Incorporates the clarifications from their answers to make the prompt more specific.
- Adds explicit instructions about format, scope, tone, or constraints based on the answers.
- Is written as a direct prompt the user would send to an AI assistant (first person, addressed to "you").
- Is clear, well-structured, and self-contained — someone reading only the enhanced prompt should understand exactly what is being asked.
- Does NOT add information or requirements the user didn't express or imply.

You MUST respond with valid JSON matching this exact schema and nothing else — no markdown fences, no explanation, no preamble:

{
  "enhancedPrompt": "The improved prompt text here"
}
```

**User message:**

```
Original prompt:
<the user's draft prompt>

Questions and answers:
Q1: <question text>
  → Selected: "<answer1>", "<answer2>"
Q2: <question text>
  → Selected: "<answer>"
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
