package research

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"chat/backend/internal/brave"
)

type Orchestrator struct {
	planner  Planner
	searcher Searcher
	reader   Reader
	cfg      OrchestratorConfig
}

func NewOrchestrator(searcher Searcher, planner Planner, reader Reader, cfg OrchestratorConfig) Orchestrator {
	if planner == nil {
		planner = NewJSONPlanner(nil)
	}
	if cfg.MaxLoops < 1 {
		cfg.MaxLoops = 1
	}
	if cfg.MaxSearchQueries < 1 {
		cfg.MaxSearchQueries = 1
	}
	if cfg.MaxSourcesRead < 1 {
		cfg.MaxSourcesRead = 1
	}
	if cfg.MaxCitations < 1 {
		cfg.MaxCitations = defaultMaxCitations
	}
	if cfg.SearchResultsPerQ < 1 {
		cfg.SearchResultsPerQ = defaultResultsPerPass
	}
	if cfg.SourceFetchTimeout <= 0 {
		cfg.SourceFetchTimeout = defaultSourceFetchTimeout
	}
	if cfg.SourceMaxBytes <= 0 {
		cfg.SourceMaxBytes = defaultSourceMaxBytes
	}

	return Orchestrator{
		planner:  planner,
		searcher: searcher,
		reader:   reader,
		cfg:      cfg,
	}
}

