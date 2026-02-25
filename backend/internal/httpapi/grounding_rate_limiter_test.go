package httpapi

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"chat/backend/internal/brave"
)

type timedGroundingSearcher struct {
	mu        sync.Mutex
	callTimes []time.Time
}

func (s *timedGroundingSearcher) Search(_ context.Context, _ string, _ int) ([]brave.SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callTimes = append(s.callTimes, time.Now())
	return nil, nil
}

func (s *timedGroundingSearcher) times() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Time, len(s.callTimes))
	copy(out, s.callTimes)
	return out
}

func TestRateLimitedGroundingSearcherAppliesMinimumSpacing(t *testing.T) {
	searcher := &timedGroundingSearcher{}
	limited := newRateLimitedGroundingSearcher(searcher, 40*time.Millisecond)

	if _, err := limited.Search(context.Background(), "one", 5); err != nil {
		t.Fatalf("first search: %v", err)
	}
	if _, err := limited.Search(context.Background(), "two", 5); err != nil {
		t.Fatalf("second search: %v", err)
	}

	calls := searcher.times()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[1].Sub(calls[0]) < 35*time.Millisecond {
		t.Fatalf("expected calls to be spaced by at least 35ms, got %v", calls[1].Sub(calls[0]))
	}
}

func TestRateLimitedGroundingSearcherHonorsContextCancel(t *testing.T) {
	searcher := &timedGroundingSearcher{}
	limited := newRateLimitedGroundingSearcher(searcher, 120*time.Millisecond)

	if _, err := limited.Search(context.Background(), "first", 5); err != nil {
		t.Fatalf("first search: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	_, err := limited.Search(ctx, "second", 5)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	calls := searcher.times()
	if len(calls) != 1 {
		t.Fatalf("expected only 1 underlying call after canceled wait, got %d", len(calls))
	}
}
