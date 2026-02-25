package httpapi

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"chat/backend/internal/research"
)

func (h Handler) buildResearchConfig(profile research.ModeProfile) research.OrchestratorConfig {
	overrides := research.OrchestratorConfig{}
	switch profile {
	case research.ModeDeepResearch:
		overrides.MaxLoops = h.cfg.DeepResearchMaxLoops
		overrides.MaxSourcesRead = h.cfg.DeepResearchMaxSourcesRead
		overrides.MaxSearchQueries = h.cfg.DeepResearchMaxSearchQ
		overrides.MaxCitations = h.cfg.ResearchMaxCitationsDeep
		overrides.Timeout = time.Duration(h.cfg.DeepResearchTimeoutSeconds) * time.Second
		overrides.MinSearchInterval = braveFreeTierSpacing
	case research.ModeChat:
		fallthrough
	default:
		overrides.MaxLoops = h.cfg.ChatResearchMaxLoops
		overrides.MaxSourcesRead = h.cfg.ChatResearchMaxSourcesRead
		overrides.MaxSearchQueries = h.cfg.ChatResearchMaxSearchQ
		overrides.MaxCitations = h.cfg.ResearchMaxCitationsChat
		overrides.Timeout = time.Duration(h.cfg.ChatResearchTimeoutSeconds) * time.Second
	}
	overrides.SourceFetchTimeout = time.Duration(h.cfg.ResearchSourceTimeoutSecs) * time.Second
	overrides.SourceMaxBytes = int64(h.cfg.ResearchSourceMaxBytes)

	return research.ResolveProfile(profile, overrides)
}

func (h Handler) runResearchOrchestrator(
	ctx context.Context,
	profile research.ModeProfile,
	question string,
	timeSensitive bool,
	onProgress func(research.Progress),
) (research.OrchestratorResult, error) {
	cfg := h.buildResearchConfig(profile)
	planner := research.NewJSONPlanner(h.researchPlannerResponder)
	orchestrator := research.NewOrchestrator(h.grounding, planner, h.researchReader, cfg)
	result, err := orchestrator.Run(ctx, question, timeSensitive, onProgress)

	readSuccessRate := 0.0
	if result.ReadAttempts > 0 {
		successes := result.ReadAttempts - result.ReadFailures
		if successes < 0 {
			successes = 0
		}
		readSuccessRate = float64(successes) / float64(result.ReadAttempts)
	}

	log.Printf(
		"research orchestrator completed: profile=%s loops=%d searches=%d sources_considered=%d sources_read=%d read_attempts=%d read_failures=%d read_success_rate=%.2f read_failure_reasons=%q stop_reason=%s warning_present=%t err_present=%t",
		profile,
		result.Loops,
		result.SearchQueries,
		result.SourcesConsidered,
		result.SourcesRead,
		result.ReadAttempts,
		result.ReadFailures,
		readSuccessRate,
		formatTopReadFailureReasons(result.ReadFailureReasons, 3),
		result.StopReason,
		researchWarning(result) != "",
		err != nil,
	)

	return result, err
}

func researchWarning(result research.OrchestratorResult) string {
	if len(result.Warnings) > 0 {
		return strings.TrimSpace(result.Warnings[0])
	}
	return strings.TrimSpace(result.Warning)
}

func convertResearchCitations(citations []research.Citation, max int) []citationResponse {
	if len(citations) == 0 {
		return nil
	}
	if max > 0 && len(citations) > max {
		citations = citations[:max]
	}
	converted := make([]citationResponse, 0, len(citations))
	for _, item := range citations {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		converted = append(converted, citationResponse{
			URL:            strings.TrimSpace(item.URL),
			Title:          trimToRunes(strings.TrimSpace(item.Title), 240),
			Snippet:        trimToRunes(strings.TrimSpace(item.Snippet), 800),
			SourceProvider: fallback(item.SourceProvider, "brave"),
		})
	}
	return converted
}

func formatTopReadFailureReasons(reasons map[string]int, limit int) string {
	if len(reasons) == 0 || limit <= 0 {
		return ""
	}

	type pair struct {
		reason string
		count  int
	}
	items := make([]pair, 0, len(reasons))
	for reason, count := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" || count <= 0 {
			continue
		}
		items = append(items, pair{reason: reason, count: count})
	}
	if len(items) == 0 {
		return ""
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].reason < items[j].reason
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s:%d", item.reason, item.count))
	}
	return strings.Join(parts, ",")
}
