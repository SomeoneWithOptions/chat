package research

import (
	"sort"
	"strings"

	"chat/backend/internal/brave"
)

func rankSearchResults(query string, loop int, timeSensitive bool, results []brave.SearchResult) []Citation {
	candidates := make([]Citation, 0, len(results))
	for _, result := range results {
		rawURL := strings.TrimSpace(result.URL)
		if rawURL == "" {
			continue
		}
		candidates = append(candidates, Citation{
			URL:            rawURL,
			Title:          trimToRunes(strings.TrimSpace(result.Title), 240),
			Snippet:        trimToRunes(strings.TrimSpace(result.Snippet), 800),
			SourceProvider: "brave",
			Query:          query,
			Pass:           loop,
			Score:          scoreEvidence(query, result, timeSensitive),
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Pass == candidates[j].Pass {
				return candidates[i].URL < candidates[j].URL
			}
			return candidates[i].Pass < candidates[j].Pass
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
}
