package httpapi

import (
	"strings"

	"chat/backend/internal/research"
)

func progressEventData(progress research.Progress) map[string]any {
	event := map[string]any{
		"type":  "progress",
		"phase": progress.Phase,
	}

	if message := strings.TrimSpace(progress.Message); message != "" {
		event["message"] = message
	}
	if progress.Pass > 0 {
		event["pass"] = progress.Pass
	}
	if progress.TotalPasses > 0 {
		event["totalPasses"] = progress.TotalPasses
	}
	if progress.Loop > 0 {
		event["loop"] = progress.Loop
	}
	if progress.MaxLoops > 0 {
		event["maxLoops"] = progress.MaxLoops
	}
	if progress.SourcesConsidered > 0 {
		event["sourcesConsidered"] = progress.SourcesConsidered
	}
	if progress.SourcesRead > 0 {
		event["sourcesRead"] = progress.SourcesRead
	}

	if title := strings.TrimSpace(progress.Title); title != "" {
		event["title"] = title
	}
	if detail := strings.TrimSpace(progress.Detail); detail != "" {
		event["detail"] = detail
	}
	if progress.IsQuickStep {
		event["isQuickStep"] = true
	}
	if progress.Decision != "" {
		event["decision"] = progress.Decision
	}

	return event
}

func summarizedProgress(progress research.Progress, summaryInput research.ProgressSummaryInput) research.Progress {
	return research.WithProgressSummary(progress, summaryInput)
}
