package httpapi

import (
	"context"
	"sync"
	"time"

	"chat/backend/internal/brave"
)

type rateLimitedGroundingSearcher struct {
	inner       groundingSearcher
	minInterval time.Duration

	mu            sync.Mutex
	nextAllowedAt time.Time
}

func newRateLimitedGroundingSearcher(inner groundingSearcher, minInterval time.Duration) groundingSearcher {
	if inner == nil || minInterval <= 0 {
		return inner
	}
	return &rateLimitedGroundingSearcher{
		inner:       inner,
		minInterval: minInterval,
	}
}

func (s *rateLimitedGroundingSearcher) Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error) {
	if err := s.waitTurn(ctx); err != nil {
		return nil, err
	}
	return s.inner.Search(ctx, query, count)
}

func (s *rateLimitedGroundingSearcher) waitTurn(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		s.mu.Lock()
		now := time.Now()
		if s.nextAllowedAt.IsZero() || !s.nextAllowedAt.After(now) {
			s.nextAllowedAt = now.Add(s.minInterval)
			s.mu.Unlock()
			return nil
		}
		wait := time.Until(s.nextAllowedAt)
		s.mu.Unlock()

		if err := waitWithContext(ctx, wait); err != nil {
			return err
		}
	}
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
