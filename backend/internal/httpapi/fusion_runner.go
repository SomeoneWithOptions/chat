package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
)

type fusionStatusCounts struct {
	completed int
	degraded  int
	failed    int
}

func (h Handler) executeMultiModelFusionRun(ctx context.Context, run *fusionRunRecord) error {
	var config FusionRunConfig
	if err := json.Unmarshal([]byte(run.FusionConfigJSON), &config); err != nil {
		return h.failFusionRun(ctx, run.ID, "invalid fusion config")
	}

	traceCollector, err := h.loadMessageTrace(ctx, run.UserID, run.AssistantMessageID)
	if err != nil {
		traceCollector = newThinkingTraceCollector()
	}

	fusionSources := make([]FusionSourceResult, len(config.SourceModels))
	for i, sm := range config.SourceModels {
		fusionSources[i] = FusionSourceResult{
			ModelID: sm.ModelID,
			Status:  "queued",
		}
	}

	var fusionAnalysis *FusionAnalysis
	var finalResult *FusionFinalResult
	var warnings []string

	saveState := func(runStatus, desiredTraceStatus string, finished bool) {
		warnings = buildFusionWarnings(fusionSources, config.Grounding, h.cfg.FusionTargetReadableSourcesPerModel)
		content := ""
		resultModelID := ""
		var resultUsage *usageResponse
		if finalResult != nil {
			content = finalResult.Response
			resultModelID = finalResult.ModelID
			resultUsage = finalResult.Usage
		}
		_ = h.updateMultiModelFusionAssistantMessage(
			ctx,
			run.UserID,
			run.AssistantMessageID,
			content,
			traceCollector,
			collectFusionCitations(fusionSources),
			resultUsage,
			fusionSources,
			fusionAnalysis,
			resultModelID,
			warnings,
			run.ID,
			desiredTraceStatus,
		)
		_ = h.persistFusionRunState(ctx, run.ID, runStatus, fusionSources, fusionAnalysis, finalResult, warnings, finished)
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhasePlanning,
		Title:  "Starting fusion workflow",
		Detail: "Preparing the sequential fusion run",
	})
	saveState("running", thinkingTraceStatusRunning, false)

	historyMessages, err := h.listConversationPromptMessages(ctx, run.UserID, run.ConversationID, maxConversationHistoryMessages)
	if err != nil {
		return h.failFusionRun(ctx, run.ID, "failed to load conversation history")
	}

	userPrompt := ""
	for idx := len(historyMessages) - 1; idx >= 0; idx-- {
		if historyMessages[idx].Role == "user" {
			userPrompt = historyMessages[idx].Content
			break
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return h.failFusionRun(ctx, run.ID, "failed to resolve prompt")
	}

	attachedFiles, err := h.listFilesForMessage(ctx, run.UserID, run.UserMessageID)
	if err != nil {
		log.Printf("fusion attachment lookup failed: run_id=%s message_id=%s err=%v", run.ID, run.UserMessageID, err)
	} else if len(attachedFiles) > 0 {
		userPrompt = h.appendFileContextToPrompt(userPrompt, attachedFiles)
	}
	timeSensitive := isTimeSensitivePrompt(userPrompt)

	searchCoordinator := newFusionGroundingCoordinator(&h, run.ID, run.SearchBudget, h.cfg.FusionSearchResultsPerQuery)

	successCount := 0

	for i, sm := range config.SourceModels {
		traceCollector.AppendProgress(research.Progress{
			Phase:       research.PhaseSearching,
			Title:       "Running source model",
			Detail:      fmt.Sprintf("%s (%d of %d)", sm.ModelID, i+1, len(config.SourceModels)),
			Pass:        i + 1,
			TotalPasses: len(config.SourceModels),
		})
		fusionSources[i].Status = "running"
		saveState("running", thinkingTraceStatusRunning, false)

		start := time.Now()
		res, runErr := h.runMultiModelFusionSource(ctx, sm, userPrompt, timeSensitive, config.Grounding, historyMessages, searchCoordinator)
		duration := time.Since(start).Milliseconds()

		if runErr != nil {
			fusionSources[i].Status = "failed"
			fusionSources[i].Error = runErr.Error()
			fusionSources[i].DurationMs = duration
		} else {
			fusionSources[i].Status = res.Status
			fusionSources[i].Response = res.Response
			fusionSources[i].ReasoningContent = res.ReasoningContent
			fusionSources[i].Citations = res.Citations
			fusionSources[i].Usage = res.Usage
			fusionSources[i].SearchQueries = res.SearchQueries
			fusionSources[i].ReadableSources = res.ReadableSources
			fusionSources[i].Warnings = res.Warnings
			fusionSources[i].DurationMs = duration
			if res.Status == "complete" || res.Status == "degraded" {
				successCount++
			}
		}

		saveState("running", thinkingTraceStatusRunning, false)
	}

	if successCount == 0 {
		traceCollector.AppendProgress(research.Progress{
			Phase:  research.PhaseEvaluating,
			Title:  "Fusion failed",
			Detail: "Every source model failed before producing a usable answer.",
		})
		saveState("running", thinkingTraceStatusStopped, false)
		return h.failFusionRun(ctx, run.ID, "all source models failed")
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseEvaluating,
		Title:  "Running fusion analysis",
		Detail: "Comparing the completed source passes",
	})
	saveState("running", thinkingTraceStatusRunning, false)

	analysis, err := h.runFusionAnalysis(ctx, config.FusionModel, userPrompt, fusionSources, historyMessages)
	if err != nil {
		log.Printf("fusion analysis failed: %v", err)
	} else {
		fusionAnalysis = analysis
		saveState("running", thinkingTraceStatusRunning, false)
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseSynthesizing,
		Title:  "Writing final fused answer",
		Detail: "Combining the source passes into one answer",
	})
	saveState("running", thinkingTraceStatusRunning, false)

	finalResult, err = h.runMultiModelFusionSynthesis(ctx, config.FusionModel, userPrompt, fusionSources, fusionAnalysis, historyMessages)
	if err != nil {
		return h.failFusionRun(ctx, run.ID, "fusion synthesis failed")
	}
	if shouldWarnFusionEvidenceQuality(fusionSources, config.Grounding, h.cfg.FusionTargetReadableSourcesPerModel) {
		finalResult.Response = "Warning: evidence quality was below target because no grounded source model reached the readable-source goal.\n\n" + strings.TrimSpace(finalResult.Response)
	}

	traceCollector.AppendProgress(research.Progress{
		Phase:  research.PhaseFinalizing,
		Title:  "Finalizing fusion run",
		Detail: "Saving final payload",
	})
	traceCollector.MarkDone()
	saveState("completed", thinkingTraceStatusDone, true)

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

