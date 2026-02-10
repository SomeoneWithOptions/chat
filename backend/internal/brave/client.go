package brave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"chat/backend/internal/config"
)

const maxErrorBodyBytes = 8 * 1024
const maxQueryWords = 50

var ErrMissingAPIKey = errors.New("brave api key is not configured")

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("brave returned %d: %s", e.StatusCode, e.Body)
}

type SearchResult struct {
	URL     string
	Title   string
	Snippet string
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type searchAPIResponse struct {
	Web struct {
		Results []searchAPIResult `json:"results"`
	} `json:"web"`
	Results []searchAPIResult `json:"results"`
}

type searchAPIResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Snippet       string   `json:"snippet"`
	ExtraSnippets []string `json:"extra_snippets"`
}

func NewClient(cfg config.Config, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{
		apiKey:     strings.TrimSpace(cfg.BraveAPIKey),
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BraveBaseURL), "/"),
		httpClient: httpClient,
	}
}

func (c Client) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, nil
	}
	trimmedQuery = trimToWordLimit(trimmedQuery, maxQueryWords)

	if count <= 0 {
		count = 5
	}

	endpoint, err := url.Parse(c.baseURL + "/web/search")
	if err != nil {
		return nil, fmt.Errorf("parse brave endpoint: %w", err)
	}

	params := endpoint.Query()
	params.Set("q", trimmedQuery)
	params.Set("count", fmt.Sprintf("%d", count))
	params.Set("spellcheck", "0")
	params.Set("text_decorations", "0")
	endpoint.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build brave request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Subscription-Token", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request brave: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, APIError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	var parsed searchAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}

	rawResults := parsed.Web.Results
	if len(rawResults) == 0 {
		rawResults = parsed.Results
	}

	results := make([]SearchResult, 0, len(rawResults))
	seenURLs := make(map[string]struct{}, len(rawResults))
	for _, item := range rawResults {
		rawURL := strings.TrimSpace(item.URL)
		if rawURL == "" {
			continue
		}
		if _, exists := seenURLs[rawURL]; exists {
			continue
		}
		seenURLs[rawURL] = struct{}{}

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = rawURL
		}

		snippet := strings.TrimSpace(item.Description)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Snippet)
		}
		if snippet == "" && len(item.ExtraSnippets) > 0 {
			snippet = strings.TrimSpace(item.ExtraSnippets[0])
		}

		results = append(results, SearchResult{
			URL:     rawURL,
			Title:   title,
			Snippet: snippet,
		})

		if len(results) >= count {
			break
		}
	}

	return results, nil
}

func trimToWordLimit(input string, maxWords int) string {
	if maxWords <= 0 {
		return ""
	}
	words := strings.Fields(strings.TrimSpace(input))
	if len(words) <= maxWords {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:maxWords], " ")
}
