package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireModelSyncTokenAllowsMatchingBearerToken(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.ModelSyncBearerToken = "sync-token-123"

	req := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	req.Header.Set("Authorization", "Bearer sync-token-123")
	resp := httptest.NewRecorder()

	handler.RequireModelSyncToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusNoContent, resp.Code, resp.Body.String())
	}
}

func TestRequireModelSyncTokenRejectsMissingBearerToken(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.ModelSyncBearerToken = "sync-token-123"

	req := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	resp := httptest.NewRecorder()

	handler.RequireModelSyncToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusUnauthorized, resp.Code, resp.Body.String())
	}
}

func TestRequireModelSyncTokenRejectsWrongBearerToken(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.ModelSyncBearerToken = "sync-token-123"

	req := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp := httptest.NewRecorder()

	handler.RequireModelSyncToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusUnauthorized, resp.Code, resp.Body.String())
	}
}

func TestRequireModelSyncTokenRejectsWhenTokenNotConfigured(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	req := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	req.Header.Set("Authorization", "Bearer sync-token-123")
	resp := httptest.NewRecorder()

	handler.RequireModelSyncToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusServiceUnavailable, resp.Code, resp.Body.String())
	}
}