func (o Orchestrator) Run(ctx context.Context, question string, timeSensitive bool, onProgress func(Progress)) (OrchestratorResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		return OrchestratorResult{StopReason: StopReasonSufficient}, nil
	}

	runCtx := ctx
	cancel := func() {}
	if o.cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, o.cfg.Timeout)
	}
	defer cancel()

	if o.searcher == nil {
		warning := "Grounding is unavailable for this response."
		return OrchestratorResult{
			Warnings:      []string{warning},
			Warning:       warning,
			StopReason:    StopReasonError,
			SearchQueries: 0,
		}, nil
	}

	pool := NewEvidencePool()
	warnings := make([]string, 0, 4)
	previousQueries := make([]string, 0, o.cfg.MaxSearchQueries)
	coverageGaps := make([]string, 0, 4)
	usedQueries := 0
	sourcesRead := 0
	sourcesConsidered := 0
	lastSearchAttemptAt := time.Time{}
	stopReason := StopReasonBudgetExhausted
	loopsExecuted := 0

	for loop := 1; loop <= o.cfg.MaxLoops; loop++ {
		loopsExecuted = loop
		if err := runCtx.Err(); err != nil {
			return o.resultWithStop(pool, loop-1, usedQueries, sourcesConsidered, sourcesRead, warnings, StopReasonTimeout), err
		}

		rankedEvidence := pool.Rank()
		planInput := PlannerInput{
			Question:            trimmedQuestion,
			TimeSensitive:       timeSensitive,
			Loop:                loop,
			MaxLoops:            o.cfg.MaxLoops,
			UsedQueries:         usedQueries,
			MaxQueries:          o.cfg.MaxSearchQueries,
			RemainingReadBudget: o.cfg.MaxSourcesRead - sourcesRead,
			CoverageGaps:        coverageGaps,
			PreviousQueries:     previousQueries,
			Evidence:            rankedEvidence,
		}

		emitProgress(onProgress, WithProgressSummary(Progress{
			Phase:             PhasePlanning,
			Message:           fmt.Sprintf("Planning loop %d of %d", loop, o.cfg.MaxLoops),
			Loop:              loop,
			MaxLoops:          o.cfg.MaxLoops,
			Pass:              loop,
			TotalPasses:       o.cfg.MaxLoops,
			SourcesRead:       sourcesRead,
			SourcesConsidered: sourcesConsidered,
		}, ProgressSummaryInput{
			Phase: PhasePlanning,
		}))

		decision, err := o.planner.InitialPlan(runCtx, planInput)
		decisionUsedFallback := false
		if loop > 1 {
			decision, err = o.planner.EvaluateEvidence(runCtx, planInput)
		}
		if err != nil {
			warnings = appendUniqueWarning(warnings, "Planner failed; continuing with bounded fallback strategy.")
			decision = HeuristicPlanner{}.EvaluateEvidence(planInput)
			if loop == 1 {
				decision = HeuristicPlanner{}.InitialPlan(planInput)
			}
			decisionUsedFallback = true
		}
		if len(decision.CoverageGaps) > 0 {
			coverageGaps = decision.CoverageGaps
		}

		if decision.NextAction == NextActionFinalize && len(rankedEvidence) > 0 {
			stopReason = StopReasonSufficient
			break
		}

		if usedQueries >= o.cfg.MaxSearchQueries {
			stopReason = StopReasonBudgetExhausted
			break
		}

		queries := dedupeQueries(decision.Queries)
		if len(queries) == 0 {
			queries = buildFallbackQueries(trimmedQuestion, timeSensitive, loop, 1)
		}
		remainingQueries := o.cfg.MaxSearchQueries - usedQueries
		if len(queries) > remainingQueries {
			queries = queries[:remainingQueries]
		}
		if len(queries) == 0 {
			stopReason = StopReasonBudgetExhausted
			break
		}

		emitProgress(onProgress, WithProgressSummary(Progress{
			Phase:             PhaseSearching,
			Message:           fmt.Sprintf("Searching %d query variants", len(queries)),
			Loop:              loop,
			MaxLoops:          o.cfg.MaxLoops,
			Pass:              loop,
			TotalPasses:       o.cfg.MaxLoops,
			SourcesRead:       sourcesRead,
			SourcesConsidered: sourcesConsidered,
		}, ProgressSummaryInput{
			Phase:        PhaseSearching,
			QueryCount:   len(queries),
			Decision:     DecisionFromNextAction(decision.NextAction),
			UsedFallback: decisionUsedFallback,
		}))

		candidates := make([]Citation, 0, len(queries)*o.cfg.SearchResultsPerQ)
		for _, query := range queries {
			if err := waitBeforeSearchAttempt(runCtx, &lastSearchAttemptAt, o.cfg.MinSearchInterval); err != nil {
				return o.resultWithStop(pool, loop-1, usedQueries, sourcesConsidered, sourcesRead, warnings, StopReasonTimeout), err
			}

			usedQueries++
			previousQueries = append(previousQueries, query)
			results, searchErr := o.searcher.Search(runCtx, query, o.cfg.SearchResultsPerQ)
			lastSearchAttemptAt = time.Now()
			if searchErr != nil && isBraveRateLimitError(searchErr) {
				retryDelay := o.cfg.MinSearchInterval
				if retryDelay <= 0 {
					retryDelay = defaultRateLimitRetryDelay
				}
				if waitErr := waitForRetry(runCtx, retryDelay); waitErr != nil {
					return o.resultWithStop(pool, loop-1, usedQueries, sourcesConsidered, sourcesRead, warnings, StopReasonTimeout), waitErr
				}
				if err := waitBeforeSearchAttempt(runCtx, &lastSearchAttemptAt, o.cfg.MinSearchInterval); err != nil {
					return o.resultWithStop(pool, loop-1, usedQueries, sourcesConsidered, sourcesRead, warnings, StopReasonTimeout), err
				}
				results, searchErr = o.searcher.Search(runCtx, query, o.cfg.SearchResultsPerQ)
				lastSearchAttemptAt = time.Now()
			}
			if searchErr != nil {
				statusCode := 0
				var apiErr brave.APIError
				if errors.As(searchErr, &apiErr) {
					statusCode = apiErr.StatusCode
				}
				log.Printf(
					"research search failed: loop=%d query_chars=%d status_code=%d err=%v",
					loop,
					len([]rune(strings.TrimSpace(query))),
					statusCode,
					searchErr,
				)
				if errors.Is(searchErr, brave.ErrMissingAPIKey) {
					warnings = appendUniqueWarning(warnings, "Grounding is unavailable because BRAVE_API_KEY is not configured.")
				} else {
					warnings = appendUniqueWarning(warnings, "A web search pass failed; continuing with available evidence.")
				}
				continue
			}

			ranked := rankSearchResults(query, loop, timeSensitive, results)
			for _, candidate := range ranked {
				pool.AddSearchCandidate(candidate, timeSensitive)
				candidates = append(candidates, candidate)
			}
		}

		remainingReadBudget := o.cfg.MaxSourcesRead - sourcesRead
		if remainingReadBudget <= 0 {
			stopReason = StopReasonBudgetExhausted
			break
		}

		candidates = dedupeCandidateCitations(candidates)
		if len(candidates) == 0 {
			if len(pool.Rank()) > 0 {
				stopReason = StopReasonSufficient
				break
			}
			continue
		}

		if len(candidates) > remainingReadBudget {
			candidates = candidates[:remainingReadBudget]
		}

		emitProgress(onProgress, WithProgressSummary(Progress{
			Phase:             PhaseReading,
			Message:           fmt.Sprintf("Reading %d candidate sources", len(candidates)),
			Loop:              loop,
			MaxLoops:          o.cfg.MaxLoops,
			Pass:              loop,
			TotalPasses:       o.cfg.MaxLoops,
			SourcesRead:       sourcesRead,
			SourcesConsidered: sourcesConsidered + len(candidates),
		}, ProgressSummaryInput{
			Phase:          PhaseReading,
			CandidateCount: len(candidates),
			Decision:       DecisionFromNextAction(decision.NextAction),
		}))

		for _, candidate := range candidates {
			if pool.HasRead(candidate.URL) {
				continue
			}
			sourcesConsidered++
			if o.reader == nil {
				continue
			}
			readResult, readErr := o.reader.Read(runCtx, candidate.URL)
			if readErr != nil {
				warnings = appendUniqueWarning(warnings, "A source could not be read; continuing with search snippets.")
				continue
			}
			pool.AddReadResult(candidate, readResult, timeSensitive)
			sourcesRead++
			if sourcesRead >= o.cfg.MaxSourcesRead {
				break
			}
		}

		emitProgress(onProgress, WithProgressSummary(Progress{
			Phase:             PhaseEvaluating,
			Message:           "Evaluating evidence sufficiency",
			Loop:              loop,
			MaxLoops:          o.cfg.MaxLoops,
			Pass:              loop,
			TotalPasses:       o.cfg.MaxLoops,
			SourcesRead:       sourcesRead,
			SourcesConsidered: sourcesConsidered,
		}, ProgressSummaryInput{
			Phase: PhaseEvaluating,
		}))

		evalInput := PlannerInput{
			Question:             trimmedQuestion,
			TimeSensitive:        timeSensitive,
			Loop:                 loop,
			MaxLoops:             o.cfg.MaxLoops,
			UsedQueries:          usedQueries,
			MaxQueries:           o.cfg.MaxSearchQueries,
			RemainingReadBudget:  o.cfg.MaxSourcesRead - sourcesRead,
			CoverageGaps:         coverageGaps,
			PreviousQueries:      previousQueries,
			Evidence:             pool.Rank(),
			LatestReadCandidates: candidates,
		}
		evalDecision, evalErr := o.planner.EvaluateEvidence(runCtx, evalInput)
		evalUsedFallback := false
		if evalErr != nil {
			warnings = appendUniqueWarning(warnings, "Evidence evaluation fallback was used.")
			evalDecision = HeuristicPlanner{}.EvaluateEvidence(evalInput)
			evalUsedFallback = true
		}
		if len(evalDecision.CoverageGaps) > 0 {
			coverageGaps = evalDecision.CoverageGaps
		}

		if evalDecision.NextAction == NextActionFinalize {
			stopReason = StopReasonSufficient
			break
		}

		emitProgress(onProgress, WithProgressSummary(Progress{
			Phase:             PhaseIterating,
			Message:           fmt.Sprintf("Continuing to loop %d", loop+1),
			Loop:              loop,
			MaxLoops:          o.cfg.MaxLoops,
			Pass:              loop,
			TotalPasses:       o.cfg.MaxLoops,
			SourcesRead:       sourcesRead,
			SourcesConsidered: sourcesConsidered,
		}, ProgressSummaryInput{
			Phase:        PhaseIterating,
			Decision:     DecisionFromNextAction(evalDecision.NextAction),
			UsedFallback: evalUsedFallback,
		}))
	}

	return o.resultWithStop(pool, loopsExecuted, usedQueries, sourcesConsidered, sourcesRead, warnings, stopReason), nil
}

