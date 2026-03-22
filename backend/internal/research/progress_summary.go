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
		summary.Title = "Mapping the request"
		summary.Detail = "Figuring out what needs to be answered before drafting anything."
	case PhaseSearching:
		if input.QueryCount <= 1 && input.QueryCount > 0 {
			summary.Title = "Finding an initial source"
			summary.IsQuickStep = true
		} else {
			summary.Title = "Scanning for trustworthy sources"
			summary.Detail = "Looking for current, relevant sources that can support the answer."
		}
	case PhaseReading:
		summary.Title = "Pulling facts from the strongest results"
		summary.Detail = "Reading the most promising pages and extracting usable evidence."
		if input.CandidateCount == 1 {
			summary.IsQuickStep = true
		}
	case PhaseEvaluating:
		summary.Title = "Comparing what holds up"
		summary.Detail = "Checking whether the sources agree and whether the evidence is strong enough."
	case PhaseIterating:
		summary.Title = "Closing the remaining gaps"
		summary.Detail = "Running another pass where the answer still needs more support."
		summary.Decision = ProgressDecisionSearchMore
	case PhaseSynthesizing:
		summary.Title = "Shaping the response"
		summary.Detail = "Turning the research into a clear response with grounded claims."
	case PhaseFinalizing:
		summary.Title = "Polishing the final answer"
		summary.Detail = "Tightening wording, organizing citations, and preparing the response."
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
