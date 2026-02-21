package research

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultReaderRedirects  = 3
	defaultReaderMaxRunes   = 16_000
	defaultReaderUserAgent  = "chat-research-bot/1.0"
	defaultReaderMaxBodyCap = int64(1_500_000)
)

type ReaderConfig struct {
	RequestTimeout time.Duration
	MaxBytes       int64
	MaxRedirects   int
	MaxTextRunes   int
}

type HTTPReader struct {
	cfg        ReaderConfig
	httpClient *http.Client
}

func NewHTTPReader(cfg ReaderConfig, httpClient *http.Client) *HTTPReader {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultSourceFetchTimeout
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultReaderMaxBodyCap
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = defaultReaderRedirects
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = defaultReaderMaxRunes
	}

	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = secureDialContext(&net.Dialer{Timeout: cfg.RequestTimeout})
		httpClient = &http.Client{Transport: transport}
	}

	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= cfg.MaxRedirects {
			return fmt.Errorf("too many redirects")
		}
		if _, err := validateResearchURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	}

	return &HTTPReader{cfg: cfg, httpClient: httpClient}
}

func (r *HTTPReader) Read(ctx context.Context, rawURL string) (ReadResult, error) {
	if r == nil {
		return ReadResult{}, fmt.Errorf("reader is nil")
	}

	parsed, err := validateResearchURL(rawURL)
	if err != nil {
		return ReadResult{URL: rawURL, FetchStatus: "blocked"}, err
	}

	requestCtx := ctx
	cancel := func() {}
	if r.cfg.RequestTimeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, r.cfg.RequestTimeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ReadResult{URL: parsed.String(), FetchStatus: "request_failed"}, err
	}
	req.Header.Set("User-Agent", defaultReaderUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,text/markdown,application/json,text/csv,application/pdf;q=0.9,*/*;q=0.2")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return ReadResult{URL: parsed.String(), FetchStatus: "fetch_failed"}, err
	}
	defer resp.Body.Close()

	result := ReadResult{
		URL:         parsed.String(),
		FinalURL:    parsed.String(),
		FetchStatus: fmt.Sprintf("http_%d", resp.StatusCode),
		FetchedAt:   time.Now().UTC(),
	}
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if parsedType, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = parsedType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result.ContentType = contentType

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return result, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	payload, truncated, err := readBoundedBody(resp.Body, r.cfg.MaxBytes)
	if err != nil {
		return result, err
	}
	result.Truncated = truncated

	title, text, err := extractContent(contentType, payload, r.cfg.MaxTextRunes)
	if err != nil {
		if err == errUnsupportedContentType {
			result.FetchStatus = "unsupported_content_type"
			return result, err
		}
		return result, err
	}
	result.Title = title
	result.Text = text
	result.Snippet = trimToRunes(text, 900)
	if strings.TrimSpace(result.Text) == "" {
		result.FetchStatus = "empty_content"
		return result, fmt.Errorf("extracted content is empty")
	}
	result.FetchStatus = "ok"
	return result, nil
}

func readBoundedBody(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultReaderMaxBodyCap
	}
	limited := io.LimitReader(r, maxBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(payload)) > maxBytes {
		return payload[:maxBytes], true, nil
	}
	return payload, false, nil
}