func (o Orchestrator) resultWithStop(
	pool *EvidencePool,
	loops,
	searchQueries,
	sourcesConsidered,
	sourcesRead int,
	warnings []string,
	stop StopReason,
) OrchestratorResult {
	ranked := pool.Rank()
	if o.cfg.MaxCitations > 0 && len(ranked) > o.cfg.MaxCitations {
		ranked = ranked[:o.cfg.MaxCitations]
	}
	citations := make([]Citation, 0, len(ranked))
	for _, item := range ranked {
		citations = append(citations, item.Citation)
	}

	result := OrchestratorResult{
		Loops:             loops,
		SearchQueries:     searchQueries,
		SourcesConsidered: sourcesConsidered,
		SourcesRead:       sourcesRead,
		Citations:         citations,
		Evidence:          ranked,
		Warnings:          warnings,
		StopReason:        stop,
	}
	if len(warnings) > 0 {
		result.Warning = warnings[0]
	}
	if stop == "" {
		result.StopReason = StopReasonBudgetExhausted
	}
	return result
}

func dedupeCandidateCitations(citations []Citation) []Citation {
	if len(citations) == 0 {
		return nil
	}
	deduped := make([]Citation, 0, len(citations))
	seen := make(map[string]struct{}, len(citations))
	for _, citation := range citations {
		key := canonicalOrRawURL(citation.URL)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, citation)
	}
	sort.SliceStable(deduped, func(i, j int) bool {
		if deduped[i].Score == deduped[j].Score {
			if deduped[i].Pass == deduped[j].Pass {
				return deduped[i].URL < deduped[j].URL
			}
			return deduped[i].Pass < deduped[j].Pass
		}
		return deduped[i].Score > deduped[j].Score
	})
	return deduped
}

func appendUniqueWarning(warnings []string, warning string) []string {
	trimmed := strings.TrimSpace(warning)
	if trimmed == "" {
		return warnings
	}
	for _, existing := range warnings {
		if strings.EqualFold(strings.TrimSpace(existing), trimmed) {
			return warnings
		}
	}
	return append(warnings, trimmed)
}

func emitProgress(onProgress func(Progress), progress Progress) {
	if onProgress == nil {
		return
	}
	log.Printf(
		"research progress ux: phase=%s loop=%d max_loops=%d sources_read=%d sources_considered=%d title=%q has_detail=%t is_quick_step=%t decision=%s",
		progress.Phase,
		progress.Loop,
		progress.MaxLoops,
		progress.SourcesRead,
		progress.SourcesConsidered,
		progress.Title,
		strings.TrimSpace(progress.Detail) != "",
		progress.IsQuickStep,
		progress.Decision,
	)
	onProgress(progress)
}
