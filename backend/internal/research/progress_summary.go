package research

import "strings"

type ProgressDecision string

const (
	ProgressDecisionSearchMore ProgressDecision = "search_more"
	ProgressDecisionFinalize   ProgressDecision = "finalize"
	ProgressDecisionFallback   ProgressDecision = "fallback"
)

type ProgressSummary struct {
	Title       string
	Detail      string
	IsQuickStep bool
	Decision    ProgressDecision
}

type ProgressSummaryInput struct {
	Phase          Phase
	Message        string
	QueryCount     int
	CandidateCount int
	Decision       ProgressDecision
	UsedFallback   bool
}

func BuildProgressSummary(input ProgressSummaryInput) ProgressSummary {
	summary := ProgressSummary{}

	switch input.Phase {
	case PhasePlanning:
		summary.Title = "Planning next step"
		summary.Detail = "Checking what evidence is still missing"
	case PhaseSearching:
		if input.QueryCount <= 1 && input.QueryCount > 0 {
			summary.Title = "Getting grounding results"
			summary.IsQuickStep = true
		} else {
			summary.Title = "Searching trusted sources"
			summary.Detail = "Searching trusted sources for corroboration"
		}
	case PhaseReading:
		summary.Title = "Reading selected sources"
		summary.Detail = "Using top-ranked pages to improve accuracy"
		if input.CandidateCount == 1 {
			summary.IsQuickStep = true
		}
	case PhaseEvaluating:
		summary.Title = "Checking evidence quality"
		summary.Detail = "Deciding whether we can answer confidently"
	case PhaseIterating:
		summary.Title = "Running another pass"
		summary.Detail = "Need one more search to close gaps"
		summary.Decision = ProgressDecisionSearchMore
	case PhaseSynthesizing:
		summary.Title = "Drafting response"
		summary.Detail = "Grounding claims to collected sources"
	case PhaseFinalizing:
		summary.Title = "Finalizing answer"
		summary.Detail = "Ordering citations and sending response"
		summary.Decision = ProgressDecisionFinalize
	default:
		summary.Title = strings.TrimSpace(input.Message)
	}

	if summary.Title == "" {
		summary.Title = "Working on your request"
	}
	if summary.IsQuickStep {
		summary.Detail = ""
	}

	if input.UsedFallback {
		summary.Decision = ProgressDecisionFallback
	} else if input.Decision != "" {
		summary.Decision = input.Decision
	}

	return summary
}

func WithProgressSummary(progress Progress, summaryInput ProgressSummaryInput) Progress {
	if summaryInput.Phase == "" {
		summaryInput.Phase = progress.Phase
	}
	if strings.TrimSpace(summaryInput.Message) == "" {
		summaryInput.Message = progress.Message
	}

	summary := BuildProgressSummary(summaryInput)
	progress.Title = summary.Title
	progress.Detail = summary.Detail
	progress.IsQuickStep = summary.IsQuickStep
	progress.Decision = summary.Decision
	return progress
}

func DecisionFromNextAction(nextAction NextAction) ProgressDecision {
	switch nextAction {
	case NextActionFinalize:
		return ProgressDecisionFinalize
	case NextActionSearchMore:
		return ProgressDecisionSearchMore
	default:
		return ""
	}
}
