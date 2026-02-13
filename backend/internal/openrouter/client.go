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

type Usage struct {
	PromptTokens     int  `json:"promptTokens"`
	CompletionTokens int  `json:"completionTokens"`
	TotalTokens      int  `json:"totalTokens"`
	ReasoningTokens  *int `json:"reasoningTokens,omitempty"`
	CostMicrosUSD    *int `json:"costMicrosUsd,omitempty"`
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
	Model         string           `json:"model"`
	Messages      []Message        `json:"messages"`
	Reasoning     *ReasoningConfig `json:"reasoning,omitempty"`
	Stream        bool             `json:"stream"`
	StreamOptions *streamOptions   `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type reasoningDetail struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type streamAPIUsage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details"`
	Cost                    json.RawMessage          `json:"cost"`
}

type streamAPIResponse struct {
	Choices []struct {
		Delta struct {
			Content          string            `json:"content"`
			ReasoningDetails []reasoningDetail `json:"reasoning_details"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *streamAPIUsage `json:"usage,omitempty"`
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

type upstreamStatusError struct {
	statusCode int
	body       string
}

func (e upstreamStatusError) Error() string {
	return fmt.Sprintf("openrouter models returned %d: %s", e.statusCode, e.body)
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
	onReasoning func(string) error,
	onUsage func(Usage) error,
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
		StreamOptions: &streamOptions{
			IncludeUsage: true,
		},
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

		if parsed.Usage != nil && onUsage != nil {
			usage := Usage{
				PromptTokens:     parsed.Usage.PromptTokens,
				CompletionTokens: parsed.Usage.CompletionTokens,
				TotalTokens:      parsed.Usage.TotalTokens,
				CostMicrosUSD:    parseOptionalPriceMicros(parsed.Usage.Cost),
			}
			if parsed.Usage.CompletionTokensDetails != nil {
				reasoningTokens := parsed.Usage.CompletionTokensDetails.ReasoningTokens
				usage.ReasoningTokens = &reasoningTokens
			}
			if err := onUsage(usage); err != nil {
				return err
			}
		}

		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return errors.New(strings.TrimSpace(parsed.Error.Message))
		}

		for _, choice := range parsed.Choices {
			// Handle reasoning tokens first (they typically arrive before content)
			for _, detail := range choice.Delta.ReasoningDetails {
				if detail.Type == "reasoning.text" && detail.Text != "" {
					if onReasoning != nil {
						if err := onReasoning(detail.Text); err != nil {
							return err
						}
					}
				}
			}

			// Handle content tokens
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

func parseOptionalPriceMicros(raw json.RawMessage) *int {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil
	}
	micros := parsePriceMicros(raw)
	return &micros
}

func (c Client) ListModels(ctx context.Context) ([]Model, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	models, err := c.listModelsFromPath(ctx, "/models/user")
	if err == nil {
		return models, nil
	}

	var upstreamErr upstreamStatusError
	if errors.As(err, &upstreamErr) && (upstreamErr.statusCode == http.StatusNotFound || upstreamErr.statusCode == http.StatusMethodNotAllowed) {
		return c.listModelsFromPath(ctx, "/models")
	}

	return nil, err
}

func (c Client) listModelsFromPath(ctx context.Context, path string) ([]Model, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
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
		return nil, upstreamStatusError{
			statusCode: resp.StatusCode,
			body:       strings.TrimSpace(string(body)),
		}
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
