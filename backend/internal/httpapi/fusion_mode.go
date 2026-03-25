package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"chat/backend/internal/brave"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const braveProviderName = "brave"

type multiModelFusionQueuedStreamInput struct {
	UserID         string
	UserMessageID  string
	ConversationID string
	SourceModels   []FusionSourceSpec
	FusionModel    FusionSourceSpec
	Grounding      bool
}

type fusionQueuedStreamInput struct {
	UserID          string
	UserMessageID   string
	ConversationID  string
	ModelID         string
	ReasoningEffort string
	Grounding       bool
}

type fusionRunRecord struct {
	ID                 string
	UserID             string
	ConversationID     string
	UserMessageID      string
	AssistantMessageID string
	ModelID            string
	ReasoningEffort    string
	Status             string
	SearchBudget       int
	SearchesUsed       int
	SourcesRead        int
	LastError          string
	WorkflowType       string
	SourceModelIDsJSON string
	FusionModelID      string
	GroundingEnabled   bool
	FusionConfigJSON   string
}

type fusionPerspectiveDefinition struct {
	Name   string
	System string
}

type fusionPerspectiveResult struct {
	Role        string   `json:"role"`
	Summary     string   `json:"summary"`
	Objections  []string `json:"objections,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	EvidenceIDs []int    `json:"evidenceIds,omitempty"`
}

func (h Handler) streamFusionQueuedResponse(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, input fusionQueuedStreamInput) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	metadataEvent := map[string]any{
		"type":           "metadata",
		"grounding":      true,
		"deepResearch":   false,
		"responseMode":   "fusion",
		"modelId":        input.ModelID,
		"conversationId": input.ConversationID,
		"userMessageId":  input.UserMessageID,
	}
	if input.ReasoningEffort != "" {
		metadataEvent["reasoningEffort"] = input.ReasoningEffort
	}
	_ = writeSSEEvent(w, metadataEvent)

	traceCollector := newThinkingTraceCollector()
	initialProgress := summarizedProgress(research.Progress{
		Phase:   research.PhasePlanning,
		Message: "Queueing fusion run",
	}, research.ProgressSummaryInput{
		Phase: research.PhasePlanning,
	})
	traceCollector.AppendProgress(initialProgress)
	_ = writeSSEEvent(w, progressEventData(initialProgress))
	flusher.Flush()

	assistantMessageID, err := h.insertMessageWithCitations(
		ctx,
		input.UserID,
		input.ConversationID,
		"assistant",
		"",
		"",
		input.ModelID,
		true,
		false,
		"fusion",
		nil,
		traceCollector.Snapshot(),
		nil,
		nil,
	)
	if err != nil {
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to create fusion placeholder"})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		return
	}

	searchBudget, budgetErr := h.availableFusionSearchBudget(ctx)
	if budgetErr != nil {
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to reserve Brave search budget"})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		_ = h.updateFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "Fusion mode is temporarily unavailable while Brave quota is being checked.", traceCollector, nil, nil, nil, thinkingTraceStatusStopped)
		return
	}

	runID := uuid.NewString()
	if searchBudget < h.cfg.FusionMinSearchQueries {
		if err := h.insertFusionRun(ctx, fusionRunRecord{
			ID:                 runID,
			UserID:             input.UserID,
			ConversationID:     input.ConversationID,
			UserMessageID:      input.UserMessageID,
			AssistantMessageID: assistantMessageID,
			ModelID:            input.ModelID,
			ReasoningEffort:    input.ReasoningEffort,
			Status:             "failed",
			SearchBudget:       searchBudget,
			LastError:          "Not enough Brave monthly budget remains for fusion mode.",
		}); err != nil {
			log.Printf("fusion run persist failed: %v", err)
		}
		_ = writeSSEEvent(w, map[string]any{
			"type":    "warning",
			"scope":   "fusion",
			"message": "Fusion mode needs at least 20 Brave searches, but the remaining monthly Brave budget is too low right now.",
		})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		_ = h.updateFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "Fusion mode could not start because the remaining Brave free-tier budget is below the minimum search reserve.", traceCollector, nil, nil, nil, thinkingTraceStatusStopped)
		return
	}

	if err := h.insertFusionRun(ctx, fusionRunRecord{
		ID:                 runID,
		UserID:             input.UserID,
		ConversationID:     input.ConversationID,
		UserMessageID:      input.UserMessageID,
		AssistantMessageID: assistantMessageID,
		ModelID:            input.ModelID,
		ReasoningEffort:    input.ReasoningEffort,
		Status:             "queued",
		SearchBudget:       searchBudget,
	}); err != nil {
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to queue fusion run"})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		return
	}

	queuedProgress := summarizedProgress(research.Progress{
		Phase:   research.PhasePlanning,
		Message: "Queued fusion workflow",
	}, research.ProgressSummaryInput{
		Phase: research.PhasePlanning,
	})
	traceCollector.AppendProgress(queuedProgress)
	_ = writeSSEEvent(w, progressEventData(queuedProgress))
	_ = writeSSEEvent(w, map[string]any{"type": "done"})
	flusher.Flush()

	if err := h.updateFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "", traceCollector, nil, nil, nil, thinkingTraceStatusRunning); err != nil {
		log.Printf("fusion placeholder update failed: run_id=%s err=%v", runID, err)
	}
	if err := h.enqueueFusionRun(context.Background(), runID); err != nil {
		log.Printf("fusion enqueue failed: run_id=%s err=%v", runID, err)
		_ = h.failFusionRun(context.Background(), runID, "failed to enqueue fusion run")
	}
}

func (h Handler) InternalRunFusion(w http.ResponseWriter, r *http.Request) {
	if err := h.requireInternalWorkerToken(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "fusion run id is required")
		return
	}
	if err := h.executeFusionRun(r.Context(), runID); err != nil {
		log.Printf("fusion worker failed: run_id=%s err=%v", runID, err)
		writeError(w, http.StatusInternalServerError, "fusion_run_failed", "fusion run failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h Handler) requireInternalWorkerToken(r *http.Request) error {
	expected := strings.TrimSpace(h.cfg.InternalWorkerBearerToken)
	if expected == "" {
		return nil
	}
	actual, err := readBearerToken(r)
	if err != nil {
		return err
	}
	if subtleConstantTimeCompare(actual, expected) {
		return nil
	}
	return errors.New("invalid worker token")
}

func subtleConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (h Handler) enqueueFusionRun(ctx context.Context, runID string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(h.cfg.InternalWorkerBaseURL), "/")
	if baseURL == "" {
		go func() {
			localCtx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.FusionTimeoutSeconds+30)*time.Second)
			defer cancel()
			if err := h.executeFusionRun(localCtx, runID); err != nil {
				log.Printf("fusion local worker failed: run_id=%s err=%v", runID, err)
			}
		}()
		return nil
	}

	go func() {
		workerCtx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.FusionTimeoutSeconds+30)*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(workerCtx, http.MethodPost, baseURL+"/internal/fusion-runs/"+runID, bytes.NewReader(nil))
		if err != nil {
			log.Printf("fusion remote worker request build failed: run_id=%s err=%v", runID, err)
			_ = h.failFusionRun(context.Background(), runID, "failed to enqueue fusion run")
			return
		}
		if token := strings.TrimSpace(h.cfg.InternalWorkerBearerToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("fusion remote worker request failed: run_id=%s err=%v", runID, err)
			_ = h.failFusionRun(context.Background(), runID, "failed to enqueue fusion run")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			log.Printf("fusion remote worker returned non-2xx: run_id=%s status=%d", runID, resp.StatusCode)
			_ = h.failFusionRun(context.Background(), runID, "failed to enqueue fusion run")
		}
	}()
	return nil
}

func (h Handler) executeFusionRun(ctx context.Context, runID string) error {
	run, err := h.claimFusionRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return nil
	}
	if run.WorkflowType == "multi_model" {
		return h.executeMultiModelFusionRun(ctx, run)
	}

	traceCollector, err := h.loadMessageTrace(ctx, run.UserID, run.AssistantMessageID)
	if err != nil {
		traceCollector = newThinkingTraceCollector()
	}
	appendStageProgress := func(progress research.Progress) {
		traceCollector.AppendProgress(progress)
		if updateErr := h.updateFusionAssistantMessage(ctx, run.UserID, run.AssistantMessageID, "", traceCollector, nil, nil, nil, thinkingTraceStatusRunning); updateErr != nil {
			log.Printf("fusion progress persist failed: run_id=%s err=%v", run.ID, updateErr)
		}
	}
	appendStageProgress(research.Progress{
		Phase:  research.PhasePlanning,
		Title:  "Starting fusion workflow",
		Detail: "Worker picked up the queued run",
	})

	searcher := &distributedBraveSearcher{
		handler:   &h,
		runID:     run.ID,
		runBudget: run.SearchBudget,
	}

	cfg := h.buildResearchConfig(research.ModeFusion)
	cfg.MinSearchQueries = h.cfg.FusionMinSearchQueries
	cfg.MaxSearchQueries = run.SearchBudget
	cfg.MaxSourcesRead = h.cfg.FusionMaxSourcesRead
	cfg.Timeout = time.Duration(h.cfg.FusionTimeoutSeconds) * time.Second

	plannerResponder := newOpenRouterPlannerResponder(h.openrouter, run.ModelID, plannerReasoningEffort(run.ReasoningEffort))
	planner := research.NewJSONPlanner(plannerResponder)
	orchestrator := research.NewOrchestrator(searcher, planner, h.researchReader, cfg)

	historyMessages, err := h.listConversationPromptMessages(ctx, run.UserID, run.ConversationID, maxConversationHistoryMessages)
	if err != nil {
		return h.failFusionRun(ctx, run.ID, "failed to load fusion conversation history")
	}

	userPrompt := ""
	for idx := len(historyMessages) - 1; idx >= 0; idx-- {
		if historyMessages[idx].Role == "user" {
			userPrompt = historyMessages[idx].Content
			break
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return h.failFusionRun(ctx, run.ID, "failed to resolve fusion prompt")
	}
	attachedFiles, err := h.listFilesForMessage(ctx, run.UserID, run.UserMessageID)
	if err != nil {
		log.Printf("fusion attachment lookup failed: run_id=%s message_id=%s err=%v", run.ID, run.UserMessageID, err)
	} else if len(attachedFiles) > 0 {
		userPrompt = h.appendFileContextToPrompt(userPrompt, attachedFiles)
	}
	timeSensitive := isTimeSensitivePrompt(userPrompt)

	var progressMu sync.Mutex
	researchResult, err := orchestrator.Run(ctx, userPrompt, timeSensitive, func(progress research.Progress) {
		progressMu.Lock()
		defer progressMu.Unlock()
		appendStageProgress(progress)
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("fusion research failed: run_id=%s err=%v", run.ID, err)
	}

	citations := convertResearchCitations(researchResult.Citations, h.cfg.ResearchMaxCitationsDeep)
	roles, roleErr := h.runFusionPerspectiveReview(ctx, run.ModelID, run.ReasoningEffort, userPrompt, citations, appendStageProgress)
	if roleErr != nil {
		return h.failFusionRun(ctx, run.ID, "fusion debate failed")
	}
	appendStageProgress(research.Progress{
		Phase:  research.PhaseSynthesizing,
		Title:  "Writing final answer",
		Detail: "Combining evidence and debate results",
	})
	answer, reasoningContent, usage, synthErr := h.runFusionSynthesis(ctx, run.ModelID, run.ReasoningEffort, userPrompt, historyMessages, citations, roles)
	if synthErr != nil {
		return h.failFusionRun(ctx, run.ID, "fusion synthesis failed")
	}

	orderedCitations := orderCitationsByClaims(citations, answer)
	appendStageProgress(research.Progress{
		Phase:  research.PhaseFinalizing,
		Title:  "Finalizing response",
		Detail: "Saving answer, citations, and fusion summaries",
	})
	traceCollector.MarkDone()
	if err := h.updateFusionAssistantMessage(ctx, run.UserID, run.AssistantMessageID, answer, traceCollector, orderedCitations, messageUsageFromOpenRouter(usage), toFusionSummaries(roles), thinkingTraceStatusDone); err != nil {
		return err
	}

	if err := h.finishFusionRun(ctx, run.ID, "completed", "", searcher.searchesUsed, researchResult.SourcesRead); err != nil {
		return err
	}
	if usage != nil {
		h.enrichAndPersistMessageUsageAsync(run.UserID, run.AssistantMessageID, run.ModelID, *usage, time.Now(), time.Now())
	}
	_ = reasoningContent
	return nil
}

func (h Handler) runFusionPerspectiveReview(ctx context.Context, modelID, reasoningEffort, question string, citations []citationResponse, onProgress func(research.Progress)) ([]fusionPerspectiveResult, error) {
	roles := []fusionPerspectiveDefinition{
		{Name: "Scout", System: "You are Scout. Find the strongest broad answer and the most useful resource directions."},
		{Name: "Skeptic", System: "You are Skeptic. Look for contradictions, edge cases, and reasons the obvious answer may be incomplete."},
		{Name: "Verifier", System: "You are Verifier. Focus on source quality, recency, and citation discipline."},
		{Name: "User Advocate", System: "You are User Advocate. Focus on what the user actually needs, what would make the answer clearer, and what actionability is missing."},
	}

	firstRound := make([]fusionPerspectiveResult, len(roles))
	if onProgress != nil {
		onProgress(research.Progress{
			Phase:       research.PhaseEvaluating,
			Title:       "Comparing fusion viewpoints",
			Detail:      "Round 1 of 2",
			Pass:        1,
			TotalPasses: 2,
		})
	}
	if err := h.runRoleRound(ctx, modelID, reasoningEffort, question, citations, nil, roles, firstRound); err != nil {
		return nil, err
	}

	secondRound := make([]fusionPerspectiveResult, len(roles))
	if onProgress != nil {
		onProgress(research.Progress{
			Phase:       research.PhaseIterating,
			Title:       "Running rebuttal round",
			Detail:      "Round 2 of 2",
			Pass:        2,
			TotalPasses: 2,
		})
	}
	if err := h.runRoleRound(ctx, modelID, reasoningEffort, question, citations, firstRound, roles, secondRound); err != nil {
		return nil, err
	}
	return secondRound, nil
}

func (h Handler) runRoleRound(ctx context.Context, modelID, reasoningEffort, question string, citations []citationResponse, peerResults []fusionPerspectiveResult, defs []fusionPerspectiveDefinition, out []fusionPerspectiveResult) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(defs))
	for i, def := range defs {
		i, def := i, def
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := h.runRolePrompt(ctx, modelID, reasoningEffort, question, citations, peerResults, def)
			if err != nil {
				errCh <- err
				return
			}
			out[i] = response
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (h Handler) runRolePrompt(ctx context.Context, modelID, reasoningEffort, question string, citations []citationResponse, peerResults []fusionPerspectiveResult, def fusionPerspectiveDefinition) (fusionPerspectiveResult, error) {
	var prompt strings.Builder
	prompt.WriteString(def.System)
	prompt.WriteString("\nReturn strict JSON with keys: role, summary, objections, confidence, evidenceIds.\n")
	prompt.WriteString("Keep summary to 2-4 sentences. Use evidenceIds that exist.\n")
	prompt.WriteString("Question:\n")
	prompt.WriteString(question)
	prompt.WriteString("\n\nEvidence:\n")
	prompt.WriteString(buildDeepResearchEvidencePrompt(citations, isTimeSensitivePrompt(question)))
	if len(peerResults) > 0 {
		prompt.WriteString("\nPeer summaries:\n")
		for _, peer := range peerResults {
			if strings.EqualFold(peer.Role, def.Name) {
				continue
			}
			prompt.WriteString("- ")
			prompt.WriteString(peer.Role)
			prompt.WriteString(": ")
			prompt.WriteString(peer.Summary)
			prompt.WriteString("\n")
		}
	}
	response, _, err := h.collectChatCompletion(ctx, modelID, reasoningEffort, []openrouter.Message{
		{Role: "system", Content: "You are a structured JSON generator."},
		{Role: "user", Content: prompt.String()},
	})
	if err != nil {
		return fusionPerspectiveResult{}, err
	}
	var parsed fusionPerspectiveResult
	if err := json.Unmarshal([]byte(extractJSONObject(response)), &parsed); err != nil {
		return fusionPerspectiveResult{
			Role:       def.Name,
			Summary:    strings.TrimSpace(response),
			Confidence: 0.5,
		}, nil
	}
	if strings.TrimSpace(parsed.Role) == "" {
		parsed.Role = def.Name
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		parsed.Summary = fmt.Sprintf("%s could not produce a structured summary.", def.Name)
	}
	return parsed, nil
}

func (h Handler) runFusionSynthesis(ctx context.Context, modelID, reasoningEffort, question string, history []openrouter.Message, citations []citationResponse, roles []fusionPerspectiveResult) (string, string, *openrouter.Usage, error) {
	var prompt strings.Builder
	prompt.WriteString("Write the best possible answer for the user using the cited evidence")
	if len(roles) > 0 {
		prompt.WriteString(" and the role summaries")
	}
	prompt.WriteString(".\n")
	prompt.WriteString("Rules:\n")
	prompt.WriteString("- Use citations like [1], [2].\n")
	prompt.WriteString("- Be explicit about uncertainty.\n")
	prompt.WriteString("- Prefer accuracy over confidence.\n\n")
	prompt.WriteString("Question:\n")
	prompt.WriteString(question)
	prompt.WriteString("\n\nEvidence:\n")
	prompt.WriteString(buildDeepResearchEvidencePrompt(citations, isTimeSensitivePrompt(question)))
	if len(roles) > 0 {
		prompt.WriteString("\nRole summaries:\n")
		for _, role := range roles {
			prompt.WriteString("- ")
			prompt.WriteString(role.Role)
			prompt.WriteString(": ")
			prompt.WriteString(role.Summary)
			if len(role.Objections) > 0 {
				prompt.WriteString(" Objections: ")
				prompt.WriteString(strings.Join(role.Objections, "; "))
			}
			prompt.WriteString("\n")
		}
	}

	messages := []openrouter.Message{
		{Role: "system", Content: buildDeepResearchSystemPrompt(isTimeSensitivePrompt(question))},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, openrouter.Message{Role: "user", Content: prompt.String()})
	content, reasoning, err := h.collectChatCompletion(ctx, modelID, reasoningEffort, messages)
	if err != nil {
		return "", "", nil, err
	}
	return content, reasoning.reasoning, reasoning.usage, nil
}

type completionResult struct {
	reasoning string
	usage     *openrouter.Usage
}

func (h Handler) collectChatCompletion(ctx context.Context, modelID, reasoningEffort string, messages []openrouter.Message) (string, completionResult, error) {
	var content strings.Builder
	var reasoning strings.Builder
	var usage *openrouter.Usage
	err := h.openrouter.StreamChatCompletion(
		ctx,
		openrouter.StreamRequest{
			Model:     modelID,
			Messages:  messages,
			Reasoning: openRouterReasoningConfig(reasoningEffort),
		},
		func() error { return nil },
		func(delta string) error {
			content.WriteString(delta)
			return nil
		},
		func(delta string) error {
			reasoning.WriteString(delta)
			return nil
		},
		func(next openrouter.Usage) error {
			copied := next
			usage = &copied
			return nil
		},
	)
	if err != nil {
		return "", completionResult{}, err
	}
	return strings.TrimSpace(content.String()), completionResult{
		reasoning: strings.TrimSpace(reasoning.String()),
		usage:     usage,
	}, nil
}

func extractJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return trimmed[start : end+1]
	}
	return trimmed
}

func toFusionSummaries(results []fusionPerspectiveResult) []fusionSummary {
	summaries := make([]fusionSummary, 0, len(results))
	for _, result := range results {
		summaries = append(summaries, fusionSummary{
			Role:        result.Role,
			Summary:     result.Summary,
			Objections:  result.Objections,
			Confidence:  result.Confidence,
			EvidenceIDs: result.EvidenceIDs,
		})
	}
	return summaries
}

func (h Handler) insertFusionRun(ctx context.Context, run fusionRunRecord) error {
	if run.WorkflowType == "" {
		run.WorkflowType = "single_model"
	}
	_, err := h.db.ExecContext(ctx, `
INSERT INTO fusion_runs (
  id, user_id, conversation_id, user_message_id, assistant_message_id, model_id, reasoning_effort, status, search_budget, searches_used, sources_read, last_error, workflow_type, source_model_ids_json, fusion_model_id, grounding_enabled, fusion_config_json, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`, run.ID, run.UserID, run.ConversationID, run.UserMessageID, run.AssistantMessageID, run.ModelID, nullableString(run.ReasoningEffort), run.Status, run.SearchBudget, run.SearchesUsed, run.SourcesRead, nullableString(run.LastError), run.WorkflowType, nullableString(run.SourceModelIDsJSON), nullableString(run.FusionModelID), run.GroundingEnabled, nullableString(run.FusionConfigJSON))
	return err
}

func (h Handler) claimFusionRun(ctx context.Context, runID string) (*fusionRunRecord, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	run := fusionRunRecord{}
	err = tx.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, user_message_id, assistant_message_id, model_id, COALESCE(reasoning_effort, ''), status, search_budget, searches_used, sources_read, COALESCE(last_error, ''), workflow_type, COALESCE(source_model_ids_json, ''), COALESCE(fusion_model_id, ''), grounding_enabled, COALESCE(fusion_config_json, '')
FROM fusion_runs
WHERE id = ?
LIMIT 1;
`, runID).Scan(&run.ID, &run.UserID, &run.ConversationID, &run.UserMessageID, &run.AssistantMessageID, &run.ModelID, &run.ReasoningEffort, &run.Status, &run.SearchBudget, &run.SearchesUsed, &run.SourcesRead, &run.LastError, &run.WorkflowType, &run.SourceModelIDsJSON, &run.FusionModelID, &run.GroundingEnabled, &run.FusionConfigJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.Status != "queued" {
		return nil, nil
	}
	result, err := tx.ExecContext(ctx, `
UPDATE fusion_runs
SET status = 'running', started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status = 'queued';
`, runID)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	run.Status = "running"
	return &run, nil
}

func (h Handler) failFusionRun(ctx context.Context, runID, message string) error {
	run, err := h.loadFusionRun(ctx, runID)
	if err == nil && run != nil {
		traceCollector, traceErr := h.loadMessageTrace(ctx, run.UserID, run.AssistantMessageID)
		if traceErr != nil {
			traceCollector = newThinkingTraceCollector()
		}
		traceCollector.MarkStopped(message)
		if run.WorkflowType == "multi_model" {
			_ = h.updateMultiModelFusionAssistantMessage(ctx, run.UserID, run.AssistantMessageID, message, traceCollector, nil, nil, nil, nil, "", []string{message}, runID, thinkingTraceStatusStopped)
			_ = h.persistFusionRunPublicStatus(ctx, runID, FusionRunStatusResponse{
				ID:       runID,
				Status:   "failed",
				Warnings: []string{message},
			})
		} else {
			_ = h.updateFusionAssistantMessage(ctx, run.UserID, run.AssistantMessageID, message, traceCollector, nil, nil, nil, thinkingTraceStatusStopped)
		}
	}
	_, err = h.db.ExecContext(ctx, `
UPDATE fusion_runs
SET status = 'failed', last_error = ?, finished_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`, message, runID)
	return err
}

func (h Handler) finishFusionRun(ctx context.Context, runID, status, lastError string, searchesUsed, sourcesRead int) error {
	_, err := h.db.ExecContext(ctx, `
UPDATE fusion_runs
SET status = ?, last_error = ?, searches_used = ?, sources_read = ?, finished_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`, status, nullableString(lastError), searchesUsed, sourcesRead, runID)
	return err
}

func (h Handler) loadFusionRun(ctx context.Context, runID string) (*fusionRunRecord, error) {
	run := fusionRunRecord{}
	err := h.db.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, user_message_id, assistant_message_id, model_id, COALESCE(reasoning_effort, ''), status, search_budget, searches_used, sources_read, COALESCE(last_error, ''), workflow_type, COALESCE(source_model_ids_json, ''), COALESCE(fusion_model_id, ''), grounding_enabled, COALESCE(fusion_config_json, '')
FROM fusion_runs
WHERE id = ?
LIMIT 1;
`, runID).Scan(&run.ID, &run.UserID, &run.ConversationID, &run.UserMessageID, &run.AssistantMessageID, &run.ModelID, &run.ReasoningEffort, &run.Status, &run.SearchBudget, &run.SearchesUsed, &run.SourcesRead, &run.LastError, &run.WorkflowType, &run.SourceModelIDsJSON, &run.FusionModelID, &run.GroundingEnabled, &run.FusionConfigJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (h Handler) loadMessageTrace(ctx context.Context, userID, messageID string) (*thinkingTraceCollector, error) {
	var raw sql.NullString
	err := h.db.QueryRowContext(ctx, `
SELECT thinking_trace_json
FROM messages
WHERE id = ? AND user_id = ?
LIMIT 1;
`, messageID, userID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	collector := newThinkingTraceCollector()
	if trace, ok := decodeThinkingTraceJSON(raw.String); ok {
		collector.trace = *trace
	}
	return collector, nil
}

func (h Handler) updateFusionAssistantMessage(ctx context.Context, userID, messageID, content string, traceCollector *thinkingTraceCollector, citations []citationResponse, usage *messageUsage, summaries []fusionSummary, desiredStatus string) error {
	trace := traceCollector.Snapshot()
	if trace != nil && desiredStatus != "" {
		trace.Status = desiredStatus
	}
	traceJSON, err := encodeThinkingTraceJSON(trace)
	if err != nil {
		return err
	}
	summariesJSON, err := encodeFusionSummariesJSON(summaries)
	if err != nil {
		return err
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE messages
SET content = ?, thinking_trace_json = ?, fusion_summaries_json = ?,
    prompt_tokens = COALESCE(?, prompt_tokens),
    completion_tokens = COALESCE(?, completion_tokens),
    total_tokens = COALESCE(?, total_tokens),
    reasoning_tokens = COALESCE(?, reasoning_tokens),
    cost_microusd = COALESCE(?, cost_microusd),
    byok_inference_cost_microusd = COALESCE(?, byok_inference_cost_microusd),
    tokens_per_second = COALESCE(?, tokens_per_second),
    usage_model_id = COALESCE(?, usage_model_id),
    usage_provider_name = COALESCE(?, usage_provider_name),
    response_mode = 'fusion'
WHERE id = ? AND user_id = ?;
`, content, traceJSON, summariesJSON,
		nullableUsageInt(usage, func(u *messageUsage) *int { return &u.PromptTokens }),
		nullableUsageInt(usage, func(u *messageUsage) *int { return &u.CompletionTokens }),
		nullableUsageInt(usage, func(u *messageUsage) *int { return &u.TotalTokens }),
		nullableUsageInt(usage, func(u *messageUsage) *int { return u.ReasoningTokens }),
		nullableUsageInt(usage, func(u *messageUsage) *int { return u.CostMicrosUSD }),
		nullableUsageInt(usage, func(u *messageUsage) *int { return u.ByokInferenceCostMicrosUSD }),
		nullableUsageFloat(usage, func(u *messageUsage) *float64 { return u.TokensPerSecond }),
		nullableUsageString(usage, func(u *messageUsage) string { return u.ModelID }),
		nullableUsageString(usage, func(u *messageUsage) string { return u.ProviderName }),
		messageID, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM citations WHERE message_id = ?;`, messageID); err != nil {
		return err
	}
	for _, citation := range citations {
		if strings.TrimSpace(citation.URL) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO citations (id, message_id, url, title, snippet, source_provider)
VALUES (?, ?, ?, ?, ?, ?);
`, uuid.NewString(), messageID, citation.URL, nullableString(citation.Title), nullableString(citation.Snippet), nullableString(citation.SourceProvider)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableUsageInt(usage *messageUsage, selector func(*messageUsage) *int) any {
	if usage == nil {
		return nil
	}
	value := selector(usage)
	if value == nil {
		return nil
	}
	return *value
}

func nullableUsageFloat(usage *messageUsage, selector func(*messageUsage) *float64) any {
	if usage == nil {
		return nil
	}
	value := selector(usage)
	if value == nil {
		return nil
	}
	return *value
}

func nullableUsageString(usage *messageUsage, selector func(*messageUsage) string) any {
	if usage == nil {
		return nil
	}
	value := strings.TrimSpace(selector(usage))
	if value == "" {
		return nil
	}
	return value
}

func (h Handler) availableFusionSearchBudget(ctx context.Context) (int, error) {
	used, err := h.currentBraveMonthlyUsage(ctx)
	if err != nil {
		return 0, err
	}
	available := h.cfg.BraveMonthlyQueryLimit - h.cfg.BraveMonthlyQueryReserve - used
	if available < 0 {
		available = 0
	}
	if available > h.cfg.FusionHardMaxSearchQueries {
		available = h.cfg.FusionHardMaxSearchQueries
	}
	return available, nil
}

func (h Handler) currentBraveMonthlyUsage(ctx context.Context) (int, error) {
	var used int
	err := h.db.QueryRowContext(ctx, `
SELECT queries_used
FROM brave_monthly_usage
WHERE provider = ? AND month_key = ?
LIMIT 1;
`, braveProviderName, currentMonthKey(time.Now().UTC())).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}

func currentMonthKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

type distributedBraveSearcher struct {
	handler      *Handler
	runID        string
	runBudget    int
	searchesUsed int
}

func (s *distributedBraveSearcher) Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error) {
	if s.searchesUsed >= s.runBudget {
		return nil, errors.New("fusion search budget exhausted")
	}
	if err := s.handler.acquireDistributedBraveLease(ctx, braveProviderName, braveFreeTierSpacing); err != nil {
		return nil, err
	}
	if err := s.handler.consumeBraveMonthlyQuery(ctx); err != nil {
		return nil, err
	}
	s.searchesUsed++
	results, err := s.handler.grounding.Search(ctx, query, count)
	if err != nil {
		_ = s.handler.recordFusionRunSearches(ctx, s.runID, s.searchesUsed)
		return nil, err
	}
	_ = s.handler.recordFusionRunSearches(ctx, s.runID, s.searchesUsed)
	return results, nil
}

func (h Handler) recordFusionRunSearches(ctx context.Context, runID string, searchesUsed int) error {
	_, err := h.db.ExecContext(ctx, `
UPDATE fusion_runs
SET searches_used = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`, searchesUsed, runID)
	return err
}

func (h Handler) consumeBraveMonthlyQuery(ctx context.Context) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	monthKey := currentMonthKey(time.Now().UTC())
	if _, err := tx.ExecContext(ctx, `
INSERT INTO brave_monthly_usage (provider, month_key, queries_used, updated_at)
VALUES (?, ?, 0, CURRENT_TIMESTAMP)
ON CONFLICT(provider, month_key) DO NOTHING;
`, braveProviderName, monthKey); err != nil {
		return err
	}
	maxAllowed := h.cfg.BraveMonthlyQueryLimit - h.cfg.BraveMonthlyQueryReserve
	if maxAllowed < 0 {
		maxAllowed = 0
	}
	result, err := tx.ExecContext(ctx, `
UPDATE brave_monthly_usage
SET queries_used = queries_used + 1, updated_at = CURRENT_TIMESTAMP
WHERE provider = ? AND month_key = ? AND queries_used < ?;
`, braveProviderName, monthKey, maxAllowed)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("brave monthly search reserve exhausted")
	}
	return tx.Commit()
}

