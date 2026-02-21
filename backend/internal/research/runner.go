package research

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"chat/backend/internal/brave"
)

const (
	defaultMinPasses               = 3
	defaultMaxPasses               = 6
	defaultResultsPerPass          = 6
	defaultMaxCitations            = 10
	defaultHighConfidenceThreshold = 0.58
	defaultRateLimitRetryDelay     = 1200 * time.Millisecond
)

type Phase string

const (
	PhasePlanning     Phase = "planning"
	PhaseSearching    Phase = "searching"
	PhaseReading      Phase = "reading"
	PhaseEvaluating   Phase = "evaluating"
	PhaseIterating    Phase = "iterating"
	PhaseSynthesizing Phase = "synthesizing"
	PhaseFinalizing   Phase = "finalizing"
)

type Progress struct {
	Phase             Phase            `json:"phase"`
	Message           string           `json:"message,omitempty"`
	Title             string           `json:"title,omitempty"`
	Detail            string           `json:"detail,omitempty"`
	IsQuickStep       bool             `json:"isQuickStep,omitempty"`
	Decision          ProgressDecision `json:"decision,omitempty"`
	Pass              int              `json:"pass,omitempty"`
	TotalPasses       int              `json:"totalPasses,omitempty"`
	Loop              int              `json:"loop,omitempty"`
	MaxLoops          int              `json:"maxLoops,omitempty"`
	SourcesConsidered int              `json:"sourcesConsidered,omitempty"`
	SourcesRead       int              `json:"sourcesRead,omitempty"`
}

type Citation struct {
	URL            string
	Title          string
	Snippet        string
	SourceProvider string
	Query          string
	Pass           int
	Score          float64
}

type Result struct {
	Passes    int
	Citations []Citation
	Warning   string
}

type Searcher interface {
	Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error)
}

type Config struct {
	MinPasses               int
	MaxPasses               int
	ResultsPerPass          int
	MaxCitations            int
	HighConfidenceThreshold float64
	MinSearchInterval       time.Duration
}

type Runner struct {
	searcher Searcher
	cfg      Config
}

func NewRunner(searcher Searcher, cfg Config) Runner {
	return Runner{
		searcher: searcher,
		cfg: Config{
			MinPasses:               intOrDefault(cfg.MinPasses, defaultMinPasses),
			MaxPasses:               intOrDefault(cfg.MaxPasses, defaultMaxPasses),
			ResultsPerPass:          intOrDefault(cfg.ResultsPerPass, defaultResultsPerPass),
			MaxCitations:            intOrDefault(cfg.MaxCitations, defaultMaxCitations),
			HighConfidenceThreshold: floatOrDefault(cfg.HighConfidenceThreshold, defaultHighConfidenceThreshold),
		},
	}
}

