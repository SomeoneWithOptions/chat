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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

type createConversationRequest struct {
	Title string `json:"title"`
}

type conversationResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type messageResponse struct {
	ID                  string  `json:"id"`
	ConversationID      string  `json:"conversationId"`
	Role                string  `json:"role"`
	Content             string  `json:"content"`
	ModelID             *string `json:"modelId,omitempty"`
	GroundingEnabled    bool    `json:"groundingEnabled"`
	DeepResearchEnabled bool    `json:"deepResearchEnabled"`
	CreatedAt           string  `json:"createdAt"`
}

func (h Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}

	var req createConversationRequest
	if err := decodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	conversation, err := h.insertConversation(r.Context(), user.ID, req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to create conversation")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"conversation": conversation})
}

func (h Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
SELECT id, title, created_at, updated_at
FROM conversations
WHERE user_id = ?
ORDER BY updated_at DESC, created_at DESC
LIMIT 200;
`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read conversations")
		return
	}
	defer rows.Close()

	conversations := make([]conversationResponse, 0, 16)
	for rows.Next() {
		var conversation conversationResponse
		if err := rows.Scan(&conversation.ID, &conversation.Title, &conversation.CreatedAt, &conversation.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "failed to parse conversations")
			return
		}
		conversations = append(conversations, conversation)
	}

	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to iterate conversations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
}

func (h Handler) ListConversationMessages(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}

	conversationID := strings.TrimSpace(chi.URLParam(r, "id"))
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation id is required")
		return
	}

	exists, err := h.conversationExists(r.Context(), user.ID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read conversation")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "conversation_not_found", "conversation not found")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
SELECT m.id, m.conversation_id, m.role, m.content, m.model_id, m.grounding_enabled, m.deep_research_enabled, m.created_at
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = ? AND c.user_id = ?
ORDER BY m.created_at ASC, m.id ASC;
`, conversationID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read messages")
		return
	}
	defer rows.Close()

	messages := make([]messageResponse, 0, 32)
	for rows.Next() {
		var message messageResponse
		var modelID sql.NullString
		var groundingEnabled int
		var deepResearchEnabled int

		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&modelID,
			&groundingEnabled,
			&deepResearchEnabled,
			&message.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "failed to parse messages")
			return
		}

		message.ModelID = nullableStringPointer(modelID)
		message.GroundingEnabled = groundingEnabled == 1
		message.DeepResearchEnabled = deepResearchEnabled == 1
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to iterate messages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
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

	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
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
	conversationID, err := h.resolveConversationID(r.Context(), user.ID, req.ConversationID, req.Message)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation_not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve conversation")
		return
	}

	if err := h.insertMessage(r.Context(), user.ID, conversationID, "user", req.Message, modelID, grounding, deepResearch); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to persist message")
		return
	}

	started := false
	var assistantContent strings.Builder

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
				"type":           "metadata",
				"grounding":      grounding,
				"deepResearch":   deepResearch,
				"modelId":        modelID,
				"conversationId": conversationID,
			}); err != nil {
				return err
			}
			flusher.Flush()
			started = true
			return nil
		},
		func(delta string) error {
			assistantContent.WriteString(delta)

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

	if assistantContent.Len() > 0 {
		if err := h.insertMessage(r.Context(), user.ID, conversationID, "assistant", assistantContent.String(), modelID, grounding, deepResearch); err != nil {
			if !started {
				writeError(w, http.StatusInternalServerError, "db_error", "failed to persist assistant response")
				return
			}
			_ = writeSSEEvent(w, map[string]any{
				"type":    "error",
				"message": "failed to persist assistant response",
			})
			flusher.Flush()
		}
	}

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

func (h Handler) insertConversation(ctx context.Context, userID, requestedTitle string) (conversationResponse, error) {
	var conversation conversationResponse
	err := h.db.QueryRowContext(ctx, `
INSERT INTO conversations (id, user_id, title)
VALUES (?, ?, ?)
RETURNING id, title, created_at, updated_at;
`, uuid.NewString(), userID, normalizeConversationTitle(requestedTitle)).Scan(
		&conversation.ID,
		&conversation.Title,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	if err != nil {
		return conversationResponse{}, err
	}

	return conversation, nil
}

func (h Handler) conversationExists(ctx context.Context, userID, conversationID string) (bool, error) {
	var id string
	err := h.db.QueryRowContext(ctx, `
SELECT id
FROM conversations
WHERE id = ? AND user_id = ?
LIMIT 1;
`, conversationID, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h Handler) resolveConversationID(ctx context.Context, userID, requestedConversationID, seedMessage string) (string, error) {
	conversationID := strings.TrimSpace(requestedConversationID)
	if conversationID == "" {
		conversation, err := h.insertConversation(ctx, userID, seedMessage)
		if err != nil {
			return "", err
		}
		return conversation.ID, nil
	}

	exists, err := h.conversationExists(ctx, userID, conversationID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", sql.ErrNoRows
	}
	return conversationID, nil
}

func (h Handler) insertMessage(ctx context.Context, userID, conversationID, role, content, modelID string, groundingEnabled, deepResearchEnabled bool) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nullableModelID, err := resolveNullableModelID(ctx, tx, modelID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (
  id,
  conversation_id,
  user_id,
  role,
  content,
  model_id,
  grounding_enabled,
  deep_research_enabled
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, uuid.NewString(), conversationID, userID, role, content, nullableModelID, boolToInt(groundingEnabled), boolToInt(deepResearchEnabled)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;
`, conversationID, userID); err != nil {
		return err
	}

	return tx.Commit()
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

func resolveNullableModelID(ctx context.Context, tx *sql.Tx, rawModelID string) (sql.NullString, error) {
	modelID := strings.TrimSpace(rawModelID)
	if modelID == "" {
		return sql.NullString{}, nil
	}

	var existingID string
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM models
WHERE id = ?
LIMIT 1;
`, modelID).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: existingID, Valid: true}, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeConversationTitle(raw string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if normalized == "" {
		return "New Chat"
	}

	const maxRunes = 120
	runes := []rune(normalized)
	if len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes]))
	}

	return normalized
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
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