func (h Handler) acquireDistributedBraveLease(ctx context.Context, provider string, minInterval time.Duration) error {
	if minInterval <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		tx, err := h.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO brave_rate_limits (provider, next_allowed_at, updated_at)
VALUES (?, NULL, CURRENT_TIMESTAMP)
ON CONFLICT(provider) DO NOTHING;
`, provider); err != nil {
			tx.Rollback()
			return err
		}

		var nextAllowedAt sql.NullString
		if err := tx.QueryRowContext(ctx, `
SELECT next_allowed_at
FROM brave_rate_limits
WHERE provider = ?
LIMIT 1;
`, provider).Scan(&nextAllowedAt); err != nil {
			tx.Rollback()
			return err
		}

		now := time.Now().UTC()
		leaseUntil := now.Add(minInterval)
		if !nextAllowedAt.Valid || strings.TrimSpace(nextAllowedAt.String) == "" {
			if _, err := tx.ExecContext(ctx, `
UPDATE brave_rate_limits
SET next_allowed_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE provider = ?;
`, leaseUntil.Format(time.RFC3339Nano), provider); err != nil {
				tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			return nil
		}

		existing, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(nextAllowedAt.String))
		if err != nil {
			existing = time.Time{}
		}
		if existing.IsZero() || !existing.After(now) {
			if _, err := tx.ExecContext(ctx, `
UPDATE brave_rate_limits
SET next_allowed_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE provider = ?;
`, leaseUntil.Format(time.RFC3339Nano), provider); err != nil {
				tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			return nil
		}

		waitFor := time.Until(existing)
		if err := tx.Commit(); err != nil {
			return err
		}
		if err := waitWithContext(ctx, waitFor); err != nil {
			return err
		}
	}
}

func (h Handler) updateMultiModelFusionAssistantMessage(
	ctx context.Context,
	userID string,
	messageID string,
	content string,
	traceCollector *thinkingTraceCollector,
	citations []citationResponse,
	usage *usageResponse,
	fusionSources []FusionSourceResult,
	fusionAnalysis *FusionAnalysis,
	resultModelID string,
	warnings []string,
	fusionRunID string,
	desiredStatus string,
) error {
	trace := traceCollector.Snapshot()
	if trace != nil && desiredStatus != "" {
		trace.Status = desiredStatus
	}
	traceJSON, err := encodeThinkingTraceJSON(trace)
	if err != nil {
		return err
	}

	var sourcesJSON any
	if len(fusionSources) > 0 {
		sourcesBytes, _ := json.Marshal(fusionSources)
		sourcesJSON = string(sourcesBytes)
	}

	var analysisJSON any
	if fusionAnalysis != nil {
		analysisBytes, _ := json.Marshal(fusionAnalysis)
		analysisJSON = string(analysisBytes)
	}

	var resultUsageJSON any
	if usage != nil {
		usageBytes, _ := json.Marshal(usage)
		resultUsageJSON = string(usageBytes)
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE messages
SET content = COALESCE(NULLIF(?, ''), content),
    thinking_trace_json = ?,
    fusion_sources_json = COALESCE(?, fusion_sources_json),
    fusion_analysis_json = COALESCE(?, fusion_analysis_json),
    fusion_result_model_id = COALESCE(?, fusion_result_model_id),
    fusion_result_usage_json = COALESCE(?, fusion_result_usage_json),
    prompt_tokens = COALESCE(?, prompt_tokens),
    completion_tokens = COALESCE(?, completion_tokens),
    total_tokens = COALESCE(?, total_tokens),
    reasoning_tokens = COALESCE(?, reasoning_tokens),
    cost_microusd = COALESCE(?, cost_microusd),
    byok_inference_cost_microusd = COALESCE(?, byok_inference_cost_microusd),
    tokens_per_second = COALESCE(?, tokens_per_second),
    usage_model_id = COALESCE(?, usage_model_id),
    usage_provider_name = COALESCE(?, usage_provider_name),
    fusion_run_id = COALESCE(?, fusion_run_id),
    response_mode = 'fusion'
WHERE id = ? AND user_id = ?;
`, content, traceJSON, sourcesJSON, analysisJSON, nullableString(resultModelID), resultUsageJSON,
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return &u.PromptTokens }),
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return &u.CompletionTokens }),
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return &u.TotalTokens }),
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return u.ReasoningTokens }),
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return u.CostMicrosUSD }),
		nullableUsageResponseInt(usage, func(u *usageResponse) *int { return u.ByokInferenceCostMicrosUSD }),
		nullableUsageResponseFloat(usage, func(u *usageResponse) *float64 { return u.TokensPerSecond }),
		nullableUsageResponseString(usage, func(u *usageResponse) string { return u.ModelID }),
		nullableUsageResponseString(usage, func(u *usageResponse) string { return u.ProviderName }),
		nullableString(fusionRunID), messageID, userID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM citations WHERE message_id = ?;`, messageID); err != nil {
		return err
	}
	for _, citation := range citations {
		if strings.TrimSpace(citation.URL) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO citations (id, message_id, url, title, snippet, source_provider)
VALUES (?, ?, ?, ?, ?, ?);
`, uuid.NewString(), messageID, citation.URL, nullableString(citation.Title), nullableString(citation.Snippet), nullableString(citation.SourceProvider)); err != nil {
			return err
		}
	}

	_ = warnings

	return tx.Commit()
}

