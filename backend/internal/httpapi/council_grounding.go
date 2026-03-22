package httpapi

import (
	"context"
	"errors"
	"sync"
	"time"

	"chat/backend/internal/brave"
)

// A globally serialized searcher for the council run.
type councilGroundingCoordinator struct {
	mu           sync.Mutex
	handler      *Handler
	runID        string
	runBudget    int
	searchesUsed int
	lastSearchAt time.Time
	minSpacing   time.Duration
}

func newCouncilGroundingCoordinator(h *Handler, runID string, budget int) *councilGroundingCoordinator {
	return &councilGroundingCoordinator{
		handler:    h,
		runID:      runID,
		runBudget:  budget,
		minSpacing: 1100 * time.Millisecond,
	}
}

func (c *councilGroundingCoordinator) Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.searchesUsed >= c.runBudget {
		return nil, errors.New("council search budget exhausted")
	}

	// Wait to enforce global spacing for this council run
	elapsed := time.Since(c.lastSearchAt)
	if elapsed < c.minSpacing {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.minSpacing - elapsed):
		}
	}

	// Distributed lease and usage limits across the system
	if err := c.handler.acquireDistributedBraveLease(ctx, braveProviderName, braveFreeTierSpacing); err != nil {
		return nil, err
	}
	if err := c.handler.consumeBraveMonthlyQuery(ctx); err != nil {
		return nil, err
	}

	c.searchesUsed++
	c.lastSearchAt = time.Now()

	// Force count=15 for council mode to get more sources per query
	if count < 15 {
		count = 15
	}

	results, err := c.handler.grounding.Search(ctx, query, count)
	if err != nil {
		_ = c.handler.recordAgentRunSearches(ctx, c.runID, c.searchesUsed)
		return nil, err
	}
	_ = c.handler.recordAgentRunSearches(ctx, c.runID, c.searchesUsed)

	return results, nil
}
