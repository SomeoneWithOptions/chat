package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
)

// The overall executeCouncilRun orchestrator
func (h Handler) executeCouncilRun(ctx context.Context, run *agentRunRecord) error {
	traceCollector, err := h.loadMessageTrace(ctx, run.UserID, run.AssistantMessageID)
	if err != nil {
		traceCollector = newThinkingTraceCollector()
	}

	var agentSources []CouncilSourceResult
	var agentAnalysis *CouncilAnalysis
	var resultModelID string
	var agentRunID = run.ID

	// Helper to persist state quickly
	saveState := func(status string) {
		_ = h.updateCouncilAssistantMessage(
			ctx, run.UserID, run.AssistantMessageID, "", traceCollector, nil, nil, agentSources, agentAnalysis, resultModelID, agentRunID, status,
		)
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhasePlanning,
		Title:  "Starting council workflow",
		Detail: "Initializing multi-model generation",
	})
	saveState(thinkingTraceStatusRunning)

	var config CouncilRunConfig
	if err := json.Unmarshal([]byte(run.CouncilConfigJSON), &config); err != nil {
		return h.failAgentRun(ctx, run.ID, "invalid council config")
	}

	historyMessages, err := h.listConversationPromptMessages(ctx, run.UserID, run.ConversationID, maxConversationHistoryMessages)
	if err != nil {
		return h.failAgentRun(ctx, run.ID, "failed to load conversation history")
	}

	userPrompt := ""
	for idx := len(historyMessages) - 1; idx >= 0; idx-- {
		if historyMessages[idx].Role == "user" {
			userPrompt = historyMessages[idx].Content
			break
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return h.failAgentRun(ctx, run.ID, "failed to resolve prompt")
	}

	attachedFiles, err := h.listFilesForMessage(ctx, run.UserID, run.UserMessageID)
	if err != nil {
		log.Printf("council attachment lookup failed: run_id=%s message_id=%s err=%v", run.ID, run.UserMessageID, err)
	} else if len(attachedFiles) > 0 {
		userPrompt = h.appendFileContextToPrompt(userPrompt, attachedFiles)
	}
	timeSensitive := isTimeSensitivePrompt(userPrompt)

	// Build the globally serialized Brave search coordinator for the council
	searchCoordinator := newCouncilGroundingCoordinator(&h, run.ID, run.SearchBudget)

	// Fan out to each source model
	var wg sync.WaitGroup
	var mu sync.Mutex

	agentSources = make([]CouncilSourceResult, len(config.SourceModels))

	for i, sm := range config.SourceModels {
		agentSources[i] = CouncilSourceResult{
			ModelID: sm.ModelID,
			Status:  "queued",
		}
	}
	saveState(thinkingTraceStatusRunning)

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseSearching,
		Title:  "Running source models",
		Detail: fmt.Sprintf("Fanning out %d queries...", len(config.SourceModels)),
	})
	saveState(thinkingTraceStatusRunning)

	successCount := 0

	for i, sm := range config.SourceModels {
		wg.Add(1)
		go func(idx int, sourceModel CouncilSourceSpec) {
			defer wg.Done()

			// Update status to running
			mu.Lock()
			agentSources[idx].Status = "running"
			saveState(thinkingTraceStatusRunning)
			mu.Unlock()

			start := time.Now()
			res, err := h.runCouncilSourceModel(ctx, sourceModel, userPrompt, timeSensitive, config.Grounding, historyMessages, searchCoordinator)
			duration := time.Since(start).Milliseconds()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				agentSources[idx].Status = "failed"
				agentSources[idx].Error = err.Error()
				agentSources[idx].DurationMs = duration
			} else {
				agentSources[idx].Status = res.Status
				agentSources[idx].Response = res.Response
				agentSources[idx].ReasoningContent = res.ReasoningContent
				agentSources[idx].Citations = res.Citations
				agentSources[idx].Usage = res.Usage
				agentSources[idx].SearchQueries = res.SearchQueries
				agentSources[idx].ReadableSources = res.ReadableSources
				agentSources[idx].Warnings = res.Warnings
				agentSources[idx].DurationMs = duration

				if res.Status == "complete" || res.Status == "degraded" {
					successCount++
				}
			}
			saveState(thinkingTraceStatusRunning)
		}(i, sm)
	}

	wg.Wait()

	if successCount == 0 {
		traceCollector.AppendProgress(research.Progress{
			Phase:  research.PhaseEvaluating,
			Title:  "Council failed",
			Detail: "All source models failed to generate valid output.",
		})
		saveState(thinkingTraceStatusStopped)
		return h.failAgentRun(ctx, run.ID, "all source models failed")
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseEvaluating,
		Title:  "Running fusion analysis",
		Detail: "Comparing source model answers...",
	})
	saveState(thinkingTraceStatusRunning)

	analysis, err := h.runCouncilAnalysis(ctx, config.FusionModel, userPrompt, agentSources, historyMessages)
	if err != nil {
		log.Printf("council analysis failed: %v", err)
		// We still proceed even if analysis fails to avoid blocking the final answer.
	} else {
		agentAnalysis = analysis
		saveState(thinkingTraceStatusRunning)
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseSynthesizing,
		Title:  "Writing final fused answer",
		Detail: "Combining multi-model results",
	})
	saveState(thinkingTraceStatusRunning)

	finalResult, err := h.runCouncilSynthesis(ctx, config.FusionModel, userPrompt, agentSources, agentAnalysis, historyMessages)
	if err != nil {
		return h.failAgentRun(ctx, run.ID, "council synthesis failed")
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseFinalizing,
		Title:  "Finalizing council run",
		Detail: "Saving final payload",
	})
	traceCollector.MarkDone()

	// Compile all citations from all source models to save to the main message
	var allCitations []citationResponse
	seenCitations := make(map[string]bool)
	for _, src := range agentSources {
		for _, c := range src.Citations {
			if !seenCitations[c.URL] {
				seenCitations[c.URL] = true
				allCitations = append(allCitations, c)
			}
		}
	}

	_ = h.updateCouncilAssistantMessage(
		ctx, run.UserID, run.AssistantMessageID, finalResult.Response, traceCollector, allCitations, finalResult.Usage, agentSources, agentAnalysis, finalResult.ModelID, agentRunID, thinkingTraceStatusDone,
	)

	_, _ = h.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = 'completed', 
			source_results_json = ?, 
			fusion_analysis_json = ?, 
			fusion_result_json = ?, 
			finished_at = CURRENT_TIMESTAMP, 
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?;
	`, toJSONStr(agentSources), toJSONStr(agentAnalysis), toJSONStr(finalResult), run.ID)

	return nil
}

func toJSONStr(v any) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func (h Handler) runCouncilSourceModel(
	ctx context.Context,
	spec CouncilSourceSpec,
	prompt string,
	timeSensitive bool,
	grounding bool,
	history []openrouter.Message,
	searchCoordinator *councilGroundingCoordinator,
) (*CouncilSourceResult, error) {
	if !grounding {
		answer, reasoningContent, usage, err := h.runAgentSynthesis(ctx, spec.ModelID, spec.ReasoningEffort, prompt, history, nil, nil)
		if err != nil {
			return nil, err
		}
		return &CouncilSourceResult{
			Status:           "complete",
			Response:         answer,
			ReasoningContent: reasoningContent,
			Usage:            convertOpenRouterUsageToResponse(usage),
		}, nil
	}

	cfg := h.buildResearchConfig(research.ModeAgent)
	cfg.MinSearchQueries = 1
	cfg.MaxSearchQueries = 3
	cfg.MaxSourcesRead = 15
	cfg.SearchResultsPerQ = 15

	plannerResponder := newOpenRouterPlannerResponder(h.openrouter, spec.ModelID, plannerReasoningEffort(spec.ReasoningEffort))
	planner := research.NewJSONPlanner(plannerResponder)
	orchestrator := research.NewOrchestrator(searchCoordinator, planner, h.researchReader, cfg)

	result, err := orchestrator.Run(ctx, prompt, timeSensitive, func(progress research.Progress) {})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("council source model research failed: %v", err)
	}

	status := "complete"
	if result.SourcesRead < 15 {
		status = "degraded"
	}

	citations := convertResearchCitations(result.Citations, h.cfg.ResearchMaxCitationsDeep)
	roles, roleErr := h.runAgentRoleDebate(ctx, spec.ModelID, spec.ReasoningEffort, prompt, citations, nil)
	if roleErr != nil {
		return nil, errors.New("agent debate failed")
	}

	answer, reasoningContent, usage, synthErr := h.runAgentSynthesis(ctx, spec.ModelID, spec.ReasoningEffort, prompt, history, citations, roles)
	if synthErr != nil {
		return nil, errors.New("agent synthesis failed")
	}

	return &CouncilSourceResult{
		Status:           status,
		Response:         answer,
		ReasoningContent: reasoningContent,
		Citations:        citations,
		Usage:            convertOpenRouterUsageToResponse(usage),
		SearchQueries:    result.SearchQueries,
		ReadableSources:  result.SourcesRead,
		Warnings:         result.Warnings,
	}, nil
}

func convertOpenRouterUsageToResponse(u *openrouter.Usage) *usageResponse {
	if u == nil {
		return nil
	}
	return &usageResponse{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		ReasoningTokens:  u.ReasoningTokens,
		CostMicrosUSD:    u.CostMicrosUSD,
		TokensPerSecond:  u.TokensPerSecond,
		ModelID:          u.ModelID,
		ProviderName:     u.ProviderName,
	}
}

func (h Handler) runCouncilAnalysis(
	ctx context.Context,
	fusionModel CouncilSourceSpec,
	prompt string,
	sources []CouncilSourceResult,
	history []openrouter.Message,
) (*CouncilAnalysis, error) {
	var input strings.Builder
	input.WriteString("Compare the following answers provided by different AI models to the user's prompt.\n")
	input.WriteString("Extract the areas of agreement, key differences, partial coverage, unique insights, and blind spots.\n")
	input.WriteString("Output strict JSON matching the requested schema. Do NOT wrap in markdown blocks, just raw JSON.\n")
	input.WriteString(`Schema:
{
  "agreement": [{"point": "string", "sourceModels": ["modelId"]}],
  "keyDifferences": [{"topic": "string", "positions": [{"sourceModel": "modelId", "summary": "string"}]}],
  "partialCoverage": [{"point": "string", "sourceModels": ["modelId"]}],
  "uniqueInsights": [{"point": "string", "sourceModels": ["modelId"]}],
  "blindSpots": [{"point": "string"}]
}
`)
	input.WriteString("\nUser Prompt:\n")
	input.WriteString(prompt)
	input.WriteString("\n\nSource Model Answers:\n")
	for _, src := range sources {
		if src.Status != "complete" && src.Status != "degraded" {
			continue
		}
		input.WriteString(fmt.Sprintf("\n--- Source Model: %s (Status: %s) ---\n", src.ModelID, src.Status))
		input.WriteString(src.Response)
		input.WriteString("\n----------------------------------------\n")
	}

	messages := []openrouter.Message{
		{Role: "system", Content: "You are an analytical AI. Output valid JSON strictly following the requested schema. Do not use markdown formatting tags around the JSON."},
		{Role: "user", Content: input.String()},
	}

	content, _, err := h.collectChatCompletion(ctx, fusionModel.ModelID, fusionModel.ReasoningEffort, messages)
	if err != nil {
		return nil, err
	}

	contentStr := strings.TrimSpace(content)
	if strings.HasPrefix(contentStr, "```json") {
		contentStr = strings.TrimPrefix(contentStr, "```json")
		contentStr = strings.TrimSuffix(contentStr, "```")
	} else if strings.HasPrefix(contentStr, "```") {
		contentStr = strings.TrimPrefix(contentStr, "```")
		contentStr = strings.TrimSuffix(contentStr, "```")
	}

	var analysis CouncilAnalysis
	if err := json.Unmarshal([]byte(contentStr), &analysis); err != nil {
		return nil, err
	}

	return &analysis, nil
}

