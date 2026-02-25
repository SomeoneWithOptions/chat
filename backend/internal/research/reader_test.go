package research

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestValidateResearchURLSchemeAllowDeny(t *testing.T) {
	if _, err := validateResearchURL("https://example.com/page"); err != nil {
		t.Fatalf("expected https to be allowed: %v", err)
	}
	if _, err := validateResearchURL("http://example.com/page"); err != nil {
		t.Fatalf("expected http to be allowed: %v", err)
	}
	if _, err := validateResearchURL("file:///etc/passwd"); err == nil {
		t.Fatal("expected file scheme to be denied")
	}
}

func TestValidateResearchURLBlocksPrivateIP(t *testing.T) {
	if _, err := validateResearchURL("http://127.0.0.1:8080/admin"); err == nil {
		t.Fatal("expected private loopback ip to be blocked")
	}
	if _, err := validateResearchURL("http://[::1]/"); err == nil {
		t.Fatal("expected ipv6 loopback to be blocked")
	}
}

func TestReaderBodySizeCap(t *testing.T) {
	payload := strings.Repeat("a", 2048)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader(payload)),
				Request:    req,
			}, nil
		}),
	}
	reader := NewHTTPReader(ReaderConfig{MaxBytes: 256, MaxTextRunes: 512, RequestTimeout: 2 * time.Second}, client)

	result, err := reader.Read(context.Background(), "https://example.com/large")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result")
	}
	if len(result.Text) == 0 || len(result.Text) > 256 {
		t.Fatalf("expected bounded extracted text, got length=%d", len(result.Text))
	}
}

func TestReaderTimeoutBehavior(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}
	reader := NewHTTPReader(ReaderConfig{RequestTimeout: 20 * time.Millisecond}, client)

	_, err := reader.Read(context.Background(), "https://example.com/slow")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestReaderExtractionSmokeByContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "html", contentType: "text/html", body: "<html><head><title>T</title></head><body><h1>Hello</h1><p>World</p></body></html>"},
		{name: "text", contentType: "text/plain", body: "plain text"},
		{name: "markdown", contentType: "text/markdown", body: "# Header\nBody"},
		{name: "json", contentType: "application/json", body: "{\"a\":1,\"b\":2}"},
		{name: "csv", contentType: "text/csv", body: "a,b\n1,2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{tc.contentType}},
						Body:       io.NopCloser(strings.NewReader(tc.body)),
						Request:    req,
					}, nil
				}),
			}
			reader := NewHTTPReader(ReaderConfig{RequestTimeout: time.Second}, client)
			result, err := reader.Read(context.Background(), "https://example.com/content")
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if strings.TrimSpace(result.Text) == "" {
				t.Fatalf("expected non-empty extracted text")
			}
		})
	}
}

func TestClassifyReadFailureTimeout(t *testing.T) {
	reason := classifyReadFailure(context.DeadlineExceeded, ReadResult{FetchStatus: "fetch_failed"})
	if reason != "timeout" {
		t.Fatalf("expected timeout reason, got %q", reason)
	}
}

func TestClassifyReadFailureUnsupportedContentType(t *testing.T) {
	reason := classifyReadFailure(errUnsupportedContentType, ReadResult{FetchStatus: "unsupported_content_type"})
	if reason != "unsupported_content_type" {
		t.Fatalf("expected unsupported_content_type reason, got %q", reason)
	}
}

func TestClassifyReadFailureBlockedURL(t *testing.T) {
	reason := classifyReadFailure(fmt.Errorf("%w", errBlockedURLHost), ReadResult{FetchStatus: "blocked"})
	if reason != "blocked_url" {
		t.Fatalf("expected blocked_url reason, got %q", reason)
	}
}
