package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"chat/backend/internal/openrouter"
)

const maxEnhanceIterations = 3

// --- Request / Response types ---

type enhanceAnswerPayload struct {
	QuestionID      string   `json:"questionId"`
	QuestionText    string   `json:"questionText"`
	SelectedOptions []string `json:"selectedOptions"`
}

type enhancePromptRequest struct {
	Prompt                      string                 `json:"prompt"`
	ModelID                     string                 `json:"modelId"`
	ReasoningEffort             string                 `json:"reasoningEffort"`
	Answers                     []enhanceAnswerPayload `json:"answers"`
	PreviousEnhancedPrompt      string                 `json:"previousEnhancedPrompt"`
	PreviousQuestionsAndAnswers string                 `json:"previousQuestionsAndAnswers"`
	Iteration                   int                    `json:"iteration"`
}

type enhanceQuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type enhanceQuestion struct {
	ID      string                  `json:"id"`
	Text    string                  `json:"text"`
	Type    string                  `json:"type"`
	Options []enhanceQuestionOption `json:"options"`
}

type enhancePromptResponse struct {
	Questions      []enhanceQuestion `json:"questions,omitempty"`
	EnhancedPrompt string            `json:"enhancedPrompt,omitempty"`
}

// --- LLM response parsing types ---

type llmQuestionsResponse struct {
	Questions []enhanceQuestion `json:"questions"`
}

type llmEnhanceResponse struct {
	EnhancedPrompt string `json:"enhancedPrompt"`
}

// --- Handler ---

func (h Handler) EnhancePrompt(w http.ResponseWriter, r *http.Request) {
	if h.completer == nil {
		writeError(w, http.StatusServiceUnavailable, "enhance_unavailable", "prompt enhancement is not available")
		return
	}

	var req enhancePromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "prompt is required")
		return
	}

	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "modelId is required")
		return
	}

	if req.Iteration < 0 || req.Iteration > maxEnhanceIterations {
		writeError(w, http.StatusBadRequest, "invalid_request", "iteration must be between 0 and 3")
		return
	}

	// Determine which phase we're in.
	hasAnswers := len(req.Answers) > 0
	isGoDeeper := req.Iteration > 0 && !hasAnswers && strings.TrimSpace(req.PreviousEnhancedPrompt) != ""

	if hasAnswers {
		if err := validateAnswers(req.Answers); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		h.handleEnhance(w, r, req, prompt, modelID)
		return
	}

	if isGoDeeper {
		h.handleGoDeeperQuestions(w, r, req, prompt, modelID)
		return
	}

	h.handleInitialQuestions(w, r, prompt, modelID, req.ReasoningEffort)
}

func validateAnswers(answers []enhanceAnswerPayload) error {
	for _, a := range answers {
		if strings.TrimSpace(a.QuestionID) == "" {
			return errAnswerMissingQuestionID
		}
		if len(a.SelectedOptions) == 0 {
			return errAnswerMissingOptions
		}
	}
	return nil
}

var (
	errAnswerMissingQuestionID = newValidationError("each answer must have a non-empty questionId")
	errAnswerMissingOptions    = newValidationError("each answer must have at least one selectedOption")
)

type validationError struct {
	message string
}

func newValidationError(msg string) validationError {
	return validationError{message: msg}
}

func (e validationError) Error() string {
	return e.message
}

// --- Phase handlers ---

func (h Handler) handleInitialQuestions(w http.ResponseWriter, r *http.Request, prompt, modelID, reasoningEffort string) {
	messages := []openrouter.Message{
		{Role: "system", Content: promptEnhanceSystemInitial},
		{Role: "user", Content: buildEnhanceInitialUserMessage(prompt)},
	}

	content, err := h.callEnhanceLLM(r, messages, modelID, reasoningEffort)
	if err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", "failed to analyze prompt")
		return
	}

	questions, err := parseQuestionsJSON(content)
	if err != nil {
		// Retry the LLM call once on parse failure.
		content, err = h.callEnhanceLLM(r, messages, modelID, reasoningEffort)
		if err != nil {
			writeError(w, http.StatusBadGateway, "llm_error", "failed to analyze prompt")
			return
		}
		questions, err = parseQuestionsJSON(content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "parse_error", "failed to parse model response")
			return
		}
	}

	writeJSON(w, http.StatusOK, enhancePromptResponse{Questions: questions})
}

