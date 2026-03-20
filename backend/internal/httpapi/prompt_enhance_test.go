package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chat/backend/internal/openrouter"
	"chat/backend/internal/session"
)

// stubCompleterStreamer wraps stubStreamer and adds a non-streaming ChatCompletion
// method so the handler's completer field is populated during construction.
type stubCompleterStreamer struct {
	stubStreamer
	completionContent string
	completionUsage   openrouter.Usage
	completionErr     error
	completionCalls   int
}

func (s *stubCompleterStreamer) ChatCompletion(_ context.Context, req openrouter.StreamRequest) (string, openrouter.Usage, error) {
	s.completionCalls++
	if s.onRequest != nil {
		s.onRequest(req)
	}
	return s.completionContent, s.completionUsage, s.completionErr
}

// newTestHandlerWithCompleter creates a test handler using a stubCompleterStreamer.
func newTestHandlerWithCompleter(t *testing.T, completer *stubCompleterStreamer) (Handler, *sql.DB) {
	t.Helper()
	handler, db := newTestHandler(t, completer)
	return handler, db
}

func enhanceRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/prompt/enhance", strings.NewReader(body))
	return requestWithSessionUser(req, session.User{
		ID:        "u1",
		GoogleSub: "sub1",
		Email:     "test@test.com",
		Name:      "Test User",
	})
}

// --- Tests ---

func TestEnhancePromptReturnsQuestions(t *testing.T) {
	t.Parallel()

	questionsJSON := `{
		"questions": [
			{"id": "q1", "text": "What detail level?", "type": "single_select", "options": [
				{"id": "opt_a", "label": "Brief"}, {"id": "opt_b", "label": "Detailed"}, {"id": "opt_c", "label": "Comprehensive"}
			]},
			{"id": "q2", "text": "Include examples?", "type": "yes_no", "options": [
				{"id": "yes", "label": "Yes"}, {"id": "no", "label": "No"}
			]}
		]
	}`

	streamer := &stubCompleterStreamer{completionContent: questionsJSON}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "Explain how databases work", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result enhancePromptResponse
	decodeJSONBody(t, resp, &result)

	if len(result.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(result.Questions))
	}
	if result.Questions[0].ID != "q1" {
		t.Fatalf("expected q1, got %s", result.Questions[0].ID)
	}
	if result.Questions[1].Type != "yes_no" {
		t.Fatalf("expected yes_no, got %s", result.Questions[1].Type)
	}
	if result.EnhancedPrompt != "" {
		t.Fatalf("expected no enhanced prompt, got %q", result.EnhancedPrompt)
	}
}

func TestEnhancePromptReturnsEnhancedPrompt(t *testing.T) {
	t.Parallel()

	enhancedJSON := `{"enhancedPrompt": "Explain how relational databases work, focusing on indexing and query optimization. Provide a detailed technical explanation with code examples."}`

	streamer := &stubCompleterStreamer{completionContent: enhancedJSON}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{
		"prompt": "Explain how databases work",
		"modelId": "openrouter/free",
		"answers": [
			{"questionId": "q1", "questionText": "What detail level?", "selectedOptions": ["Detailed"]},
			{"questionId": "q2", "questionText": "Include examples?", "selectedOptions": ["Yes"]}
		]
	}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result enhancePromptResponse
	decodeJSONBody(t, resp, &result)

	if result.EnhancedPrompt == "" {
		t.Fatalf("expected enhanced prompt, got empty")
	}
	if !strings.Contains(result.EnhancedPrompt, "relational databases") {
		t.Fatalf("unexpected enhanced prompt: %s", result.EnhancedPrompt)
	}
	if len(result.Questions) != 0 {
		t.Fatalf("expected no questions, got %d", len(result.Questions))
	}
}

