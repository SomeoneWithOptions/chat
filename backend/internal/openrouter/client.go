package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"chat/backend/internal/config"
)

const maxErrorBodyBytes = 8 * 1024

var ErrMissingAPIKey = errors.New("openrouter api key is not configured")

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type streamAPIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
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

	payload, err := json.Marshal(streamAPIRequest{
		Model:    strings.TrimSpace(req.Model),
		Messages: req.Messages,
		Stream:   true,
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