func (h Handler) runMultiModelFusionSource(
	ctx context.Context,
	spec FusionSourceSpec,
	prompt string,
	timeSensitive bool,
	grounding bool,
	history []openrouter.Message,
	searchCoordinator *fusionGroundingCoordinator,
) (*FusionSourceResult, error) {
	sourceCtx := ctx
	cancel := func() {}
	if h.cfg.FusionSourceTimeoutSeconds > 0 {
		sourceCtx, cancel = context.WithTimeout(ctx, time.Duration(h.cfg.FusionSourceTimeoutSeconds)*time.Second)
	}
	defer cancel()

	if !grounding {
		answer, reasoningContent, usage, err := h.runFusionSynthesis(sourceCtx, spec.ModelID, spec.ReasoningEffort, prompt, history, nil, nil)
		if err != nil {
			return nil, err
		}
		return &FusionSourceResult{
			Status:           "complete",
			Response:         answer,
			ReasoningContent: reasoningContent,
			Usage:            convertOpenRouterUsageToResponse(usage),
		}, nil
	}

	groundingResult, err := h.runFusionSinglePassGrounding(sourceCtx, prompt, timeSensitive, searchCoordinator)
	if err != nil {
		return nil, err
	}

	status := "complete"
	if groundingResult.ReadableSources < h.cfg.FusionTargetReadableSourcesPerModel {
		status = "degraded"
	}

	answer, reasoningContent, usage, synthErr := h.runFusionSynthesis(sourceCtx, spec.ModelID, spec.ReasoningEffort, prompt, history, groundingResult.Citations, nil)
	if synthErr != nil {
		return nil, fmt.Errorf("source synthesis failed: %w", synthErr)
	}

	return &FusionSourceResult{
		Status:           status,
		Response:         answer,
		ReasoningContent: reasoningContent,
		Citations:        groundingResult.Citations,
		Usage:            convertOpenRouterUsageToResponse(usage),
		SearchQueries:    groundingResult.SearchQueries,
		ReadableSources:  groundingResult.ReadableSources,
		Warnings:         groundingResult.Warnings,
	}, nil
}

