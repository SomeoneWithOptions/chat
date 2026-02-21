package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type PromptResponder interface {
	Respond(ctx context.Context, prompt string) (string, error)
}

type JSONPlanner struct {
	responder PromptResponder
	fallback  HeuristicPlanner
}

func NewJSONPlanner(responder PromptResponder) JSONPlanner {
	return JSONPlanner{responder: responder, fallback: HeuristicPlanner{}}
}

func (p JSONPlanner) InitialPlan(ctx context.Context, input PlannerInput) (PlannerDecision, error) {
	prompt := buildPlannerPrompt(input)
	decision, err := p.fromResponder(ctx, prompt)
	if err == nil {
		if decision.NextAction == NextActionSearchMore && len(decision.Queries) == 0 {
			decision.Queries = buildFallbackQueries(input.Question, input.TimeSensitive, input.Loop, max(1, input.MaxQueries-input.UsedQueries))
		}
		return decision, nil
	}
	fallback := p.fallback.InitialPlan(input)
	fallback.Reason = strings.TrimSpace(fallback.Reason + "; fallback used")
	return fallback, nil
}

func (p JSONPlanner) EvaluateEvidence(ctx context.Context, input PlannerInput) (PlannerDecision, error) {
	prompt := buildEvaluationPrompt(input)
	decision, err := p.fromResponder(ctx, prompt)
	if err == nil {
		if decision.NextAction == NextActionSearchMore && len(decision.Queries) == 0 {
			decision.Queries = buildFallbackQueries(input.Question, input.TimeSensitive, input.Loop+1, max(1, input.MaxQueries-input.UsedQueries))
		}
		return decision, nil
	}
	fallback := p.fallback.EvaluateEvidence(input)
	fallback.Reason = strings.TrimSpace(fallback.Reason + "; fallback used")
	return fallback, nil
}

func (p JSONPlanner) fromResponder(ctx context.Context, prompt string) (PlannerDecision, error) {
	if p.responder == nil {
		return PlannerDecision{}, errors.New("planner responder unavailable")
	}
	raw, err := p.responder.Respond(ctx, prompt)
	if err != nil {
		return PlannerDecision{}, err
	}
	decision, err := parsePlannerDecision(raw)
	if err != nil {
		return PlannerDecision{}, err
	}
	return decision, nil
}

type HeuristicPlanner struct{}

func (HeuristicPlanner) InitialPlan(input PlannerInput) PlannerDecision {
	queries := buildFallbackQueries(input.Question, input.TimeSensitive, input.Loop, input.MaxQueries)
	return PlannerDecision{
		NextAction:   NextActionSearchMore,
		Queries:      queries,
		CoverageGaps: []string{"Need authoritative sources that directly address the question"},
		Confidence:   0.2,
		Reason:       "initial evidence set is empty",
	}
}

func (HeuristicPlanner) EvaluateEvidence(input PlannerInput) PlannerDecision {
	evidenceCount := len(input.Evidence)
	fullTextCount := 0
	hasContradiction := false
	hasFreshSignal := false

	for _, item := range input.Evidence {
		if item.HasFullText {
			fullTextCount++
		}
		if item.Contradiction {
			hasContradiction = true
		}
		if hasFreshnessSignal(item.Title + " " + item.Snippet + " " + item.Excerpt) {
			hasFreshSignal = true
		}
	}

	if input.TimeSensitive {
		hasFreshSignal = hasFreshSignal || evidenceHasCurrentYear(input.Evidence)
	}

	isEnough := evidenceCount >= 3 && fullTextCount >= 1
	if input.TimeSensitive {
		isEnough = isEnough && hasFreshSignal
	}
	if hasContradiction {
		isEnough = false
	}

	if isEnough {
		return PlannerDecision{
			NextAction:   NextActionFinalize,
			CoverageGaps: nil,
			Confidence:   0.72,
			Reason:       "evidence appears sufficient for synthesis",
		}
	}

	if input.Loop >= input.MaxLoops || input.UsedQueries >= input.MaxQueries || input.RemainingReadBudget <= 0 {
		return PlannerDecision{
			NextAction:   NextActionFinalize,
			CoverageGaps: []string{"Evidence may be incomplete due to budget limits"},
			Confidence:   0.38,
			Reason:       "budget limits reached",
		}
	}

	queries := buildFallbackQueries(input.Question, input.TimeSensitive, input.Loop+1, max(1, input.MaxQueries-input.UsedQueries))
	gaps := []string{
		"Need stronger corroboration from independent sources",
	}
	if input.TimeSensitive && !hasFreshSignal {
		gaps = append(gaps, "Need explicit publication/update dates for recency-sensitive claims")
	}
	if hasContradiction {
		gaps = append(gaps, "Need tie-breaker sources to resolve conflicting signals")
	}

	return PlannerDecision{
		NextAction:        NextActionSearchMore,
		Queries:           queries,
		CoverageGaps:      gaps,
		TargetSourceTypes: []string{"official docs", "news", "standards"},
		Confidence:        0.34,
		Reason:            "evidence not yet sufficient",
	}
}

func parsePlannerDecision(raw string) (PlannerDecision, error) {
	jsonRaw := extractJSONBlock(raw)
	if jsonRaw == "" {
		return PlannerDecision{}, errors.New("planner response did not include json")
	}
	decoder := json.NewDecoder(strings.NewReader(jsonRaw))
	decoder.DisallowUnknownFields()

	var decision PlannerDecision
	if err := decoder.Decode(&decision); err != nil {
		return PlannerDecision{}, err
	}
	if decision.NextAction != NextActionSearchMore && decision.NextAction != NextActionFinalize {
		return PlannerDecision{}, errors.New("planner nextAction must be search_more or finalize")
	}
	if decision.Confidence < 0 {
		decision.Confidence = 0
	}
	if decision.Confidence > 1 {
		decision.Confidence = 1
	}
	decision.Queries = dedupeQueries(decision.Queries)
	decision.CoverageGaps = dedupeStrings(decision.CoverageGaps)
	decision.TargetSourceTypes = dedupeStrings(decision.TargetSourceTypes)
	decision.Reason = strings.TrimSpace(decision.Reason)
	return decision, nil
}

func buildFallbackQueries(question string, timeSensitive bool, loop, budget int) []string {
	maxQueries := clampInt(budget, 1, 4)
	if loop <= 0 {
		loop = 1
	}
	queries := buildPassQueries(question, timeSensitive, maxQueries, maxQueries)
	for i := range queries {
		queries[i] = strings.TrimSpace(queries[i])
	}
	if len(queries) == 0 {
		base := strings.TrimSpace(question)
		if base != "" {
			queries = []string{base}
		}
	}
	return dedupeQueries(queries)
}

func dedupeQueries(queries []string) []string {
	if len(queries) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(queries))
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func extractJSONBlock(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		return value
	}
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(value[start : end+1])
}

func evidenceHasCurrentYear(items []Evidence) bool {
	for _, item := range items {
		if hasFreshnessSignal(item.Title + " " + item.Snippet + " " + item.Excerpt) {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
