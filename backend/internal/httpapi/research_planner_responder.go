package httpapi

import (
	"context"
	"errors"
	"strings"

	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
)

type openRouterPlannerResponder struct {
	streamer        chatStreamer
	modelID         string
	reasoningEffort string
}

func newOpenRouterPlannerResponder(streamer chatStreamer, modelID, reasoningEffort string) research.PromptResponder {
	if streamer == nil || strings.TrimSpace(modelID) == "" {
		return nil
	}
	return openRouterPlannerResponder{
		streamer:        streamer,
		modelID:         strings.TrimSpace(modelID),
		reasoningEffort: strings.TrimSpace(reasoningEffort),
	}
}

func (r openRouterPlannerResponder) Respond(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("planner prompt is empty")
	}

	var out strings.Builder
	err := r.streamer.StreamChatCompletion(
		ctx,
		openrouter.StreamRequest{
			Model: r.modelID,
			Messages: []openrouter.Message{
				{
					Role:    "system",
					Content: "You are a research planner. Return only valid JSON that follows the provided schema.",
				},
				{Role: "user", Content: prompt},
			},
			Reasoning: openRouterReasoningConfig(r.reasoningEffort),
		},
		nil,
		func(delta string) error {
			out.WriteString(delta)
			return nil
		},
		nil,
		nil,
	)
	if err != nil {
		return "", err
	}
	response := strings.TrimSpace(out.String())
	if response == "" {
		return "", errors.New("planner response was empty")
	}
	return response, nil
}