func TestEnhancePromptGoDeeper(t *testing.T) {
	t.Parallel()

	questionsJSON := `{
		"questions": [
			{"id": "q3", "text": "What database engine?", "type": "single_select", "options": [
				{"id": "opt_a", "label": "PostgreSQL"}, {"id": "opt_b", "label": "MySQL"}, {"id": "opt_c", "label": "SQLite"}
			]}
		]
	}`

	streamer := &stubCompleterStreamer{completionContent: questionsJSON}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{
		"prompt": "Explain how databases work",
		"modelId": "openrouter/free",
		"previousEnhancedPrompt": "Explain how relational databases work with indexing.",
		"previousQuestionsAndAnswers": "Q1: What detail level?\n  → Selected: \"Detailed\"",
		"iteration": 1
	}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result enhancePromptResponse
	decodeJSONBody(t, resp, &result)

	if len(result.Questions) == 0 {
		t.Fatalf("expected questions for go deeper, got none")
	}
	if result.Questions[0].ID != "q3" {
		t.Fatalf("expected q3, got %s", result.Questions[0].ID)
	}
}

func TestEnhancePromptEmptyPromptRejects(t *testing.T) {
	t.Parallel()

	streamer := &stubCompleterStreamer{}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}

	var errResp errorResponse
	decodeJSONBody(t, resp, &errResp)
	if errResp.Error.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %s", errResp.Error.Code)
	}
}

func TestEnhancePromptMissingModelRejects(t *testing.T) {
	t.Parallel()

	streamer := &stubCompleterStreamer{}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "hello world", "modelId": ""}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestEnhancePromptIterationCap(t *testing.T) {
	t.Parallel()

	streamer := &stubCompleterStreamer{}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "hello", "modelId": "openrouter/free", "iteration": 4}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	var errResp errorResponse
	decodeJSONBody(t, resp, &errResp)
	if !strings.Contains(errResp.Error.Message, "iteration") {
		t.Fatalf("expected iteration error, got: %s", errResp.Error.Message)
	}
}

func TestEnhancePromptHandlesModelJSONError(t *testing.T) {
	t.Parallel()

	callCount := 0
	streamer := &stubCompleterStreamer{}
	// Override ChatCompletion to always return invalid JSON.
	origContent := "this is not json at all"
	streamer.completionContent = origContent

	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	streamer.onRequest = func(_ openrouter.StreamRequest) {
		callCount++
	}

	req := enhanceRequest(`{"prompt": "Explain databases", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	// Should have retried once (2 calls total).
	if callCount != 2 {
		t.Fatalf("expected 2 LLM calls (original + retry), got %d", callCount)
	}

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body.String())
	}

	var errResp errorResponse
	decodeJSONBody(t, resp, &errResp)
	if errResp.Error.Code != "parse_error" {
		t.Fatalf("expected parse_error, got %s", errResp.Error.Code)
	}
}

func TestEnhancePromptStripsMarkdownFences(t *testing.T) {
	t.Parallel()

	fencedJSON := "```json\n" + `{
		"questions": [
			{"id": "q1", "text": "Scope?", "type": "single_select", "options": [
				{"id": "a", "label": "Narrow"}, {"id": "b", "label": "Broad"}
			]}
		]
	}` + "\n```"

	streamer := &stubCompleterStreamer{completionContent: fencedJSON}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "Write an essay", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result enhancePromptResponse
	decodeJSONBody(t, resp, &result)

	if len(result.Questions) != 1 {
		t.Fatalf("expected 1 question after stripping fences, got %d", len(result.Questions))
	}
	if result.Questions[0].ID != "q1" {
		t.Fatalf("expected q1, got %s", result.Questions[0].ID)
	}
}

func TestEnhancePromptLLMError(t *testing.T) {
	t.Parallel()

	streamer := &stubCompleterStreamer{completionErr: errors.New("upstream timeout")}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "test prompt", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", resp.Code, resp.Body.String())
	}

	var errResp errorResponse
	decodeJSONBody(t, resp, &errResp)
	if errResp.Error.Code != "llm_error" {
		t.Fatalf("expected llm_error, got %s", errResp.Error.Code)
	}
}