func nullableUsageResponseInt(usage *usageResponse, selector func(*usageResponse) *int) any {
	if usage == nil {
		return nil
	}
	value := selector(usage)
	if value == nil {
		return nil
	}
	return *value
}

func nullableUsageResponseFloat(usage *usageResponse, selector func(*usageResponse) *float64) any {
	if usage == nil {
		return nil
	}
	value := selector(usage)
	if value == nil {
		return nil
	}
	return *value
}

func nullableUsageResponseString(usage *usageResponse, selector func(*usageResponse) string) any {
	if usage == nil {
		return nil
	}
	value := strings.TrimSpace(selector(usage))
	if value == "" {
		return nil
	}
	return value
}

func (h Handler) streamMultiModelFusionQueuedResponse(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, input multiModelFusionQueuedStreamInput) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	runID := uuid.NewString()

	metadataEvent := map[string]any{
		"type":           "metadata",
		"grounding":      input.Grounding,
		"deepResearch":   false,
		"responseMode":   "fusion",
		"modelId":        input.FusionModel.ModelID,
		"conversationId": input.ConversationID,
		"userMessageId":  input.UserMessageID,
		"fusionRunId":    runID,
	}
	if input.FusionModel.ReasoningEffort != "" {
		metadataEvent["reasoningEffort"] = input.FusionModel.ReasoningEffort
	}
	_ = writeSSEEvent(w, metadataEvent)

	traceCollector := newThinkingTraceCollector()
	initialProgress := summarizedProgress(research.Progress{
		Phase:   research.PhasePlanning,
		Message: "Queueing fusion workflow",
	}, research.ProgressSummaryInput{
		Phase: research.PhasePlanning,
	})
	traceCollector.AppendProgress(initialProgress)
	_ = writeSSEEvent(w, progressEventData(initialProgress))
	flusher.Flush()

	assistantMessageID, err := h.insertMessageWithCitations(
		ctx,
		input.UserID,
		input.ConversationID,
		"assistant",
		"",
		"",
		input.FusionModel.ModelID,
		input.Grounding,
		false,
		"fusion",
		nil,
		traceCollector.Snapshot(),
		nil,
		nil,
	)
	if err != nil {
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to create fusion placeholder"})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		return
	}

	searchBudget := 0
	if input.Grounding {
		var budgetErr error
		searchBudget, budgetErr = h.availableFusionSearchBudget(ctx)
		if budgetErr != nil {
			_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to reserve Brave search budget"})
			_ = writeSSEEvent(w, map[string]any{"type": "done"})
			flusher.Flush()
			_ = h.updateMultiModelFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "Fusion mode is temporarily unavailable while Brave quota is being checked.", traceCollector, nil, nil, nil, nil, "", nil, runID, thinkingTraceStatusStopped)
			return
		}
	}

	minRequiredBudget := len(input.SourceModels) * 1
	if input.Grounding && searchBudget < minRequiredBudget {
		if err := h.insertFusionRun(ctx, fusionRunRecord{
			ID:                 runID,
			UserID:             input.UserID,
			ConversationID:     input.ConversationID,
			UserMessageID:      input.UserMessageID,
			AssistantMessageID: assistantMessageID,
			ModelID:            input.FusionModel.ModelID,
			ReasoningEffort:    input.FusionModel.ReasoningEffort,
			Status:             "failed",
			SearchBudget:       searchBudget,
			LastError:          "Not enough Brave monthly budget remains for fusion mode.",
			WorkflowType:       "multi_model",
		}); err != nil {
			log.Printf("fusion run persist failed: %v", err)
		}
		_ = writeSSEEvent(w, map[string]any{
			"type":    "warning",
			"scope":   "fusion",
			"message": "Fusion mode needs search budget, but the remaining monthly Brave budget is too low right now.",
		})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		_ = h.updateMultiModelFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "Fusion mode could not start because the remaining Brave free-tier budget is below the minimum search reserve.", traceCollector, nil, nil, nil, nil, "", nil, runID, thinkingTraceStatusStopped)
		return
	}

	configJSON, _ := json.Marshal(FusionRunConfig{
		SourceModels: input.SourceModels,
		FusionModel:  input.FusionModel,
		Grounding:    input.Grounding,
	})

	var sourceModelIDs []string
	for _, sm := range input.SourceModels {
		sourceModelIDs = append(sourceModelIDs, sm.ModelID)
	}
	sourceModelIDsJSON, _ := json.Marshal(sourceModelIDs)

	if err := h.insertFusionRun(ctx, fusionRunRecord{
		ID:                 runID,
		UserID:             input.UserID,
		ConversationID:     input.ConversationID,
		UserMessageID:      input.UserMessageID,
		AssistantMessageID: assistantMessageID,
		ModelID:            input.FusionModel.ModelID,
		ReasoningEffort:    input.FusionModel.ReasoningEffort,
		Status:             "queued",
		SearchBudget:       searchBudget,
		WorkflowType:       "multi_model",
		SourceModelIDsJSON: string(sourceModelIDsJSON),
		FusionModelID:      input.FusionModel.ModelID,
		GroundingEnabled:   input.Grounding,
		FusionConfigJSON:   string(configJSON),
	}); err != nil {
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": "failed to queue fusion run"})
		_ = writeSSEEvent(w, map[string]any{"type": "done"})
		flusher.Flush()
		return
	}

	queuedProgress := summarizedProgress(research.Progress{
		Phase:   research.PhasePlanning,
		Message: "Queued fusion workflow",
	}, research.ProgressSummaryInput{
		Phase: research.PhasePlanning,
	})
	traceCollector.AppendProgress(queuedProgress)
	_ = writeSSEEvent(w, progressEventData(queuedProgress))
	_ = writeSSEEvent(w, map[string]any{"type": "done"})
	flusher.Flush()

	if err := h.updateMultiModelFusionAssistantMessage(ctx, input.UserID, assistantMessageID, "", traceCollector, nil, nil, nil, nil, "", nil, runID, thinkingTraceStatusRunning); err != nil {
		log.Printf("fusion placeholder update failed: run_id=%s err=%v", runID, err)
	}
	if err := h.enqueueFusionRun(context.Background(), runID); err != nil {
		log.Printf("fusion enqueue failed: run_id=%s err=%v", runID, err)
		_ = h.failFusionRun(context.Background(), runID, "failed to enqueue fusion run")
	}
}

