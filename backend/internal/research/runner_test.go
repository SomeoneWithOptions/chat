package research

import (
	"context"
	"errors"
	"testing"
	"time"

	"chat/backend/internal/brave"
)

func TestBuildPassQueriesRespectsBounds(t *testing.T) {
	queries := buildPassQueries("What is changing in Go release notes?", true, 3, 6)
	if len(queries) < 3 || len(queries) > 6 {
		t.Fatalf("expected pass count between 3 and 6, got %d", len(queries))
	}

	queries = buildPassQueries("test", false, 5, 5)
	if len(queries) != 5 {
		t.Fatalf("expected exact pass count of 5, got %d", len(queries))
	}
}

func TestRunnerDedupesAndRanksCitations(t *testing.T) {
	runner := NewRunner(stubSearcher{
		responses: map[string][]brave.SearchResult{
			"question": {
				{URL: "https://example.com/a?ref=1", Title: "A title", Snippet: "Longer snippet with relevant facts and publication date 2026."},
				{URL: "https://example.com/a?ref=2", Title: "Duplicate URL", Snippet: "Should be deduped by canonical url."},
				{URL: "http://lowquality.net/post", Title: "Low", Snippet: "tiny"},
			},
			"question key facts evidence": {
				{URL: "https://gov.example.gov/report", Title: "Official Report", Snippet: "Comprehensive official update with 2026 coverage and release details."},
			},
			"question official sources": {
				{URL: "https://docs.vendor.com/changelog", Title: "Changelog", Snippet: "Updated release notes and timeline."},
			},
		},
	}, Config{MinPasses: 3, MaxPasses: 3, MaxCitations: 10})

	result, err := runner.Run(context.Background(), "question", true, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Passes != 3 {
		t.Fatalf("expected 3 passes, got %d", result.Passes)
	}
	if len(result.Citations) == 0 {
		t.Fatal("expected citations")
	}

	seen := map[string]struct{}{}
	for _, citation := range result.Citations {
		canonical := canonicalURL(citation.URL)
		if canonical == "" {
			canonical = citation.URL
		}
		if _, ok := seen[canonical]; ok {
			t.Fatalf("expected deduped citations, got duplicate %q", canonical)
		}
		seen[canonical] = struct{}{}
	}

	if result.Citations[0].URL != "https://gov.example.gov/report" {
		t.Fatalf("expected highest ranked official source first, got %s", result.Citations[0].URL)
	}
}

func TestRunnerHonorsContextTimeout(t *testing.T) {
	runner := NewRunner(blockingSearcher{}, Config{MinPasses: 3, MaxPasses: 3})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, "timeout test", false, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestRunnerRetriesOnRateLimit(t *testing.T) {
	searcher := &rateLimitThenSuccessSearcher{}
	runner := NewRunner(searcher, Config{MinPasses: 1, MaxPasses: 1})

	result, err := runner.Run(context.Background(), "rate limit test", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Citations) == 0 {
		t.Fatal("expected citations after retry")
	}
	if searcher.calls != 2 {
		t.Fatalf("expected 2 search attempts, got %d", searcher.calls)
	}
}

type stubSearcher struct {
	responses map[string][]brave.SearchResult
}

func (s stubSearcher) Search(_ context.Context, query string, _ int) ([]brave.SearchResult, error) {
	if values, ok := s.responses[query]; ok {
		return values, nil
	}
	return nil, nil
}

type blockingSearcher struct{}

func (blockingSearcher) Search(ctx context.Context, _ string, _ int) ([]brave.SearchResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type rateLimitThenSuccessSearcher struct {
	calls int
}

func (s *rateLimitThenSuccessSearcher) Search(_ context.Context, _ string, _ int) ([]brave.SearchResult, error) {
	s.calls++
	if s.calls == 1 {
		return nil, brave.APIError{StatusCode: 429, Body: `{"error":"rate limit"}`}
	}
	return []brave.SearchResult{
		{URL: "https://example.com/recovered", Title: "Recovered source", Snippet: "Detailed evidence after retry."},
	}, nil
}
