package research

import (
	"fmt"
	"strings"
	"time"
)

func buildPlannerPrompt(input PlannerInput) string {
	var b strings.Builder
	b.WriteString("You are a web research planner. Respond with strict JSON only.\n")
	b.WriteString("Schema: {\"nextAction\":\"search_more|finalize\",\"queries\":string[],\"coverageGaps\":string[],\"targetSourceTypes\":string[],\"confidence\":number,\"reason\":string}\n")
	b.WriteString("Rules:\n")
	b.WriteString("- If evidence is weak, stale, conflicting, or missing key facts, choose search_more.\n")
	b.WriteString("- Keep queries concise and specific.\n")
	b.WriteString("- Confidence must be between 0 and 1.\n")
	if input.TimeSensitive {
		b.WriteString(fmt.Sprintf("- Current UTC date: %s. For latest/today claims, prioritize dated official sources.\n", time.Now().UTC().Format("2006-01-02")))
	}
	b.WriteString("\nQuestion:\n")
	b.WriteString(strings.TrimSpace(input.Question))
	b.WriteString("\n")
	if len(input.PreviousQueries) > 0 {
		b.WriteString("\nPrevious queries:\n")
		for _, q := range input.PreviousQueries {
			trimmed := strings.TrimSpace(q)
			if trimmed == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}
	if len(input.CoverageGaps) > 0 {
		b.WriteString("\nKnown coverage gaps:\n")
		for _, gap := range input.CoverageGaps {
			trimmed := strings.TrimSpace(gap)
			if trimmed == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nBudget: loop %d/%d, queries %d/%d, remaining reads %d\n", input.Loop, input.MaxLoops, input.UsedQueries, input.MaxQueries, input.RemainingReadBudget))
	return strings.TrimSpace(b.String())
}

func buildEvaluationPrompt(input PlannerInput) string {
	var b strings.Builder
	b.WriteString(buildPlannerPrompt(input))
	if len(input.Evidence) == 0 {
		b.WriteString("\n\nEvidence: none\n")
		return strings.TrimSpace(b.String())
	}
	b.WriteString("\n\nEvidence summary:\n")
	for i, item := range input.Evidence {
		if i >= 12 {
			break
		}
		label := strings.TrimSpace(item.Title)
		if label == "" {
			label = item.URL
		}
		b.WriteString(fmt.Sprintf("- [%d] %s | score=%.3f | full_text=%t | contradiction=%t\n", i+1, label, item.Score, item.HasFullText, item.Contradiction))
		if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
			b.WriteString("  snippet: ")
			b.WriteString(trimToRunes(snippet, 280))
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}
