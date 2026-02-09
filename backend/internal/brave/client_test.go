package brave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chat/backend/internal/config"
)

func TestSearchReturnsResults(t *testing.T) {
	var receivedToken string
	var receivedQuery string
	var receivedCount string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Subscription-Token")
		receivedQuery = r.URL.Query().Get("q")
		receivedCount = r.URL.Query().Get("count")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "web": {
		    "results": [
		      {"url":"https://example.com/a","title":"Example A","description":"Snippet A"},
		      {"url":"https://example.com/a","title":"Example A Dup","description":"Duplicate"},
		      {"url":"https://example.com/b","title":"","description":"Snippet B"}
		    ]
		  }
		}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		BraveAPIKey:  "brave-key",
		BraveBaseURL: server.URL,
	}, server.Client())

	results, err := client.Search(context.Background(), "latest ai news", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if receivedToken != "brave-key" {
		t.Fatalf("expected subscription token header, got %q", receivedToken)
	}
	if receivedQuery != "latest ai news" {
		t.Fatalf("unexpected query: %q", receivedQuery)
	}
	if receivedCount != "3" {
		t.Fatalf("unexpected count: %q", receivedCount)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 deduped results, got %d", len(results))
	}
	if results[0].URL != "https://example.com/a" || results[0].Title != "Example A" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].URL != "https://example.com/b" || results[1].Title != "https://example.com/b" {
		t.Fatalf("unexpected second result fallback title: %+v", results[1])
	}
}

func TestSearchReturnsErrMissingAPIKey(t *testing.T) {
	client := NewClient(config.Config{
		BraveAPIKey:  "",
		BraveBaseURL: "https://api.search.brave.com/res/v1",
	}, nil)

	_, err := client.Search(context.Background(), "test", 3)
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if err != ErrMissingAPIKey {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestSearchReturnsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		BraveAPIKey:  "bad-key",
		BraveBaseURL: server.URL,
	}, server.Client())

	_, err := client.Search(context.Background(), "test", 2)
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "brave returned 401") {
		t.Fatalf("expected status in error, got %v", err)
	}
}
