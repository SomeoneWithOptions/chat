package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/config"
	"chat/backend/internal/session"
)

type Handler struct {
	cfg      config.Config
	db       *sql.DB
	sessions session.Store
	verifier auth.Verifier
}

func NewHandler(cfg config.Config, db *sql.DB, sessions session.Store, verifier auth.Verifier) Handler {
	return Handler{cfg: cfg, db: db, sessions: sessions, verifier: verifier}
}

type contextKey string

const sessionUserContextKey contextKey = "session_user"

func (h Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type authGoogleRequest struct {
	IDToken string `json:"idToken"`
}

func (h Handler) AuthGoogle(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AuthRequired {
		writeJSON(w, http.StatusOK, map[string]any{"user": anonymousUser()})
		return
	}

	var req authGoogleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	identity, err := h.identityFromRequest(r.Context(), r, req.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_google_token", err.Error())
		return
	}
	if _, ok := h.cfg.AllowedGoogleEmails[strings.ToLower(identity.Email)]; !ok {
		writeError(w, http.StatusForbidden, "email_not_allowlisted", "email is not allowed")
		return
	}

	user, err := h.sessions.UpsertUser(r.Context(), identity.GoogleSubject, identity.Email, identity.Name, identity.AvatarURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to upsert user")
		return
	}

	token, expiresAt, err := h.sessions.CreateSession(r.Context(), user.ID, h.cfg.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to create session")
		return
	}

	h.setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h Handler) AuthMe(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		if !h.cfg.AuthRequired {
			writeJSON(w, http.StatusOK, map[string]any{"user": anonymousUser()})
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	rawToken, err := readSessionCookie(r, h.cfg.SessionCookieName)
	if err == nil {
		_ = h.sessions.DeleteSession(r.Context(), rawToken)
	}
	h.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	type modelResponse struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Provider        string `json:"provider"`
		ContextWindow   int    `json:"contextWindow"`
		PromptPriceMUSD int    `json:"promptPriceMicrosUsd"`
		OutputPriceMUSD int    `json:"outputPriceMicrosUsd"`
		Curated         bool   `json:"curated"`
	}

	rows, err := h.db.QueryContext(r.Context(), `
SELECT id, display_name, provider, context_window, prompt_price_microusd, completion_price_microusd, curated
FROM models
ORDER BY curated DESC, updated_at DESC
LIMIT 200;
`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read models")
		return
	}
	defer rows.Close()

	models := make([]modelResponse, 0, 16)
	for rows.Next() {
		var m modelResponse
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.ContextWindow, &m.PromptPriceMUSD, &m.OutputPriceMUSD, &m.Curated); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "failed to parse models")
			return
		}
		models = append(models, m)
	}

	if len(models) == 0 {
		models = append(models, modelResponse{
			ID:              h.cfg.OpenRouterDefaultModel,
			Name:            "OpenRouter Free",
			Provider:        "openrouter",
			ContextWindow:   0,
			PromptPriceMUSD: 0,
			OutputPriceMUSD: 0,
			Curated:         true,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

type chatMessageRequest struct {
	ConversationID string `json:"conversationId"`
	Message        string `json:"message"`
	ModelID        string `json:"modelId"`
	Grounding      *bool  `json:"grounding"`
	DeepResearch   *bool  `json:"deepResearch"`
}

func (h Handler) ChatMessages(w http.ResponseWriter, r *http.Request) {
	var req chatMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "message is required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "server does not support streaming")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	grounding := true
	if req.Grounding != nil {
		grounding = *req.Grounding
	}

	deepResearch := false
	if req.DeepResearch != nil {
		deepResearch = *req.DeepResearch
	}

	events := []map[string]any{
		{"type": "metadata", "grounding": grounding, "deepResearch": deepResearch, "modelId": fallback(req.ModelID, h.cfg.OpenRouterDefaultModel)},
		{"type": "token", "delta": "Implementation baseline ready. "},
		{"type": "token", "delta": "OpenRouter integration comes next. "},
		{"type": "done"},
	}

	for _, event := range events {
		payload, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
	}
}

func (h Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.cfg.AuthRequired {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionUserContextKey, anonymousUser())))
			return
		}

		rawToken, err := readSessionCookie(r, h.cfg.SessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid session")
			return
		}

		user, err := h.sessions.ResolveSession(r.Context(), rawToken)
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "session expired or invalid")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve session")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionUserContextKey, user)))
	})
}

func (h Handler) identityFromRequest(ctx context.Context, r *http.Request, idToken string) (auth.GoogleIdentity, error) {
	if !h.cfg.InsecureSkipGoogleVerify {
		return h.verifier.Verify(ctx, idToken)
	}

	email := strings.TrimSpace(r.Header.Get("X-Test-Email"))
	sub := strings.TrimSpace(r.Header.Get("X-Test-Google-Sub"))
	if email == "" || sub == "" {
		return auth.GoogleIdentity{}, errors.New("insecure auth mode requires X-Test-Email and X-Test-Google-Sub headers")
	}
	return auth.GoogleIdentity{GoogleSubject: sub, Email: strings.ToLower(email), Name: strings.TrimSpace(r.Header.Get("X-Test-Name"))}, nil
}

func (h Handler) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

func (h Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func readSessionCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cookie.Value) == "" {
		return "", errors.New("empty session cookie")
	}
	return cookie.Value, nil
}

func sessionUserFromContext(ctx context.Context) (session.User, bool) {
	value := ctx.Value(sessionUserContextKey)
	if value == nil {
		return session.User{}, false
	}
	user, ok := value.(session.User)
	return user, ok
}

func fallback(value, other string) string {
	if strings.TrimSpace(value) == "" {
		return other
	}
	return strings.TrimSpace(value)
}

func anonymousUser() session.User {
	return session.User{
		ID:        "anonymous-user",
		Email:     "anonymous@chat.local",
		Name:      "Anonymous",
		GoogleSub: "anonymous",
		CreatedAt: "1970-01-01T00:00:00Z",
		UpdatedAt: "1970-01-01T00:00:00Z",
	}
}