func (h Handler) GetFusionRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "run id is required")
		return
	}

	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}

	row := h.db.QueryRowContext(r.Context(), `
		SELECT id, status, source_results_json, fusion_analysis_json, fusion_result_json, public_status_json, completed_sources, degraded_sources, failed_sources
		FROM fusion_runs 
		WHERE id = ? AND user_id = ?
	`, runID, user.ID)

	var (
		id                string
		status            string
		sourceResultsJSON sql.NullString
		analysisJSON      sql.NullString
		resultJSON        sql.NullString
		publicStatusJSON  sql.NullString
		completedSources  int
		degradedSources   int
		failedSources     int
	)

	if err := row.Scan(&id, &status, &sourceResultsJSON, &analysisJSON, &resultJSON, &publicStatusJSON, &completedSources, &degradedSources, &failedSources); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "fusion run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db_error", "failed to query fusion run")
		return
	}

	var resp FusionRunStatusResponse
	if publicStatusJSON.Valid && publicStatusJSON.String != "" {
		_ = json.Unmarshal([]byte(publicStatusJSON.String), &resp)
	}
	resp.ID = id
	resp.Status = status
	if len(resp.SourceResults) == 0 && sourceResultsJSON.Valid && sourceResultsJSON.String != "" {
		_ = json.Unmarshal([]byte(sourceResultsJSON.String), &resp.SourceResults)
	}
	if resp.Analysis == nil && analysisJSON.Valid && analysisJSON.String != "" {
		_ = json.Unmarshal([]byte(analysisJSON.String), &resp.Analysis)
	}
	if resp.Result == nil && resultJSON.Valid && resultJSON.String != "" {
		_ = json.Unmarshal([]byte(resultJSON.String), &resp.Result)
	}
	resp.CompletedSources = completedSources
	resp.DegradedSources = degradedSources
	resp.FailedSources = failedSources

	writeJSON(w, http.StatusOK, resp)
}

