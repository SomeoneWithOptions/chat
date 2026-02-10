package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
)

const (
	maxDeepResearchCitations = 10
	maxNormalCitations       = 8
)

var citationIndexPattern = regexp.MustCompile(`\[(\d{1,2})\]`)

type deepResearchStreamInput struct {
	UserID         string
	ConversationID string
	ModelID        string
	Message        string
	Prompt         string
	Grounding      bool
	History        []openrouter.Message
}

func (h Handler) streamDeepResearchResponse(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, input deepResearchStreamInput) {
	timeoutSeconds := h.cfg.DeepResearchTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}

	researchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_ = writeSSEEvent(w, map[string]any{
		"type":           "metadata",
		"grounding":      input.Grounding,
		"deepResearch":   true,
		"modelId":        input.ModelID,
		"conversationId": input.ConversationID,
	})
	flusher.Flush()

	timeSensitive := isTimeSensitivePrompt(input.Message)
	citations := make([]citationResponse, 0, maxDeepResearchCitations)
	searchWarning := ""

	runner := research.NewRunner(h.grounding, research.Config{
		MinPasses:      3,
		MaxPasses:      6,
		ResultsPerPass: maxGroundingResults,
		MaxCitations:   maxDeepResearchCitations,
	})

	if !input.Grounding {
		_ = writeSSEEvent(w, map[string]any{
			"type":        "progress",
			"phase":       research.PhasePlanning,
			"message":     "Planning deep research response",
			"totalPasses": 0,
		})
		_ = writeSSEEvent(w, map[string]any{
			"type":        "progress",
			"phase":       research.PhaseSearching,
			"message":     "Grounding disabled; skipping web search",
			"totalPasses": 0,
		})
		flusher.Flush()
	} else {
		researchResult, err := runner.Run(researchCtx, input.Message, timeSensitive, func(progress research.Progress) {
			_ = writeSSEEvent(w, map[string]any{
				"type":        "progress",
				"phase":       progress.Phase,
				"message":     progress.Message,
				"pass":        progress.Pass,
				"totalPasses": progress.TotalPasses,
			})
			flusher.Flush()
		})
		if err != nil {
			message := "deep research interrupted"
			if errors.Is(err, context.DeadlineExceeded) {
				message = fmt.Sprintf("deep research timed out after %d seconds", timeoutSeconds)
			} else if errors.Is(err, context.Canceled) {
				message = "deep research request canceled"
			}
			_ = writeSSEEvent(w, map[string]any{"type": "error", "message": message})
			_ = writeSSEEvent(w, map[string]any{"type": "done"})
			flusher.Flush()
			return
		}
		searchWarning = strings.TrimSpace(researchResult.Warning)
		for _, item := range researchResult.Citations {
			citations = append(citations, citationResponse{
				URL:            item.URL,
				Title:          trimToRunes(item.Title, 240),
				Snippet:        trimToRunes(item.Snippet, 800),
				SourceProvider: "brave",
			})
		}
	}

	if searchWarning != "" {
		_ = writeSSEEvent(w, map[string]any{
			"type":    "warning",
			"scope":   "research",
			"message": searchWarning,
		})
		flusher.Flush()
	}

	_ = writeSSEEvent(w, map[string]any{
		"type":    "progress",
		"phase":   research.PhaseSynthesizing,
		"message": "Synthesizing evidence into final answer",
	})
	flusher.Flush()

	promptMessages := []openrouter.Message{
		{Role: "system", Content: buildDeepResearchSystemPrompt(timeSensitive)},
	}
	if len(citations) > 0 {
		promptMessages = append(promptMessages, openrouter.Message{
			Role:    "system",
			Content: buildDeepResearchEvidencePrompt(citations, timeSensitive),
		})
	}
	promptMessages = append(promptMessages, input.History...)
	promptMessages = append(promptMessages, openrouter.Message{Role: "user", Content: input.Prompt})

	var assistantContent strings.Builder
	streamErr := h.openrouter.StreamChatCompletion(
		researchCtx,
		openrouter.StreamRequest{Model: input.ModelID, Messages: promptMessages},
		nil,
		func(delta string) error {
			assistantContent.WriteString(delta)
			if err := writeSSEEvent(w, map[string]any{"type": "token", "delta": delta}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
	)

	_ = writeSSEEvent(w, map[string]any{
		"type":    "progress",
		"phase":   research.PhaseFinalizing,
		"message": "Finalizing citations and response",
	})
	flusher.Flush()

	orderedCitations := orderCitationsByClaims(citations, assistantContent.String())
	if len(orderedCitations) > maxDeepResearchCitations {
		orderedCitations = orderedCitations[:maxDeepResearchCitations]
	}

	if assistantContent.Len() > 0 {
		_, err := h.insertMessageWithCitations(
			researchCtx,
			input.UserID,
			input.ConversationID,
			"assistant",
			assistantContent.String(),
			input.ModelID,
			input.Grounding,
			true,
			orderedCitations,
		)
		if err != nil {
			_ = writeSSEEvent(w, map[string]any{
				"type":    "error",
				"message": "failed to persist assistant response",
			})
			flusher.Flush()
		} else if len(orderedCitations) > 0 {
			_ = writeSSEEvent(w, map[string]any{
				"type":      "citations",
				"citations": orderedCitations,
			})
			flusher.Flush()
		}
	}

	if streamErr != nil {
		message := "stream interrupted"
		switch {
		case errors.Is(streamErr, context.DeadlineExceeded) || errors.Is(researchCtx.Err(), context.DeadlineExceeded):
			message = fmt.Sprintf("deep research timed out after %d seconds", timeoutSeconds)
		case errors.Is(streamErr, context.Canceled), errors.Is(researchCtx.Err(), context.Canceled):
			message = "deep research request canceled"
		}
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": message})
		flusher.Flush()
	}

	_ = writeSSEEvent(w, map[string]any{"type": "done"})
	flusher.Flush()
}

func orderCitationsByClaims(citations []citationResponse, answer string) []citationResponse {
	if len(citations) <= 1 {
		return citations
	}

	seen := make(map[int]struct{}, len(citations))
	ordered := make([]citationResponse, 0, len(citations))

	matches := citationIndexPattern.FindAllStringSubmatch(answer, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		index--
		if index < 0 || index >= len(citations) {
			continue
		}
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		ordered = append(ordered, citations[index])
	}

	remaining := make([]int, 0, len(citations)-len(ordered))
	for i := range citations {
		if _, ok := seen[i]; !ok {
			remaining = append(remaining, i)
		}
	}
	sort.Ints(remaining)
	for _, idx := range remaining {
		ordered = append(ordered, citations[idx])
	}

	return dedupeCitations(ordered)
}

func dedupeCitations(citations []citationResponse) []citationResponse {
	if len(citations) == 0 {
		return citations
	}

	seen := make(map[string]struct{}, len(citations))
	out := make([]citationResponse, 0, len(citations))
	for _, citation := range citations {
		rawURL := strings.TrimSpace(citation.URL)
		if rawURL == "" {
			continue
		}
		if _, ok := seen[rawURL]; ok {
			continue
		}
		seen[rawURL] = struct{}{}
		out = append(out, citation)
	}
	return out
}

func buildDeepResearchSystemPrompt(timeSensitive bool) string {
	instruction := "You are a deep research assistant. Use only the provided evidence when making factual claims, and cite supporting evidence inline with [n] markers that map to the evidence list. Never invent citations."
	if timeSensitive {
		instruction += " For latest/current/today questions, only claim recency that is directly supported by source dates. If dates are unclear, explicitly say so."
	}

	return strings.TrimSpace(instruction + `
Output with these exact section headers:
1. Direct Answer
2. Key Evidence
3. Conflicting Signals
4. Recommendations
5. Source List
Keep claims concise, explain uncertainty, and avoid unsupported speculation.`)
}

func buildDeepResearchEvidencePrompt(citations []citationResponse, timeSensitive bool) string {
	if len(citations) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Deep research evidence set:\n")
	if timeSensitive {
		builder.WriteString(fmt.Sprintf("Current date (UTC): %s\n", time.Now().UTC().Format("2006-01-02")))
		builder.WriteString("Prefer sources with explicit publication/update dates.\n")
	}
	for i, citation := range citations {
		title := strings.TrimSpace(citation.Title)
		if title == "" {
			title = citation.URL
		}
		builder.WriteString(fmt.Sprintf("\n[%d] %s\nURL: %s\n", i+1, title, citation.URL))
		if snippet := strings.TrimSpace(citation.Snippet); snippet != "" {
			builder.WriteString("Snippet: ")
			builder.WriteString(snippet)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nCite evidence IDs in brackets (for example, [1], [2]) for factual statements.")
	return strings.TrimSpace(builder.String())
}
