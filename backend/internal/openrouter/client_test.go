package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chat/backend/internal/config"
)

func TestStreamChatCompletionStreamsDeltas(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody := string(body)
		if !strings.Contains(rawBody, `"model":"openrouter/free"`) {
			t.Fatalf("request body missing model: %s", rawBody)
		}
		if !strings.Contains(rawBody, `"stream":true`) {
			t.Fatalf("request body missing stream=true: %s", rawBody)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:       "test-key",
		OpenRouterBaseURL:      server.URL,
		OpenRouterDefaultModel: "openrouter/free",
	}, server.Client())

	started := false
	var out strings.Builder
	err := client.StreamChatCompletion(
		context.Background(),
		StreamRequest{
			Model: "openrouter/free",
			Messages: []Message{
				{Role: "user", Content: "hi"},
			},
		},
		func() error {
			started = true
			return nil
		},
		func(delta string) error {
			out.WriteString(delta)
			return nil
		},
		nil, // onReasoning
		nil, // onUsage
	)
	if err != nil {
		t.Fatalf("stream chat completion: %v", err)
	}

	if !started {
		t.Fatal("expected onStart callback to be called")
	}
	if got := out.String(); got != "Hello world" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestStreamChatCompletionIncludesReasoningEffortWhenProvided(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody := string(body)
		if !strings.Contains(rawBody, `"reasoning":{"effort":"high"}`) {
			t.Fatalf("request body missing reasoning effort: %s", rawBody)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	err := client.StreamChatCompletion(
		context.Background(),
		StreamRequest{
			Model: "openrouter/free",
			Messages: []Message{
				{Role: "user", Content: "hi"},
			},
			Reasoning: &ReasoningConfig{Effort: "high"},
		},
		nil,
		nil,
		nil, // onReasoning
		nil, // onUsage
	)
	if err != nil {
		t.Fatalf("stream chat completion: %v", err)
	}
}

func TestStreamChatCompletionEmitsUsage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody := string(body)
		if !strings.Contains(rawBody, `"stream_options":{"include_usage":true}`) {
			t.Fatalf("request body missing stream_options.include_usage: %s", rawBody)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"gen-123\",\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":45,\"total_tokens\":165,\"completion_tokens_details\":{\"reasoning_tokens\":12},\"cost\":\"0.000420\",\"cost_details\":{\"upstream_inference_cost\":\"0.000111\"}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	var usage Usage
	err := client.StreamChatCompletion(
		context.Background(),
		StreamRequest{
			Model: "openrouter/free",
			Messages: []Message{
				{Role: "user", Content: "hi"},
			},
		},
		nil,
		nil,
		nil,
		func(next Usage) error {
			usage = next
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream chat completion: %v", err)
	}

	if usage.PromptTokens != 120 {
		t.Fatalf("unexpected prompt token usage: %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 45 {
		t.Fatalf("unexpected completion token usage: %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 165 {
		t.Fatalf("unexpected total token usage: %d", usage.TotalTokens)
	}
	if usage.ReasoningTokens == nil || *usage.ReasoningTokens != 12 {
		t.Fatalf("unexpected reasoning token usage: %+v", usage.ReasoningTokens)
	}
	if usage.CostMicrosUSD == nil || *usage.CostMicrosUSD != 420 {
		t.Fatalf("unexpected cost micros usage: %+v", usage.CostMicrosUSD)
	}
	if usage.ByokInferenceCostMicros == nil || *usage.ByokInferenceCostMicros != 111 {
		t.Fatalf("unexpected BYOK inference micros usage: %+v", usage.ByokInferenceCostMicros)
	}
	if usage.GenerationID != "gen-123" {
		t.Fatalf("unexpected generation id in usage: %q", usage.GenerationID)
	}
}

func TestGetGenerationParsesTimingAndUsage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/generation" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "gen-abc" {
			t.Fatalf("unexpected generation id query: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "gen-abc",
				"model": "openai/gpt-4o-mini",
				"provider_name": "OpenAI",
				"latency": 2670,
				"generation_time": 6170,
				"tokens_completion": 480,
				"native_tokens_completion": 480,
				"upstream_inference_cost": "0.008"
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	generation, err := client.GetGeneration(context.Background(), "gen-abc")
	if err != nil {
		t.Fatalf("get generation: %v", err)
	}

	if generation.ID != "gen-abc" {
		t.Fatalf("unexpected generation id: %q", generation.ID)
	}
	if generation.ModelID != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected generation model id: %q", generation.ModelID)
	}
	if generation.ProviderName != "OpenAI" {
		t.Fatalf("unexpected generation provider name: %q", generation.ProviderName)
	}
	if generation.LatencyMs == nil || *generation.LatencyMs != 2670 {
		t.Fatalf("unexpected latency ms: %+v", generation.LatencyMs)
	}
	if generation.GenerationTimeMs == nil || *generation.GenerationTimeMs != 6170 {
		t.Fatalf("unexpected generation time ms: %+v", generation.GenerationTimeMs)
	}
	if generation.TokensCompletion == nil || *generation.TokensCompletion != 480 {
		t.Fatalf("unexpected completion tokens: %+v", generation.TokensCompletion)
	}
	if generation.NativeTokensCompletion == nil || *generation.NativeTokensCompletion != 480 {
		t.Fatalf("unexpected native completion tokens: %+v", generation.NativeTokensCompletion)
	}
	if generation.UpstreamInferenceCostMicros == nil || *generation.UpstreamInferenceCostMicros != 8000 {
		t.Fatalf("unexpected upstream inference cost micros: %+v", generation.UpstreamInferenceCostMicros)
	}
}

func TestStreamChatCompletionReturnsUpstreamError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"bad auth"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	err := client.StreamChatCompletion(
		context.Background(),
		StreamRequest{
			Model: "openrouter/free",
			Messages: []Message{
				{Role: "user", Content: "hi"},
			},
		},
		nil,
		nil,
		nil, // onReasoning
		nil, // onUsage
	)
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamChatCompletionReturnsMissingKeyError(t *testing.T) {
	t.Parallel()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "",
		OpenRouterBaseURL: "https://openrouter.ai/api/v1",
	}, http.DefaultClient)

	err := client.StreamChatCompletion(
		context.Background(),
		StreamRequest{
			Model: "openrouter/free",
			Messages: []Message{
				{Role: "user", Content: "hi"},
			},
		},
		nil,
		nil,
		nil, // onReasoning
		nil, // onUsage
	)
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if err != ErrMissingAPIKey {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListModelsParsesCatalog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/models/user" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[
				{
					"id":"openrouter/free",
					"name":"OpenRouter Free",
					"context_length":131072,
					"supported_parameters":["reasoning","temperature"],
					"pricing":{"prompt":"0","completion":"0"}
				},
				{
					"id":"provider/model-two",
					"name":"",
					"top_provider":{"context_length":32768},
					"pricing":{"prompt":0.0000009,"completion":"0.000002"}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "openrouter/free" {
		t.Fatalf("unexpected first model id: %q", models[0].ID)
	}
	if models[0].Name != "OpenRouter Free" {
		t.Fatalf("unexpected first model name: %q", models[0].Name)
	}
	if models[0].ContextWindow != 131072 {
		t.Fatalf("unexpected first context window: %d", models[0].ContextWindow)
	}
	if !models[0].SupportsReasoning {
		t.Fatalf("expected first model to support reasoning")
	}

	if models[1].Name != "provider/model-two" {
		t.Fatalf("expected fallback name to model id, got %q", models[1].Name)
	}
	if models[1].ContextWindow != 32768 {
		t.Fatalf("expected top provider context length, got %d", models[1].ContextWindow)
	}
	if models[1].PromptPriceMicrosUSD != 1 {
		t.Fatalf("expected prompt price rounded to 1 micro, got %d", models[1].PromptPriceMicrosUSD)
	}
	if models[1].CompletionPriceMicrosUSD != 2 {
		t.Fatalf("unexpected completion price micros: %d", models[1].CompletionPriceMicrosUSD)
	}
}

func TestListModelsFallsBackWhenUserEndpointIsUnavailable(t *testing.T) {
	t.Parallel()

	requestPaths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)
		switch r.URL.Path {
		case "/models/user":
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data":[
					{
						"id":"openrouter/free",
						"name":"OpenRouter Free",
						"context_length":131072,
						"pricing":{"prompt":"0","completion":"0"}
					}
				]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "openrouter/free" {
		t.Fatalf("unexpected model id: %q", models[0].ID)
	}
	if len(requestPaths) != 2 || requestPaths[0] != "/models/user" || requestPaths[1] != "/models" {
		t.Fatalf("unexpected request sequence: %+v", requestPaths)
	}
}

func TestListModelsReturnsUpstreamError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: server.URL,
	}, server.Client())

	_, err := client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListModelsReturnsMissingKeyError(t *testing.T) {
	t.Parallel()

	client := NewClient(config.Config{
		OpenRouterAPIKey:  "",
		OpenRouterBaseURL: "https://openrouter.ai/api/v1",
	}, http.DefaultClient)

	_, err := client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if err != ErrMissingAPIKey {
		t.Fatalf("unexpected error: %v", err)
	}
}
