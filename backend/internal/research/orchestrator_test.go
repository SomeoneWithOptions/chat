package research

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"chat/backend/internal/brave"
)

type plannerStub struct {
	initial PlannerDecision
	eval    PlannerDecision
}

func (p plannerStub) InitialPlan(_ context.Context, _ PlannerInput) (PlannerDecision, error) {
	return p.initial, nil
}

func (p plannerStub) EvaluateEvidence(_ context.Context, _ PlannerInput) (PlannerDecision, error) {
	return p.eval, nil
}

type searcherStub struct {
	responses map[string][]brave.SearchResult
	err       error
	block     bool
}

func (s searcherStub) Search(ctx context.Context, query string, _ int) ([]brave.SearchResult, error) {
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.responses == nil {
		return nil, nil
	}
	return s.responses[query], nil
}

type readerStub struct {
	responses map[string]ReadResult
	errs      map[string]error
	err       error
}

func (r readerStub) Read(_ context.Context, rawURL string) (ReadResult, error) {
	if err, ok := r.errs[rawURL]; ok {
		return r.responses[rawURL], err
	}
	if r.err != nil {
		return ReadResult{}, r.err
	}
	if result, ok := r.responses[rawURL]; ok {
		return result, nil
	}
	return ReadResult{}, errors.New("not found")
}

func TestOrchestratorTerminatesAtMaxLoops(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{responses: map[string][]brave.SearchResult{"q1": {{URL: "https://example.com/a", Title: "A", Snippet: "snippet"}}}},
		plannerStub{
			initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}},
			eval:    PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}},
		},
		nil,
		OrchestratorConfig{MaxLoops: 2, MaxSearchQueries: 4, MaxSourcesRead: 4, MaxCitations: 4, SearchResultsPerQ: 3},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Loops != 2 {
		t.Fatalf("expected 2 loops, got %d", result.Loops)
	}
	if result.StopReason != StopReasonBudgetExhausted {
		t.Fatalf("expected budget_exhausted stop reason, got %s", result.StopReason)
	}
}

func TestOrchestratorEnforcesQueryAndReadCaps(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{responses: map[string][]brave.SearchResult{
			"q1": {{URL: "https://example.com/a", Title: "A", Snippet: "snippet a"}},
			"q2": {{URL: "https://example.com/b", Title: "B", Snippet: "snippet b"}},
			"q3": {{URL: "https://example.com/c", Title: "C", Snippet: "snippet c"}},
		}},
		plannerStub{
			initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1", "q2", "q3"}},
			eval:    PlannerDecision{NextAction: NextActionFinalize},
		},
		readerStub{responses: map[string]ReadResult{
			"https://example.com/a": {URL: "https://example.com/a", FinalURL: "https://example.com/a", ContentType: "text/plain", Text: "full text a", Snippet: "full text a", FetchStatus: "ok", FetchedAt: time.Now().UTC()},
			"https://example.com/b": {URL: "https://example.com/b", FinalURL: "https://example.com/b", ContentType: "text/plain", Text: "full text b", Snippet: "full text b", FetchStatus: "ok", FetchedAt: time.Now().UTC()},
		}},
		OrchestratorConfig{MaxLoops: 3, MaxSearchQueries: 2, MaxSourcesRead: 1, MaxCitations: 4, SearchResultsPerQ: 3},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.SearchQueries > 2 {
		t.Fatalf("expected query cap 2, got %d", result.SearchQueries)
	}
	if result.SourcesRead > 1 {
		t.Fatalf("expected read cap 1, got %d", result.SourcesRead)
	}
}

func TestOrchestratorTimeoutStopsRun(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{block: true},
		plannerStub{initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}}, eval: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}}},
		nil,
		OrchestratorConfig{MaxLoops: 2, MaxSearchQueries: 2, MaxSourcesRead: 2, MaxCitations: 2, SearchResultsPerQ: 1, Timeout: 20 * time.Millisecond},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if result.StopReason != StopReasonTimeout {
		t.Fatalf("expected timeout stop reason, got %s", result.StopReason)
	}
}

