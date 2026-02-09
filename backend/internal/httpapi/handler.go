package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/config"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/session"
)

type Handler struct {
	cfg        config.Config
	db         *sql.DB
	sessions   session.Store
	verifier   auth.Verifier
	openrouter chatStreamer
}

type chatStreamer interface {
	StreamChatCompletion(ctx context.Context, req openrouter.StreamRequest, onStart func() error, onDelta func(string) error) error
}

func NewHandler(cfg config.Config, db *sql.DB, sessions session.Store, verifier auth.Verifier, streamer chatStreamer) Handler {
	return Handler{cfg: cfg, db: db, sessions: sessions, verifier: verifier, openrouter: streamer}
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

	grounding := true
	if req.Grounding != nil {
		grounding = *req.Grounding
	}

	deepResearch := false
	if req.DeepResearch != nil {
		deepResearch = *req.DeepResearch
	}

	modelID := fallback(req.ModelID, h.cfg.OpenRouterDefaultModel)
	started := false

	streamErr := h.openrouter.StreamChatCompletion(
		r.Context(),
		openrouter.StreamRequest{
			Model: modelID,
			Messages: []openrouter.Message{
				{Role: "system", Content: buildSystemPrompt(grounding, deepResearch)},
				{Role: "user", Content: req.Message},
			},
		},
		func() error {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			if err := writeSSEEvent(w, map[string]any{
				"type":         "metadata",
				"grounding":    grounding,
				"deepResearch": deepResearch,
				"modelId":      modelID,
			}); err != nil {
				return err
			}
			flusher.Flush()
			started = true
			return nil
		},
		func(delta string) error {
			if err := writeSSEEvent(w, map[string]any{
				"type":  "token",
				"delta": delta,
			}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
	)

	if streamErr != nil {
		if !started {
			status := http.StatusBadGateway
			code := "openrouter_error"
			message := "failed to stream from OpenRouter"
			if errors.Is(streamErr, openrouter.ErrMissingAPIKey) {
				status = http.StatusInternalServerError
				code = "openrouter_unconfigured"
				message = "OPENROUTER_API_KEY is required"
			}
			writeError(w, status, code, message)
			return
		}
		_ = writeSSEEvent(w, map[string]any{
			"type":    "error",
			"message": "stream interrupted",
		})
		flusher.Flush()
	}

	_ = writeSSEEvent(w, map[string]any{"type": "done"})
	flusher.Flush()
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

func writeSSEEvent(w io.Writer, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sse payload: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", encoded); err != nil {
		return fmt.Errorf("write sse payload: %w", err)
	}
	return nil
}

func buildSystemPrompt(grounding, deepResearch bool) string {
	mode := "normal chat"
	if deepResearch {
		mode = "deep research"
	}

	if grounding {
		return fmt.Sprintf("You are a helpful assistant in %s mode. Use grounded, factual answers and call out uncertainty.", mode)
	}
	return fmt.Sprintf("You are a helpful assistant in %s mode.", mode)
}
