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
	)
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if err != ErrMissingAPIKey {
		t.Fatalf("unexpected error: %v", err)
	}
}