func (h Handler) persistFusionRunPublicStatus(ctx context.Context, runID string, status FusionRunStatusResponse) error {
	var existingRaw sql.NullString
	if err := h.db.QueryRowContext(ctx, `SELECT public_status_json FROM fusion_runs WHERE id = ?`, runID).Scan(&existingRaw); err == nil && existingRaw.Valid && existingRaw.String != "" {
		var existing FusionRunStatusResponse
		if json.Unmarshal([]byte(existingRaw.String), &existing) == nil {
			if len(status.SourceResults) == 0 {
				status.SourceResults = existing.SourceResults
			}
			if status.Analysis == nil {
				status.Analysis = existing.Analysis
			}
			if status.Result == nil {
				status.Result = existing.Result
			}
			if len(status.Warnings) == 0 {
				status.Warnings = existing.Warnings
			}
			if status.CompletedSources == 0 {
				status.CompletedSources = existing.CompletedSources
			}
			if status.DegradedSources == 0 {
				status.DegradedSources = existing.DegradedSources
			}
			if status.FailedSources == 0 {
				status.FailedSources = existing.FailedSources
			}
		}
	}
	status.ID = runID
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = h.db.ExecContext(ctx, `
UPDATE fusion_runs
SET public_status_json = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`, string(payload), runID)
	return err
}
