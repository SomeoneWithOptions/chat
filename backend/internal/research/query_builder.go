package research

import (
	"strconv"
	"strings"
	"time"
)

const singleQueryMaxWords = 48

// BuildSingleQuery returns a deterministic best-effort Brave query for a
// single-pass grounded lookup.
func BuildSingleQuery(question string, timeSensitive bool) string {
	base := strings.Join(strings.Fields(strings.TrimSpace(question)), " ")
	if base == "" {
		return ""
	}

	lowerBase := strings.ToLower(base)
	suffix := make([]string, 0, 8)

	if strings.Contains(lowerBase, " vs ") || strings.Contains(lowerBase, " versus ") {
		suffix = append(suffix, "comparison", "pros", "cons")
	}
	if strings.Contains(lowerBase, "how to") {
		suffix = append(suffix, "best", "practices")
	}
	if looksDataSeeking(lowerBase) {
		suffix = append(suffix, "statistics")
	}

	if timeSensitive {
		suffix = append(suffix, "latest", "official", strconv.Itoa(time.Now().UTC().Year()))
	} else {
		suffix = append(suffix, "official")
		if len(suffix) == 1 {
			suffix = append(suffix, "overview")
		}
	}

	suffix = dedupeQueryTerms(suffix)
	baseWordBudget := singleQueryMaxWords - len(suffix)
	if baseWordBudget < 8 {
		baseWordBudget = 8
	}
	base = trimWords(base, baseWordBudget)

	terms := append(strings.Fields(base), suffix...)
	return trimWords(strings.Join(terms, " "), singleQueryMaxWords)
}

func looksDataSeeking(lowerBase string) bool {
	keywords := []string{
		"statistics",
		"stats",
		"market share",
		"how many",
		"percentage",
		"percent",
		"numbers",
		"benchmark",
		"benchmarks",
	}
	for _, keyword := range keywords {
		if strings.Contains(lowerBase, keyword) {
			return true
		}
	}
	return false
}

func dedupeQueryTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(term)), " "))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func trimWords(input string, maxWords int) string {
	if maxWords <= 0 {
		return ""
	}
	words := strings.Fields(strings.TrimSpace(input))
	if len(words) <= maxWords {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:maxWords], " ")
}