func TestEnhancePromptAnswerValidation(t *testing.T) {
	t.Parallel()

	streamer := &stubCompleterStreamer{}
	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	// Missing questionId.
	req := enhanceRequest(`{
		"prompt": "test",
		"modelId": "openrouter/free",
		"answers": [{"questionId": "", "questionText": "test?", "selectedOptions": ["A"]}]
	}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing questionId, got %d", resp.Code)
	}

	// Missing selectedOptions.
	req2 := enhanceRequest(`{
		"prompt": "test",
		"modelId": "openrouter/free",
		"answers": [{"questionId": "q1", "questionText": "test?", "selectedOptions": []}]
	}`)
	resp2 := httptest.NewRecorder()
	handler.EnhancePrompt(resp2, req2)

	if resp2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty selectedOptions, got %d", resp2.Code)
	}
}

func TestEnhancePromptCompleterUnavailable(t *testing.T) {
	t.Parallel()

	// Use a plain stubStreamer that doesn't implement chatCompleter,
	// so the handler's completer field will be nil.
	streamer := stubStreamer{}
	handler, db := newTestHandler(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "test", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", resp.Code, resp.Body.String())
	}
}

// --- Unit tests for parsing helpers ---

func TestStripMarkdownFences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain json", `{"key": "value"}`, `{"key": "value"}`},
		{"with json fence", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"with plain fence", "```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"no fence", "hello world", "hello world"},
		{"fence with whitespace", "  ```json\n{\"a\":1}\n```  ", `{"a":1}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripMarkdownFences(tc.input)
			if result != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestParseQuestionsJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid questions", func(t *testing.T) {
		input := `{"questions": [{"id": "q1", "text": "Test?", "type": "single_select", "options": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}]}]}`
		questions, err := parseQuestionsJSON(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(questions))
		}
	})

	t.Run("empty questions", func(t *testing.T) {
		_, err := parseQuestionsJSON(`{"questions": []}`)
		if err == nil {
			t.Fatalf("expected error for empty questions")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		_, err := parseQuestionsJSON(`{"questions": [{"id": "q1", "text": "?", "type": "free_text", "options": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}]}]}`)
		if err == nil {
			t.Fatalf("expected error for invalid type")
		}
	})

	t.Run("too few options", func(t *testing.T) {
		_, err := parseQuestionsJSON(`{"questions": [{"id": "q1", "text": "?", "type": "single_select", "options": [{"id": "a", "label": "A"}]}]}`)
		if err == nil {
			t.Fatalf("expected error for too few options")
		}
	})
}

func TestParseEnhanceJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		result, err := parseEnhanceJSON(`{"enhancedPrompt": "Better prompt text"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "Better prompt text" {
			t.Fatalf("expected 'Better prompt text', got %q", result)
		}
	})

	t.Run("empty enhanced prompt", func(t *testing.T) {
		_, err := parseEnhanceJSON(`{"enhancedPrompt": ""}`)
		if err == nil {
			t.Fatalf("expected error for empty enhanced prompt")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseEnhanceJSON("not json")
		if err == nil {
			t.Fatalf("expected error for invalid json")
		}
	})
}

// Need to import sql for the db.Close() calls.
// We also verify the request captures work.
func TestEnhancePromptPassesCorrectSystemPrompt(t *testing.T) {
	t.Parallel()

	questionsJSON := `{"questions": [{"id": "q1", "text": "Scope?", "type": "single_select", "options": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}]}]}`

	var capturedReq openrouter.StreamRequest
	streamer := &stubCompleterStreamer{
		completionContent: questionsJSON,
	}
	streamer.onRequest = func(req openrouter.StreamRequest) {
		capturedReq = req
	}

	handler, db := newTestHandlerWithCompleter(t, streamer)
	t.Cleanup(func() { db.Close() })
	seedUser(t, db, "u1", "test@test.com")

	req := enhanceRequest(`{"prompt": "Write a poem", "modelId": "openrouter/free"}`)
	resp := httptest.NewRecorder()
	handler.EnhancePrompt(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	if len(capturedReq.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Fatalf("expected system role, got %s", capturedReq.Messages[0].Role)
	}
	if !strings.Contains(capturedReq.Messages[0].Content, "prompt engineering assistant") {
		t.Fatalf("system prompt missing key phrase: %s", capturedReq.Messages[0].Content)
	}
	if capturedReq.Messages[1].Role != "user" {
		t.Fatalf("expected user role, got %s", capturedReq.Messages[1].Role)
	}
	if !strings.Contains(capturedReq.Messages[1].Content, "Write a poem") {
		t.Fatalf("user message missing prompt content: %s", capturedReq.Messages[1].Content)
	}
	if capturedReq.Model != "openrouter/free" {
		t.Fatalf("expected model openrouter/free, got %s", capturedReq.Model)
	}
}

// Verify the import is used (json is used in decodeJSONBody which is in another file).
var _ = json.Unmarshal