func TestOrchestratorConvertsRecoverableErrorsToWarnings(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{err: errors.New("brave unavailable")},
		plannerStub{initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}}, eval: PlannerDecision{NextAction: NextActionFinalize}},
		nil,
		OrchestratorConfig{MaxLoops: 1, MaxSearchQueries: 1, MaxSourcesRead: 1, MaxCitations: 1, SearchResultsPerQ: 1},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected recoverable warning, got none")
	}
}

func TestOrchestratorWarnsOnlyWhenAllReadAttemptsFail(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{responses: map[string][]brave.SearchResult{
			"q1": {
				{URL: "https://example.com/a", Title: "A", Snippet: "snippet a"},
				{URL: "https://example.com/b", Title: "B", Snippet: "snippet b"},
			},
		}},
		plannerStub{
			initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}},
			eval:    PlannerDecision{NextAction: NextActionFinalize},
		},
		readerStub{
			responses: map[string]ReadResult{
				"https://example.com/a": {URL: "https://example.com/a", FetchStatus: "unsupported_content_type"},
				"https://example.com/b": {URL: "https://example.com/b", FinalURL: "https://example.com/b", ContentType: "text/plain", Text: "full text b", Snippet: "full text b", FetchStatus: "ok", FetchedAt: time.Now().UTC()},
			},
			errs: map[string]error{
				"https://example.com/a": errUnsupportedContentType,
			},
		},
		OrchestratorConfig{MaxLoops: 1, MaxSearchQueries: 2, MaxSourcesRead: 4, MaxCitations: 4, SearchResultsPerQ: 3},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.ReadAttempts != 2 {
		t.Fatalf("expected 2 read attempts, got %d", result.ReadAttempts)
	}
	if result.ReadFailures != 1 {
		t.Fatalf("expected 1 read failure, got %d", result.ReadFailures)
	}
	if result.ReadFailureReasons["unsupported_content_type"] != 1 {
		t.Fatalf("expected unsupported_content_type classification, got %+v", result.ReadFailureReasons)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "search snippets") {
			t.Fatalf("did not expect read warning when at least one source read succeeded: %+v", result.Warnings)
		}
	}
}

func TestOrchestratorWarnsWhenAllReadAttemptsFail(t *testing.T) {
	orchestrator := NewOrchestrator(
		searcherStub{responses: map[string][]brave.SearchResult{
			"q1": {
				{URL: "https://example.com/a", Title: "A", Snippet: "snippet a"},
				{URL: "https://example.com/b", Title: "B", Snippet: "snippet b"},
			},
		}},
		plannerStub{
			initial: PlannerDecision{NextAction: NextActionSearchMore, Queries: []string{"q1"}},
			eval:    PlannerDecision{NextAction: NextActionFinalize},
		},
		readerStub{
			responses: map[string]ReadResult{
				"https://example.com/a": {URL: "https://example.com/a", FetchStatus: "blocked"},
				"https://example.com/b": {URL: "https://example.com/b", FetchStatus: "http_403"},
			},
			errs: map[string]error{
				"https://example.com/a": errBlockedURLHost,
				"https://example.com/b": errors.New("upstream returned status 403"),
			},
		},
		OrchestratorConfig{MaxLoops: 1, MaxSearchQueries: 2, MaxSourcesRead: 4, MaxCitations: 4, SearchResultsPerQ: 3},
	)

	result, err := orchestrator.Run(context.Background(), "question", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.ReadAttempts != 2 || result.ReadFailures != 2 {
		t.Fatalf("expected all reads to fail, got attempts=%d failures=%d", result.ReadAttempts, result.ReadFailures)
	}
	if result.ReadFailureReasons["blocked_url"] != 1 || result.ReadFailureReasons["http_status"] != 1 {
		t.Fatalf("expected bounded failure reasons, got %+v", result.ReadFailureReasons)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning when all reads fail")
	}
	if !strings.Contains(result.Warnings[0], "search snippets") {
		t.Fatalf("expected fallback warning message, got %+v", result.Warnings)
	}
}
