package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"chat/backend/internal/brave"
	"chat/backend/internal/research"
)

type fusionGroundingResult struct {
	Citations       []citationResponse
	SearchQueries   int
	ReadableSources int
	Warnings        []string
}

// A globally serialized searcher for the fusion run.
type fusionGroundingCoordinator struct {
	mu           sync.Mutex
	handler      *Handler
	runID        string
	runBudget    int
	defaultCount int
	searchesUsed int
	lastSearchAt time.Time
	minSpacing   time.Duration
}

func newFusionGroundingCoordinator(h *Handler, runID string, budget int, defaultCount int) *fusionGroundingCoordinator {
	if defaultCount < 1 {
		defaultCount = 15
	}
	return &fusionGroundingCoordinator{
		handler:      h,
		runID:        runID,
		runBudget:    budget,
		defaultCount: defaultCount,
		minSpacing:   1100 * time.Millisecond,
	}
}

func (c *fusionGroundingCoordinator) Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.searchesUsed >= c.runBudget {
		return nil, errors.New("fusion search budget exhausted")
	}

	// Wait to enforce global spacing for this fusion run
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

	if count < c.defaultCount {
		count = c.defaultCount
	}

	results, err := c.handler.grounding.Search(ctx, query, count)
	if err != nil {
		_ = c.handler.recordFusionRunSearches(ctx, c.runID, c.searchesUsed)
		return nil, err
	}
	_ = c.handler.recordFusionRunSearches(ctx, c.runID, c.searchesUsed)

	return results, nil
}

func (h Handler) runFusionSinglePassGrounding(
	ctx context.Context,
	prompt string,
	timeSensitive bool,
	searchCoordinator *fusionGroundingCoordinator,
) (fusionGroundingResult, error) {
	if searchCoordinator == nil {
		return fusionGroundingResult{}, errors.New("fusion grounding coordinator unavailable")
	}

	query := strings.TrimSpace(research.BuildSingleQuery(prompt, timeSensitive))
	if query == "" {
		return fusionGroundingResult{}, errors.New("fusion grounding query is empty")
	}

	results, err := searchCoordinator.Search(ctx, query, h.cfg.FusionSearchResultsPerQuery)
	if err != nil {
		return fusionGroundingResult{}, err
	}
	if len(results) == 0 {
		return fusionGroundingResult{}, errors.New("fusion grounding returned no sources")
	}

	citations := make([]citationResponse, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	readableSources := 0
	readFailures := 0

	for _, item := range results {
		citation := citationResponse{
			URL:            strings.TrimSpace(item.URL),
			Title:          trimFusionText(strings.TrimSpace(item.Title), 240),
			Snippet:        trimFusionText(strings.TrimSpace(item.Snippet), 800),
			SourceProvider: "brave",
		}
		canonical := canonicalFusionURL(citation.URL)
		if canonical == "" {
			canonical = citation.URL
		}
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}

		if h.researchReader != nil {
			readResult, readErr := h.researchReader.Read(ctx, citation.URL)
			if readErr == nil {
				readableSources++
				if title := strings.TrimSpace(readResult.Title); title != "" {
					citation.Title = trimFusionText(title, 240)
				}
				if snippet := fusionReadSnippet(readResult); snippet != "" {
					citation.Snippet = snippet
				}
			} else {
				readFailures++
			}
		}

		if citation.Title == "" {
			citation.Title = citation.URL
		}
		citations = append(citations, citation)
	}

	if len(citations) == 0 {
		return fusionGroundingResult{}, errors.New("fusion grounding produced no usable citations")
	}

	warnings := make([]string, 0, 1)
	if readableSources == 0 {
		warnings = append(warnings, "Could not read returned sources directly; using Brave snippets for this source-model pass.")
	} else if readFailures > 0 {
		warnings = append(warnings, fmt.Sprintf("Used Brave snippets for %d source(s) that could not be read directly.", readFailures))
	}

	return fusionGroundingResult{
		Citations:       citations,
		SearchQueries:   1,
		ReadableSources: readableSources,
		Warnings:        warnings,
	}, nil
}

func fusionReadSnippet(result research.ReadResult) string {
	snippet := strings.TrimSpace(result.Snippet)
	if snippet == "" {
		snippet = strings.TrimSpace(result.Text)
	}
	return trimFusionText(snippet, 800)
}

func trimFusionText(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(raw) <= limit {
		return raw
	}
	return string([]rune(raw)[:limit])
}

func canonicalFusionURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.EscapedPath(), "/")
	return parsed.String()
}
