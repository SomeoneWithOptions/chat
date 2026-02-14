package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	maxNormalCitations       = 10
	braveFreeTierSpacing     = 1100 * time.Millisecond
)

var citationIndexPattern = regexp.MustCompile(`\[(\d{1,2})\]`)

type deepResearchStreamInput struct {
	UserID          string
	UserMessageID   string
	ConversationID  string
	ModelID         string
	ReasoningEffort string
	Message         string
	Prompt          string
	Grounding       bool
	IsAnonymous     bool
	History         []openrouter.Message
}

func (h Handler) streamDeepResearchResponse(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, input deepResearchStreamInput) {
	timeoutSeconds := h.cfg.DeepResearchTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 150
	}

	researchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	startedAt := time.Now()

	log.Printf(
		"deep research start: user_id=%s anonymous=%t conversation_id=%s user_message_id=%s model_id=%s grounding=%t timeout_seconds=%d message_chars=%d history_messages=%d",
		input.UserID,
		input.IsAnonymous,
		input.ConversationID,
		input.UserMessageID,
		input.ModelID,
		input.Grounding,
		timeoutSeconds,
		len([]rune(input.Message)),
		len(input.History),
	)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	metadataEvent := map[string]any{
		"type":           "metadata",
		"grounding":      input.Grounding,
		"deepResearch":   true,
		"modelId":        input.ModelID,
		"conversationId": input.ConversationID,
	}
	if input.ReasoningEffort != "" {
		metadataEvent["reasoningEffort"] = input.ReasoningEffort
	}
	_ = writeSSEEvent(w, metadataEvent)
	flusher.Flush()

	timeSensitive := isTimeSensitivePrompt(input.Message)
	citations := make([]citationResponse, 0, maxDeepResearchCitations)
	searchWarning := ""

	runner := research.NewRunner(h.grounding, research.Config{
		MinPasses:         3,
		MaxPasses:         6,
		ResultsPerPass:    maxGroundingResults,
		MaxCitations:      maxDeepResearchCitations,
		MinSearchInterval: braveFreeTierSpacing,
	})

	if !input.Grounding {
		log.Printf(
			"deep research search skipped: user_id=%s conversation_id=%s user_message_id=%s reason=grounding_disabled",
			input.UserID,
			input.ConversationID,
			input.UserMessageID,
		)
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
		searchStartedAt := time.Now()
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
			log.Printf(
				"deep research search failed: user_id=%s anonymous=%t conversation_id=%s user_message_id=%s err=%v elapsed_ms=%d",
				input.UserID,
				input.IsAnonymous,
				input.ConversationID,
				input.UserMessageID,
				err,
				time.Since(searchStartedAt).Milliseconds(),
			)
			_ = writeSSEEvent(w, map[string]any{"type": "error", "message": message})
			_ = writeSSEEvent(w, map[string]any{"type": "done"})
			flusher.Flush()
			return
		}
		searchWarning = strings.TrimSpace(researchResult.Warning)
		log.Printf(
			"deep research search completed: user_id=%s conversation_id=%s user_message_id=%s passes=%d citations=%d warning_present=%t elapsed_ms=%d",
			input.UserID,
			input.ConversationID,
			input.UserMessageID,
			researchResult.Passes,
			len(researchResult.Citations),
			searchWarning != "",
			time.Since(searchStartedAt).Milliseconds(),
		)
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
		log.Printf(
			"deep research warning: user_id=%s conversation_id=%s user_message_id=%s warning=%q",
			input.UserID,
			input.ConversationID,
			input.UserMessageID,
			searchWarning,
		)
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
	var reasoningContent strings.Builder
	var assistantUsage *openrouter.Usage
	var streamStartedAt time.Time
	var firstTokenAt time.Time

	markFirstTokenAt := func() {
		if firstTokenAt.IsZero() {
			firstTokenAt = time.Now()
		}
	}
	streamErr := h.openrouter.StreamChatCompletion(
		researchCtx,
		openrouter.StreamRequest{
			Model:     input.ModelID,
			Messages:  promptMessages,
			Reasoning: openRouterReasoningConfig(input.ReasoningEffort),
		},
		func() error {
			streamStartedAt = time.Now()
			return nil
		},
		func(delta string) error {
			assistantContent.WriteString(delta)
			markFirstTokenAt()
			if err := writeSSEEvent(w, map[string]any{"type": "token", "delta": delta}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
		func(reasoning string) error {
			reasoningContent.WriteString(reasoning)
			markFirstTokenAt()
			if err := writeSSEEvent(w, map[string]any{"type": "reasoning", "delta": reasoning}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
		func(usage openrouter.Usage) error {
			copied := usage
			assistantUsage = &copied
			if err := writeSSEEvent(w, map[string]any{
				"type":  "usage",
				"usage": usageResponseFromOpenRouter(copied),
			}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
	)

	if assistantUsage != nil {
		enriched := h.usageWithOpenRouterMetrics(researchCtx, *assistantUsage, streamStartedAt, firstTokenAt)
		assistantUsage = &enriched
		_ = writeSSEEvent(w, map[string]any{
			"type":  "usage",
			"usage": usageResponseFromOpenRouter(enriched),
		})
		flusher.Flush()
	}

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
			reasoningContent.String(),
			input.ModelID,
			input.Grounding,
			true,
			orderedCitations,
			messageUsageFromOpenRouter(assistantUsage),
		)
		if err != nil {
			log.Printf(
				"deep research persist failed: user_id=%s anonymous=%t conversation_id=%s user_message_id=%s err=%v content_chars=%d citations=%d",
				input.UserID,
				input.IsAnonymous,
				input.ConversationID,
				input.UserMessageID,
				err,
				assistantContent.Len(),
				len(orderedCitations),
			)
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
		log.Printf(
			"deep research stream error: user_id=%s anonymous=%t conversation_id=%s user_message_id=%s err=%v response_chars=%d citations=%d total_elapsed_ms=%d",
			input.UserID,
			input.IsAnonymous,
			input.ConversationID,
			input.UserMessageID,
			streamErr,
			assistantContent.Len(),
			len(orderedCitations),
			time.Since(startedAt).Milliseconds(),
		)
		_ = writeSSEEvent(w, map[string]any{"type": "error", "message": message})
		flusher.Flush()
	}

	log.Printf(
		"deep research completed: user_id=%s anonymous=%t conversation_id=%s user_message_id=%s response_chars=%d citations=%d total_elapsed_ms=%d",
		input.UserID,
		input.IsAnonymous,
		input.ConversationID,
		input.UserMessageID,
		assistantContent.Len(),
		len(orderedCitations),
		time.Since(startedAt).Milliseconds(),
	)

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