func summarizeFusionSourceStatuses(sources []FusionSourceResult) fusionStatusCounts {
	counts := fusionStatusCounts{}
	for _, source := range sources {
		switch source.Status {
		case "complete":
			counts.completed++
		case "degraded":
			counts.degraded++
		case "failed":
			counts.failed++
		}
	}
	return counts
}

func shouldWarnFusionEvidenceQuality(sources []FusionSourceResult, grounding bool, targetReadableSources int) bool {
	if !grounding {
		return false
	}
	counts := summarizeFusionSourceStatuses(sources)
	if counts.completed > 0 {
		return false
	}
	return counts.degraded > 0 || counts.failed > 0
}

func buildFusionWarnings(sources []FusionSourceResult, grounding bool, targetReadableSources int) []string {
	warnings := make([]string, 0, len(sources)+1)
	for _, source := range sources {
		warnings = append(warnings, source.Warnings...)
	}
	if shouldWarnFusionEvidenceQuality(sources, grounding, targetReadableSources) {
		warnings = append(warnings, fmt.Sprintf("Evidence quality was below target: no grounded source model reached %d readable sources.", targetReadableSources))
	}
	return dedupeStrings(warnings)
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func collectFusionCitations(sources []FusionSourceResult) []citationResponse {
	allCitations := make([]citationResponse, 0, len(sources)*4)
	seen := make(map[string]struct{}, len(sources)*4)
	for _, src := range sources {
		for _, citation := range src.Citations {
			if strings.TrimSpace(citation.URL) == "" {
				continue
			}
			if _, ok := seen[citation.URL]; ok {
				continue
			}
			seen[citation.URL] = struct{}{}
			allCitations = append(allCitations, citation)
		}
	}
	return allCitations
}

func buildFusionPublicStatus(runID, runStatus string, sources []FusionSourceResult, analysis *FusionAnalysis, result *FusionFinalResult, warnings []string) FusionRunStatusResponse {
	counts := summarizeFusionSourceStatuses(sources)
	return FusionRunStatusResponse{
		ID:               runID,
		Status:           runStatus,
		SourceResults:    sources,
		Analysis:         analysis,
		Result:           result,
		Warnings:         warnings,
		CompletedSources: counts.completed,
		DegradedSources:  counts.degraded,
		FailedSources:    counts.failed,
	}
}

func (h Handler) persistFusionRunState(ctx context.Context, runID, runStatus string, sources []FusionSourceResult, analysis *FusionAnalysis, result *FusionFinalResult, warnings []string, finished bool) error {
	publicStatus := buildFusionPublicStatus(runID, runStatus, sources, analysis, result, warnings)
	publicStatusJSON, err := json.Marshal(publicStatus)
	if err != nil {
		return err
	}
	counts := summarizeFusionSourceStatuses(sources)
	query := `
UPDATE fusion_runs
SET status = ?,
    source_results_json = COALESCE(?, source_results_json),
    fusion_analysis_json = COALESCE(?, fusion_analysis_json),
    fusion_result_json = COALESCE(?, fusion_result_json),
    completed_sources = ?,
    degraded_sources = ?,
    failed_sources = ?,
    public_status_json = ?,
    finished_at = CASE WHEN ? THEN CURRENT_TIMESTAMP ELSE finished_at END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`
	_, err = h.db.ExecContext(
		ctx,
		query,
		runStatus,
		toJSONStr(sources),
		toJSONStr(analysis),
		toJSONStr(result),
		counts.completed,
		counts.degraded,
		counts.failed,
		string(publicStatusJSON),
		finished,
		runID,
	)
	return err
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

func (h Handler) runFusionAnalysis(
	ctx context.Context,
	fusionModel FusionSourceSpec,
	prompt string,
	sources []FusionSourceResult,
	history []openrouter.Message,
) (*FusionAnalysis, error) {
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

	var analysis FusionAnalysis
	if err := json.Unmarshal([]byte(contentStr), &analysis); err != nil {
		return nil, err
	}

	return &analysis, nil
}

func (h Handler) runMultiModelFusionSynthesis(
	ctx context.Context,
	fusionModel FusionSourceSpec,
	prompt string,
	sources []FusionSourceResult,
	analysis *FusionAnalysis,
	history []openrouter.Message,
) (*FusionFinalResult, error) {
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

	return &FusionFinalResult{
		ModelID:          fusionModel.ModelID,
		Response:         content,
		ReasoningContent: result.reasoning,
		Usage:            convertOpenRouterUsageToResponse(result.usage),
	}, nil
}