func (h Handler) runCouncilSynthesis(
	ctx context.Context,
	fusionModel CouncilSourceSpec,
	prompt string,
	sources []CouncilSourceResult,
	analysis *CouncilAnalysis,
	history []openrouter.Message,
) (*CouncilFinalResult, error) {
	var input strings.Builder
	input.WriteString("Write the best possible final answer for the user by fusing the following source model answers and analysis.\n")
	input.WriteString("Preserve important consensus points. Incorporate valuable unique insights where justified. Mention unresolved differences when they matter. Preserve citation discipline when grounding is enabled [1], [2]. Surface uncertainty honestly when the evidence is incomplete.\n\n")
	input.WriteString("User Prompt:\n")
	input.WriteString(prompt)

	if analysis != nil {
		analysisBytes, _ := json.MarshalIndent(analysis, "", "  ")
		input.WriteString("\n\nFusion Analysis:\n")
		input.WriteString(string(analysisBytes))
	}

	input.WriteString("\n\nSource Model Answers:\n")
	for _, src := range sources {
		if src.Status != "complete" && src.Status != "degraded" {
			continue
		}
		input.WriteString(fmt.Sprintf("\n--- Source Model: %s (Status: %s) ---\n", src.ModelID, src.Status))
		input.WriteString(src.Response)
		input.WriteString("\n----------------------------------------\n")
	}

	messages := []openrouter.Message{
		{Role: "system", Content: buildDeepResearchSystemPrompt(isTimeSensitivePrompt(prompt))},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, openrouter.Message{Role: "user", Content: input.String()})

	content, result, err := h.collectChatCompletion(ctx, fusionModel.ModelID, fusionModel.ReasoningEffort, messages)
	if err != nil {
		return nil, err
	}

	return &CouncilFinalResult{
		ModelID:          fusionModel.ModelID,
		Response:         content,
		ReasoningContent: result.reasoning,
		Usage:            convertOpenRouterUsageToResponse(result.usage),
	}, nil
}
