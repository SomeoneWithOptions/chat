package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"chat/backend/internal/config"
)

const maxErrorBodyBytes = 8 * 1024

var ErrMissingAPIKey = errors.New("openrouter api key is not configured")

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Model struct {
	ID                       string
	Name                     string
	ContextWindow            int
	PromptPriceMicrosUSD     int
	CompletionPriceMicrosUSD int
	SupportedParameters      []string
	SupportsReasoning        bool
}

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type StreamRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Reasoning *ReasoningConfig `json:"reasoning,omitempty"`
}

type streamAPIRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Reasoning *ReasoningConfig `json:"reasoning,omitempty"`
	Stream    bool             `json:"stream"`
}

type streamAPIResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type listModelsAPIResponse struct {
	Data []listModelsAPIModel `json:"data"`
}

type listModelsAPIModel struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	ContextLength       int      `json:"context_length"`
	SupportedParameters []string `json:"supported_parameters"`
	Pricing             struct {
		Prompt     json.RawMessage `json:"prompt"`
		Completion json.RawMessage `json:"completion"`
	} `json:"pricing"`
	TopProvider struct {
		ContextLength int `json:"context_length"`
	} `json:"top_provider"`
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg config.Config, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{
		apiKey:     strings.TrimSpace(cfg.OpenRouterAPIKey),
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.OpenRouterBaseURL), "/"),
		httpClient: httpClient,
	}
}

func (c Client) StreamChatCompletion(
	ctx context.Context,
	req StreamRequest,
	onStart func() error,
	onDelta func(string) error,
) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return ErrMissingAPIKey
	}
	if strings.TrimSpace(req.Model) == "" {
		return errors.New("model is required")
	}
	if len(req.Messages) == 0 {
		return errors.New("messages are required")
	}

	var reasoning *ReasoningConfig
	if req.Reasoning != nil {
		effort := strings.TrimSpace(req.Reasoning.Effort)
		if effort != "" {
			reasoning = &ReasoningConfig{Effort: effort}
		}
	}

	payload, err := json.Marshal(streamAPIRequest{
		Model:     strings.TrimSpace(req.Model),
		Messages:  req.Messages,
		Reasoning: reasoning,
		Stream:    true,
	})
	if err != nil {
		return fmt.Errorf("marshal openrouter request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build openrouter request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request openrouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return fmt.Errorf("openrouter returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if onStart != nil {
		if err := onStart(); err != nil {
			return err
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			return nil
		}

		var parsed streamAPIResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			continue
		}

		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return errors.New(strings.TrimSpace(parsed.Error.Message))
		}

		for _, choice := range parsed.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			if onDelta != nil {
				if err := onDelta(delta); err != nil {
					return err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read openrouter stream: %w", err)
	}
	return nil
}

func (c Client) ListModels(ctx context.Context) ([]Model, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build openrouter models request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request openrouter models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("openrouter models returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed listModelsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode openrouter models response: %w", err)
	}

	models := make([]Model, 0, len(parsed.Data))
	for _, model := range parsed.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}

		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = id
		}

		contextWindow := model.ContextLength
		if contextWindow <= 0 {
			contextWindow = model.TopProvider.ContextLength
		}

		supportedParameters := normalizeSupportedParameters(model.SupportedParameters)

		models = append(models, Model{
			ID:                       id,
			Name:                     name,
			ContextWindow:            contextWindow,
			PromptPriceMicrosUSD:     parsePriceMicros(model.Pricing.Prompt),
			CompletionPriceMicrosUSD: parsePriceMicros(model.Pricing.Completion),
			SupportedParameters:      supportedParameters,
			SupportsReasoning:        supportsReasoningParameter(supportedParameters),
		})
	}

	return models, nil
}

func parsePriceMicros(raw json.RawMessage) int {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return priceStringToMicros(asString)
	}

	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		if asNumber < 0 {
			return 0
		}
		return int(math.Round(asNumber * 1_000_000))
	}

	return 0
}

func normalizeSupportedParameters(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}

	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, parameter := range raw {
		normalized := strings.ToLower(strings.TrimSpace(parameter))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func supportsReasoningParameter(supported []string) bool {
	for _, parameter := range supported {
		switch parameter {
		case "reasoning", "reasoning_effort":
			return true
		}
	}
	return false
}

func priceStringToMicros(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}

	if floatValue, err := strconv.ParseFloat(trimmed, 64); err == nil {
		if floatValue < 0 {
			return 0
		}
		return int(math.Round(floatValue * 1_000_000))
	}

	rat := new(big.Rat)
	if _, ok := rat.SetString(trimmed); !ok {
		return 0
	}
	if rat.Sign() < 0 {
		return 0
	}

	rat.Mul(rat, big.NewRat(1_000_000, 1))
	value, _ := rat.Float64()
	return int(math.Round(value))
}