func (r Runner) Run(ctx context.Context, question string, timeSensitive bool, onProgress func(Progress)) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.searcher == nil {
		return Result{Warning: "Grounding is unavailable for this response."}, nil
	}

	cfg := r.cfg
	if cfg.MinPasses < 1 {
		cfg.MinPasses = defaultMinPasses
	}
	if cfg.MaxPasses < cfg.MinPasses {
		cfg.MaxPasses = cfg.MinPasses
	}
	if cfg.ResultsPerPass < 1 {
		cfg.ResultsPerPass = defaultResultsPerPass
	}
	if cfg.MaxCitations < 1 {
		cfg.MaxCitations = defaultMaxCitations
	}
	if cfg.HighConfidenceThreshold <= 0 || cfg.HighConfidenceThreshold > 1 {
		cfg.HighConfidenceThreshold = defaultHighConfidenceThreshold
	}
	if cfg.MinSearchInterval < 0 {
		cfg.MinSearchInterval = 0
	}

	baseQuestion := strings.TrimSpace(question)
	if baseQuestion == "" {
		return Result{}, nil
	}

	queries := buildPassQueries(baseQuestion, timeSensitive, cfg.MinPasses, cfg.MaxPasses)
	if onProgress != nil {
		onProgress(WithProgressSummary(Progress{
			Phase:       PhasePlanning,
			Message:     fmt.Sprintf("Planned %d research passes", len(queries)),
			TotalPasses: len(queries),
		}, ProgressSummaryInput{
			Phase: PhasePlanning,
		}))
	}

	candidates := make(map[string]Citation, len(queries)*cfg.ResultsPerPass)
	searchErrors := 0
	missingAPIKey := false
	lastSearchAttemptAt := time.Time{}

	for i, query := range queries {
		if onProgress != nil {
			onProgress(WithProgressSummary(Progress{
				Phase:       PhaseSearching,
				Message:     fmt.Sprintf("Searching pass %d of %d", i+1, len(queries)),
				Pass:        i + 1,
				TotalPasses: len(queries),
			}, ProgressSummaryInput{
				Phase:      PhaseSearching,
				QueryCount: 1,
				Decision:   ProgressDecisionSearchMore,
			}))
		}

		if err := waitBeforeSearchAttempt(ctx, &lastSearchAttemptAt, cfg.MinSearchInterval); err != nil {
			return Result{}, err
		}
		results, err := r.searcher.Search(ctx, query, cfg.ResultsPerPass)
		lastSearchAttemptAt = time.Now()
		if err != nil && isBraveRateLimitError(err) {
			retryDelay := cfg.MinSearchInterval
			if retryDelay <= 0 {
				retryDelay = defaultRateLimitRetryDelay
			}
			if waitErr := waitForRetry(ctx, retryDelay); waitErr != nil {
				return Result{}, waitErr
			}
			if err := waitBeforeSearchAttempt(ctx, &lastSearchAttemptAt, cfg.MinSearchInterval); err != nil {
				return Result{}, err
			}
			results, err = r.searcher.Search(ctx, query, cfg.ResultsPerPass)
			lastSearchAttemptAt = time.Now()
		}
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Result{}, ctx.Err()
			}
			if errors.Is(err, brave.ErrMissingAPIKey) {
				missingAPIKey = true
			}
			searchErrors++
			continue
		}

		for _, item := range results {
			rawURL := strings.TrimSpace(item.URL)
			if rawURL == "" {
				continue
			}

			canonical := canonicalURL(rawURL)
			if canonical == "" {
				canonical = rawURL
			}

			scored := Citation{
				URL:            rawURL,
				Title:          strings.TrimSpace(item.Title),
				Snippet:        strings.TrimSpace(item.Snippet),
				SourceProvider: "brave",
				Query:          query,
				Pass:           i + 1,
				Score:          scoreEvidence(query, item, timeSensitive),
			}

			if existing, ok := candidates[canonical]; ok {
				if scored.Score > existing.Score {
					candidates[canonical] = scored
				}
				continue
			}
			candidates[canonical] = scored
		}
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	result := Result{Passes: len(queries)}
	if len(candidates) == 0 {
		switch {
		case missingAPIKey:
			result.Warning = "Grounding is unavailable because BRAVE_API_KEY is not configured."
		case searchErrors > 0:
			result.Warning = "Deep research search failed. Continuing without web sources."
		}
		return result, nil
	}

	all := make([]Citation, 0, len(candidates))
	for _, citation := range candidates {
		all = append(all, citation)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score == all[j].Score {
			if all[i].Pass == all[j].Pass {
				return all[i].URL < all[j].URL
			}
			return all[i].Pass < all[j].Pass
		}
		return all[i].Score > all[j].Score
	})

	highConfidence := make([]Citation, 0, len(all))
	for _, citation := range all {
		if citation.Score >= cfg.HighConfidenceThreshold {
			highConfidence = append(highConfidence, citation)
		}
	}
	if len(highConfidence) == 0 {
		fallbackCount := 4
		if len(all) < fallbackCount {
			fallbackCount = len(all)
		}
		highConfidence = append(highConfidence, all[:fallbackCount]...)
	}
	if len(highConfidence) > cfg.MaxCitations {
		highConfidence = highConfidence[:cfg.MaxCitations]
	}

	if searchErrors > 0 {
		result.Warning = "Some deep research passes failed; synthesized from available evidence."
	}
	result.Citations = highConfidence
	return result, nil
}

func waitBeforeSearchAttempt(ctx context.Context, lastAttempt *time.Time, interval time.Duration) error {
	if lastAttempt == nil || interval <= 0 || lastAttempt.IsZero() {
		return nil
	}
	return waitForRetry(ctx, time.Until(lastAttempt.Add(interval)))
}

