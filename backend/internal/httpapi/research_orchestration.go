package httpapi

import (
	"context"
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
	return orchestrator.Run(ctx, question, timeSensitive, onProgress)
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
