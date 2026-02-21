package research

import (
	"math"
	"net/url"
	"sort"
	"strings"
)

type EvidencePool struct {
	items    map[string]Evidence
	readURLs map[string]struct{}
}

func NewEvidencePool() *EvidencePool {
	return &EvidencePool{
		items:    make(map[string]Evidence),
		readURLs: make(map[string]struct{}),
	}
}

func (p *EvidencePool) HasRead(rawURL string) bool {
	if p == nil {
		return false
	}
	key := canonicalOrRawURL(rawURL)
	_, ok := p.readURLs[key]
	return ok
}

func (p *EvidencePool) AddSearchCandidate(citation Citation, timeSensitive bool) {
	if p == nil {
		return
	}
	canonical := canonicalOrRawURL(citation.URL)
	existing, ok := p.items[canonical]
	if !ok {
		existing = Evidence{CanonicalURL: canonical}
	}

	existing.Citation = mergeCitation(existing.Citation, citation)
	existing.SourceQuality = domainAuthorityBoost(hostnameFromURL(citation.URL))
	existing.Freshness = freshnessSignalScore(citation.Title + " " + citation.Snippet)
	existing.Completeness = completenessScore(citation.Snippet)
	existing.Score = clampScore(citation.Score + existing.SourceQuality + existing.Freshness + existing.Completeness)
	if timeSensitive && existing.Freshness == 0 {
		existing.Score = clampScore(existing.Score - 0.08)
	}
	if existing.Excerpt == "" {
		existing.Excerpt = trimToRunes(strings.TrimSpace(citation.Snippet), 900)
	}

	p.items[canonical] = existing
}

func (p *EvidencePool) AddReadResult(base Citation, read ReadResult, timeSensitive bool) {
	if p == nil {
		return
	}
	canonical := canonicalOrRawURL(read.FinalURL)
	if canonical == "" {
		canonical = canonicalOrRawURL(base.URL)
	}
	existing, ok := p.items[canonical]
	if !ok {
		existing = Evidence{CanonicalURL: canonical}
	}

	merged := mergeCitation(existing.Citation, base)
	if strings.TrimSpace(read.FinalURL) != "" {
		merged.URL = read.FinalURL
	}
	if strings.TrimSpace(read.Title) != "" {
		merged.Title = read.Title
	}
	if strings.TrimSpace(read.Snippet) != "" {
		merged.Snippet = trimToRunes(read.Snippet, 900)
	}

	existing.Citation = merged
	existing.ContentType = strings.TrimSpace(read.ContentType)
	existing.Excerpt = trimToRunes(strings.TrimSpace(read.Text), 6000)
	existing.HasFullText = existing.Excerpt != ""
	existing.FetchedAt = read.FetchedAt
	existing.SourceQuality = domainAuthorityBoost(hostnameFromURL(merged.URL))
	existing.Freshness = freshnessSignalScore(merged.Title + " " + merged.Snippet + " " + existing.Excerpt)
	existing.Completeness = completenessScore(existing.Excerpt)
	existing.Contradiction = contradictionSignal(existing.Excerpt)
	existing.Score = clampScore(
		maxFloat(existing.Score, merged.Score) +
			existing.SourceQuality +
			existing.Freshness +
			existing.Completeness +
			0.22,
	)
	if timeSensitive && existing.Freshness == 0 {
		existing.Score = clampScore(existing.Score - 0.10)
	}
	if existing.Contradiction {
		existing.Score = clampScore(existing.Score - 0.05)
	}

	p.items[canonical] = existing
	p.readURLs[canonical] = struct{}{}
}

func (p *EvidencePool) Rank() []Evidence {
	if p == nil || len(p.items) == 0 {
		return nil
	}
	items := make([]Evidence, 0, len(p.items))
	for _, item := range p.items {
		items = append(items, item)
	}

	applyCorroboration(items)

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			if items[i].HasFullText != items[j].HasFullText {
				return items[i].HasFullText
			}
			if items[i].Pass == items[j].Pass {
				return items[i].URL < items[j].URL
			}
			return items[i].Pass < items[j].Pass
		}
		return items[i].Score > items[j].Score
	})
	return items
}

func (p *EvidencePool) TopCitations(limit int) []Citation {
	ranked := p.Rank()
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]Citation, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.Citation)
	}
	return out
}

func canonicalOrRawURL(rawURL string) string {
	canonical := canonicalURL(rawURL)
	if canonical != "" {
		return canonical
	}
	return strings.TrimSpace(rawURL)
}

func mergeCitation(base, incoming Citation) Citation {
	out := base
	if strings.TrimSpace(out.URL) == "" {
		out.URL = incoming.URL
	}
	if strings.TrimSpace(incoming.URL) != "" {
		out.URL = incoming.URL
	}
	if strings.TrimSpace(incoming.Title) != "" && len(incoming.Title) >= len(out.Title) {
		out.Title = incoming.Title
	}
	if strings.TrimSpace(incoming.Snippet) != "" && len(incoming.Snippet) >= len(out.Snippet) {
		out.Snippet = incoming.Snippet
	}
	if out.SourceProvider == "" {
		out.SourceProvider = incoming.SourceProvider
	}
	if incoming.Query != "" {
		out.Query = incoming.Query
	}
	if incoming.Pass > 0 {
		out.Pass = incoming.Pass
	}
	if incoming.Score > out.Score {
		out.Score = incoming.Score
	}
	return out
}

func applyCorroboration(items []Evidence) {
	for i := range items {
		tokensA := tokenSet(items[i].Title + " " + items[i].Snippet + " " + items[i].Excerpt)
		if len(tokensA) == 0 {
			continue
		}
		for j := range items {
			if i == j {
				continue
			}
			hostA := hostnameFromURL(items[i].URL)
			hostB := hostnameFromURL(items[j].URL)
			if hostA == hostB && hostA != "" {
				continue
			}
			overlap := tokenOverlap(tokensA, tokenSet(items[j].Title+" "+items[j].Snippet+" "+items[j].Excerpt))
			if overlap >= 3 {
				items[i].Corroboration += 0.03
			}
		}
		items[i].Score = clampScore(items[i].Score + math.Min(items[i].Corroboration, 0.15))
	}
}

func tokenOverlap(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	matches := 0
	for token := range a {
		if _, ok := b[token]; ok {
			matches++
		}
	}
	return matches
}

func contradictionSignal(text string) bool {
	lower := strings.ToLower(text)
	signals := []string{"contradict", "conflict", "disputed", "unclear", "not confirmed", "however", "on the other hand"}
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

func freshnessSignalScore(raw string) float64 {
	if hasFreshnessSignal(raw) {
		return 0.08
	}
	return 0
}

func completenessScore(raw string) float64 {
	length := len([]rune(strings.TrimSpace(raw)))
	switch {
	case length >= 1200:
		return 0.12
	case length >= 500:
		return 0.08
	case length >= 180:
		return 0.04
	default:
		return 0
	}
}

func hostnameFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return math.Round(value*1000) / 1000
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