func (h Handler) handleGoDeeperQuestions(w http.ResponseWriter, r *http.Request, req enhancePromptRequest, prompt, modelID string) {
	userMessage := buildEnhanceGoDeeperUserMessage(
		prompt,
		req.PreviousEnhancedPrompt,
		req.PreviousQuestionsAndAnswers,
	)

	messages := []openrouter.Message{
		{Role: "system", Content: promptEnhanceSystemGoDeeper},
		{Role: "user", Content: userMessage},
	}

	content, err := h.callEnhanceLLM(r, messages, modelID, req.ReasoningEffort)
	if err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", "failed to generate deeper questions")
		return
	}

	questions, err := parseQuestionsJSON(content)
	if err != nil {
		content, err = h.callEnhanceLLM(r, messages, modelID, req.ReasoningEffort)
		if err != nil {
			writeError(w, http.StatusBadGateway, "llm_error", "failed to generate deeper questions")
			return
		}
		questions, err = parseQuestionsJSON(content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "parse_error", "failed to parse model response")
			return
		}
	}

	writeJSON(w, http.StatusOK, enhancePromptResponse{Questions: questions})
}

func (h Handler) handleEnhance(w http.ResponseWriter, r *http.Request, req enhancePromptRequest, prompt, modelID string) {
	userMessage := buildEnhanceUserMessage(prompt, req.Answers)

	messages := []openrouter.Message{
		{Role: "system", Content: promptEnhanceSystemEnhance},
		{Role: "user", Content: userMessage},
	}

	content, err := h.callEnhanceLLM(r, messages, modelID, req.ReasoningEffort)
	if err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", "failed to enhance prompt")
		return
	}

	enhanced, err := parseEnhanceJSON(content)
	if err != nil {
		content, err = h.callEnhanceLLM(r, messages, modelID, req.ReasoningEffort)
		if err != nil {
			writeError(w, http.StatusBadGateway, "llm_error", "failed to enhance prompt")
			return
		}
		enhanced, err = parseEnhanceJSON(content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "parse_error", "failed to parse model response")
			return
		}
	}

	writeJSON(w, http.StatusOK, enhancePromptResponse{EnhancedPrompt: enhanced})
}

// --- LLM call helper ---

func (h Handler) callEnhanceLLM(r *http.Request, messages []openrouter.Message, modelID, reasoningEffort string) (string, error) {
	var reasoning *openrouter.ReasoningConfig
	effort := strings.TrimSpace(reasoningEffort)
	if effort != "" {
		reasoning = &openrouter.ReasoningConfig{Effort: effort}
	}

	content, _, err := h.completer.ChatCompletion(r.Context(), openrouter.StreamRequest{
		Model:     modelID,
		Messages:  messages,
		Reasoning: reasoning,
	})
	return content, err
}

// --- JSON parsing helpers ---

var markdownFenceRe = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n?(.*?)\\s*```\\s*$")

// stripMarkdownFences removes ```json ... ``` wrapping if present.
func stripMarkdownFences(s string) string {
	if matches := markdownFenceRe.FindStringSubmatch(s); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return s
}

func parseQuestionsJSON(raw string) ([]enhanceQuestion, error) {
	cleaned := stripMarkdownFences(strings.TrimSpace(raw))

	var parsed llmQuestionsResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, err
	}

	if len(parsed.Questions) == 0 {
		return nil, errNoQuestions
	}

	for i := range parsed.Questions {
		if strings.TrimSpace(parsed.Questions[i].ID) == "" {
			return nil, errInvalidQuestion
		}
		if strings.TrimSpace(parsed.Questions[i].Text) == "" {
			return nil, errInvalidQuestion
		}
		qType := strings.TrimSpace(parsed.Questions[i].Type)
		if qType != "single_select" && qType != "multi_select" && qType != "yes_no" {
			return nil, errInvalidQuestionType
		}
		if len(parsed.Questions[i].Options) < 2 {
			return nil, errInvalidQuestionOptions
		}
	}

	return parsed.Questions, nil
}

func parseEnhanceJSON(raw string) (string, error) {
	cleaned := stripMarkdownFences(strings.TrimSpace(raw))

	var parsed llmEnhanceResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return "", err
	}

	enhanced := strings.TrimSpace(parsed.EnhancedPrompt)
	if enhanced == "" {
		return "", errEmptyEnhancedPrompt
	}

	return enhanced, nil
}

var (
	errNoQuestions            = newValidationError("model returned no questions")
	errInvalidQuestion        = newValidationError("model returned a question with missing id or text")
	errInvalidQuestionType    = newValidationError("model returned a question with invalid type")
	errInvalidQuestionOptions = newValidationError("model returned a question with fewer than 2 options")
	errEmptyEnhancedPrompt    = newValidationError("model returned an empty enhanced prompt")
)
