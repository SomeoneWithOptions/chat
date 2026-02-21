package research

import (
	"testing"
	"time"
)

func TestEvidencePoolDedupeByCanonicalURL(t *testing.T) {
	pool := NewEvidencePool()
	pool.AddSearchCandidate(Citation{URL: "https://example.com/page?ref=1", Title: "A", Snippet: "s1", Score: 0.4}, false)
	pool.AddSearchCandidate(Citation{URL: "https://example.com/page?ref=2", Title: "A2", Snippet: "s2", Score: 0.5}, false)

	ranked := pool.Rank()
	if len(ranked) != 1 {
		t.Fatalf("expected deduped evidence, got %d", len(ranked))
	}
	if ranked[0].URL != "https://example.com/page?ref=2" {
		t.Fatalf("expected strongest citation URL to win, got %s", ranked[0].URL)
	}
}

func TestEvidencePoolRankingStability(t *testing.T) {
	pool := NewEvidencePool()
	pool.AddSearchCandidate(Citation{URL: "https://gov.example.gov/a", Title: "Official", Snippet: "detailed updated 2026 source", Score: 0.7}, true)
	pool.AddSearchCandidate(Citation{URL: "https://example.com/b", Title: "Blog", Snippet: "short note", Score: 0.3}, true)

	pool.AddReadResult(Citation{URL: "https://gov.example.gov/a", Score: 0.7}, ReadResult{
		URL:         "https://gov.example.gov/a",
		FinalURL:    "https://gov.example.gov/a",
		ContentType: "text/plain",
		Text:        "comprehensive official update published in 2026 with details",
		Snippet:     "official update 2026",
		FetchStatus: "ok",
		FetchedAt:   time.Now().UTC(),
	}, true)

	ranked := pool.Rank()
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked items, got %d", len(ranked))
	}
	if ranked[0].URL != "https://gov.example.gov/a" {
		t.Fatalf("expected official source first, got %s", ranked[0].URL)
	}
}

func TestEvidencePoolContradictionDetection(t *testing.T) {
	pool := NewEvidencePool()
	pool.AddSearchCandidate(Citation{URL: "https://example.com/a", Title: "Source", Snippet: "summary", Score: 0.6}, false)
	pool.AddReadResult(Citation{URL: "https://example.com/a", Score: 0.6}, ReadResult{
		URL:         "https://example.com/a",
		FinalURL:    "https://example.com/a",
		ContentType: "text/plain",
		Text:        "The report is disputed and contains conflicting measurements.",
		Snippet:     "disputed report",
		FetchStatus: "ok",
		FetchedAt:   time.Now().UTC(),
	}, false)

	ranked := pool.Rank()
	if len(ranked) != 1 {
		t.Fatalf("expected single evidence item")
	}
	if !ranked[0].Contradiction {
		t.Fatalf("expected contradiction flag to be set")
	}
}