func isBraveRateLimitError(err error) bool {
	var apiErr brave.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 429
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
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

func buildPassQueries(question string, timeSensitive bool, minPasses, maxPasses int) []string {
	base := strings.Join(strings.Fields(strings.TrimSpace(question)), " ")
	if base == "" {
		return nil
	}

	seed := []string{
		base,
		base + " key facts evidence",
		base + " official sources",
	}

	if timeSensitive {
		currentYear := strconv.Itoa(time.Now().UTC().Year())
		seed = append(seed,
			base+" latest update official announcement",
			base+" changelog release notes",
			base+" "+currentYear,
		)
	} else {
		seed = append(seed,
			base+" statistics report",
			base+" analysis comparison",
			base+" practical recommendations",
		)
	}

	lowerBase := strings.ToLower(base)
	if strings.Contains(lowerBase, " vs ") || strings.Contains(lowerBase, " versus ") {
		seed = append(seed, base+" comparison pros cons")
	}
	if strings.Contains(lowerBase, "how to") {
		seed = append(seed, base+" best practices")
	}

	unique := make([]string, 0, len(seed))
	seen := make(map[string]struct{}, len(seed))
	for _, query := range seed {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, normalized)
	}

	passes := clampInt(len(unique), minPasses, maxPasses)
	for len(unique) < passes {
		unique = append(unique, fmt.Sprintf("%s detailed research pass %d", base, len(unique)+1))
	}

	return unique[:passes]
}

func canonicalURL(raw string) string {
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

func scoreEvidence(query string, result brave.SearchResult, timeSensitive bool) float64 {
	score := 0.20
	title := strings.TrimSpace(result.Title)
	snippet := strings.TrimSpace(result.Snippet)

	if title != "" && !looksLikeURL(title) {
		score += 0.16
	}

	snippetLen := len([]rune(snippet))
	switch {
	case snippetLen >= 280:
		score += 0.24
	case snippetLen >= 120:
		score += 0.17
	case snippetLen >= 50:
		score += 0.10
	}

	if parsed, err := url.Parse(strings.TrimSpace(result.URL)); err == nil {
		if strings.EqualFold(parsed.Scheme, "https") {
			score += 0.06
		}
		score += domainAuthorityBoost(parsed.Hostname())
	}

	score += tokenOverlapBoost(query, title+" "+snippet)
	if timeSensitive && hasFreshnessSignal(title+" "+snippet) {
		score += 0.10
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return math.Round(score*1000) / 1000
}

func tokenOverlapBoost(query, text string) float64 {
	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 {
		return 0
	}
	textTokens := tokenSet(text)
	if len(textTokens) == 0 {
		return 0
	}

	matches := 0
	for token := range queryTokens {
		if _, ok := textTokens[token]; ok {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}

	denominator := len(queryTokens)
	if denominator > 8 {
		denominator = 8
	}
	return math.Min(0.24, (float64(matches)/float64(denominator))*0.24)
}

func domainAuthorityBoost(host string) float64 {
	lower := strings.ToLower(strings.TrimSpace(host))
	switch {
	case strings.HasSuffix(lower, ".gov") || strings.HasSuffix(lower, ".edu"):
		return 0.18
	case strings.HasSuffix(lower, ".org"):
		return 0.10
	case strings.Contains(lower, "docs.") || strings.Contains(lower, "developer") || strings.Contains(lower, "changelog"):
		return 0.08
	default:
		return 0.04
	}
}

func hasFreshnessSignal(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "updated") || strings.Contains(lower, "release") || strings.Contains(lower, "published") || strings.Contains(lower, "announced") {
		return true
	}
	currentYear := time.Now().UTC().Year()
	for year := currentYear - 1; year <= currentYear; year++ {
		if strings.Contains(lower, strconv.Itoa(year)) {
			return true
		}
	}
	return false
}

func tokenSet(raw string) map[string]struct{} {
	stopWords := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "for": {}, "with": {}, "from": {}, "that": {},
		"this": {}, "what": {}, "when": {}, "where": {}, "which": {}, "about": {}, "into": {}, "their": {},
	}

	out := make(map[string]struct{})
	fields := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, isStopWord := stopWords[field]; isStopWord {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func looksLikeURL(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.Contains(value, ".com/")
}

func intOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func floatOrDefault(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func clampInt(value, minValue, maxValue int) int {
	if minValue > maxValue {
		minValue = maxValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
