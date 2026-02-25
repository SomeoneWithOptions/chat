package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/brave"
	"chat/backend/internal/config"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
	"chat/backend/internal/session"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var (
	errInvalidReasoningEffort    = errors.New("invalid reasoning effort")
	errReasoningUnsupportedModel = errors.New("model does not support reasoning")
)

type Handler struct {
	cfg                      config.Config
	db                       *sql.DB
	sessions                 session.Store
	verifier                 auth.Verifier
	openrouter               chatStreamer
	grounding                groundingSearcher
	researchReader           research.Reader
	researchPlannerResponder research.PromptResponder
	models                   modelCataloger
	files                    fileObjectStore
}

type chatStreamer interface {
	StreamChatCompletion(
		ctx context.Context,
		req openrouter.StreamRequest,
		onStart func() error,
		onDelta func(string) error,
		onReasoning func(string) error,
		onUsage func(openrouter.Usage) error,
	) error
	GetGeneration(ctx context.Context, generationID string) (openrouter.Generation, error)
}

type modelCataloger interface {
	ListModels(ctx context.Context) ([]openrouter.Model, error)
}

type groundingSearcher interface {
	Search(ctx context.Context, query string, count int) ([]brave.SearchResult, error)
}

func NewHandler(cfg config.Config, db *sql.DB, sessions session.Store, verifier auth.Verifier, streamer chatStreamer) Handler {
	return NewHandlerWithFileStore(cfg, db, sessions, verifier, streamer, nil)
}

func NewHandlerWithFileStore(
	cfg config.Config,
	db *sql.DB,
	sessions session.Store,
	verifier auth.Verifier,
	streamer chatStreamer,
	fileStore fileObjectStore,
) Handler {
	var catalog modelCataloger
	if source, ok := streamer.(modelCataloger); ok {
		catalog = source
	}
	return Handler{
		cfg:        cfg,
		db:         db,
		sessions:   sessions,
		verifier:   verifier,
		openrouter: streamer,
		models:     catalog,
		files:      fileStore,
	}
}

type contextKey string

const sessionUserContextKey contextKey = "session_user"

type modelResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Provider          string  `json:"provider"`
	ContextWindow     int     `json:"contextWindow"`
	PromptPriceMUSD   float64 `json:"promptPriceMicrosUsd"`
	OutputPriceMUSD   float64 `json:"outputPriceMicrosUsd"`
	SupportsReasoning bool    `json:"supportsReasoning"`
	Curated           bool    `json:"curated"`
}

type modelPreferencesResponse struct {
	LastUsedModelID             string `json:"lastUsedModelId"`
	LastUsedDeepResearchModelID string `json:"lastUsedDeepResearchModelId"`
}

type reasoningPresetResponse struct {
	ModelID string `json:"modelId"`
	Mode    string `json:"mode"`
	Effort  string `json:"effort"`
}

type listModelsResponse struct {
	Models           []modelResponse           `json:"models"`
	Curated          []modelResponse           `json:"curatedModels"`
	Favorites        []string                  `json:"favorites"`
	Preferences      modelPreferencesResponse  `json:"preferences"`
	ReasoningPresets []reasoningPresetResponse `json:"reasoningPresets"`
}

type syncModelsResponse struct {
	Synced int `json:"synced"`
}

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

	// In dev mode, if the user exists by email, use their real Google Subject
	// to avoid unique constraint violations on the email column.
	if h.cfg.InsecureSkipGoogleVerify {
		existing, err := h.sessions.GetUserByEmail(r.Context(), identity.Email)
		if err == nil {
			identity.GoogleSubject = existing.GoogleSub
		}
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
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	models, err := h.listActiveModels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read models")
		return
	}
	if len(models) == 0 {
		models = append(models, modelResponse{
			ID:                h.cfg.OpenRouterDefaultModel,
			Name:              "OpenRouter Free",
			Provider:          "openrouter",
			ContextWindow:     0,
			PromptPriceMUSD:   0,
			OutputPriceMUSD:   0,
			SupportsReasoning: true,
			Curated:           true,
		})
	}

	favorites, err := h.listUserModelFavorites(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read favorites")
		return
	}

	preferences, err := h.readUserModelPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read preferences")
		return
	}
	reasoningPresets, err := h.listUserReasoningPresets(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read reasoning presets")
		return
	}

	allowed := make(map[string]struct{}, len(models))
	reasoningSupported := make(map[string]bool, len(models))
	curated := make([]modelResponse, 0, len(models))
	for _, model := range models {
		allowed[model.ID] = struct{}{}
		reasoningSupported[model.ID] = model.SupportsReasoning
		if model.Curated {
			curated = append(curated, model)
		}
	}

	preferences = normalizeModelPreferences(preferences, allowed, h.cfg.OpenRouterDefaultModel)
	filteredFavorites := filterKnownModelIDs(favorites, allowed)
	filteredReasoningPresets := filterReasoningPresets(reasoningPresets, allowed, reasoningSupported)

	writeJSON(w, http.StatusOK, listModelsResponse{
		Models:           models,
		Curated:          curated,
		Favorites:        filteredFavorites,
		Preferences:      preferences,
		ReasoningPresets: filteredReasoningPresets,
	})
}

func (h Handler) SyncModels(w http.ResponseWriter, r *http.Request) {
	synced, err := h.syncModelsFromProvider(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "sync_failed", "failed to sync models from provider")
		return
	}

	writeJSON(w, http.StatusOK, syncModelsResponse{Synced: synced})
}

func (h Handler) listActiveModels(ctx context.Context) ([]modelResponse, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT id, display_name, provider, context_window, prompt_price_microusd, completion_price_microusd, curated
     , supports_reasoning
FROM models
WHERE is_active = 1
ORDER BY curated DESC, updated_at DESC
LIMIT 500;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]modelResponse, 0, 16)
	for rows.Next() {
		var m modelResponse
		var supportsReasoning int
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.ContextWindow, &m.PromptPriceMUSD, &m.OutputPriceMUSD, &m.Curated, &supportsReasoning); err != nil {
			return nil, err
		}
		m.SupportsReasoning = supportsReasoning == 1
		models = append(models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

func (h Handler) syncModelsFromProvider(ctx context.Context) (int, error) {
	if h.models == nil {
		return 0, nil
	}

	models, err := h.models.ListModels(ctx)
	if err != nil {
		return 0, err
	}
	if len(models) == 0 {
		return 0, nil
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE models
SET is_active = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE provider = 'openrouter'
  AND curated = 0;
`); err != nil {
		return 0, err
	}

	synced := 0
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		supportedParameters, err := json.Marshal(model.SupportedParameters)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO models (
  id,
  provider,
  display_name,
  context_window,
  prompt_price_microusd,
  completion_price_microusd,
  supported_parameters_json,
  supports_reasoning,
  curated,
  is_active
)
VALUES (?, 'openrouter', ?, ?, ?, ?, ?, ?, 0, 1)
ON CONFLICT(id) DO UPDATE SET
  provider = excluded.provider,
  display_name = excluded.display_name,
  context_window = excluded.context_window,
  prompt_price_microusd = excluded.prompt_price_microusd,
  completion_price_microusd = excluded.completion_price_microusd,
  supported_parameters_json = excluded.supported_parameters_json,
  supports_reasoning = excluded.supports_reasoning,
  is_active = 1,
  updated_at = CURRENT_TIMESTAMP;
`, model.ID, model.Name, model.ContextWindow, model.PromptPriceMicrosUSD, model.CompletionPriceMicrosUSD, string(supportedParameters), boolToInt(model.SupportsReasoning)); err != nil {
			return 0, err
		}
		synced++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return synced, nil
}

type updateModelPreferencesRequest struct {
	Mode    string `json:"mode"`
	ModelID string `json:"modelId"`
}

func (h Handler) UpdateModelPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	var req updateModelPreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	mode := strings.TrimSpace(req.Mode)
	if mode != "chat" && mode != "deep_research" {
		writeError(w, http.StatusBadRequest, "invalid_request", "mode must be one of: chat, deep_research")
		return
	}

	modelID := fallback(req.ModelID, h.cfg.OpenRouterDefaultModel)
	preferences, err := h.persistModelSelection(r.Context(), user.ID, mode, modelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to persist preferences")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"preferences": preferences})
}

type updateModelFavoriteRequest struct {
	ModelID  string `json:"modelId"`
	Favorite bool   `json:"favorite"`
}

type updateReasoningPresetRequest struct {
	ModelID string `json:"modelId"`
	Mode    string `json:"mode"`
	Effort  string `json:"effort"`
}

func (h Handler) UpdateModelFavorite(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	var req updateModelFavoriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "modelId is required")
		return
	}

	if err := h.setModelFavorite(r.Context(), user.ID, modelID, req.Favorite); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to update favorite")
		return
	}

	favorites, err := h.listUserModelFavorites(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read favorites")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"favorites": favorites})
}

func (h Handler) UpdateModelReasoningPreset(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	var req updateReasoningPresetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "modelId is required")
		return
	}

	mode := strings.TrimSpace(req.Mode)
	if mode != "chat" && mode != "deep_research" {
		writeError(w, http.StatusBadRequest, "invalid_request", "mode must be one of: chat, deep_research")
		return
	}

	effort, ok := normalizeReasoningEffort(req.Effort)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "effort must be one of: low, medium, high")
		return
	}

	if err := h.setReasoningPreset(r.Context(), user.ID, modelID, mode, effort); err != nil {
		if errors.Is(err, errReasoningUnsupportedModel) {
			writeError(w, http.StatusBadRequest, "invalid_request", "selected model does not support reasoning controls")
			return
		}
		writeError(w, http.StatusInternalServerError, "db_error", "failed to update reasoning preset")
		return
	}

	presets, err := h.listUserReasoningPresets(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read reasoning presets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"reasoningPresets": presets})
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

type citationResponse struct {
	URL            string `json:"url"`
	Title          string `json:"title,omitempty"`
	Snippet        string `json:"snippet,omitempty"`
	SourceProvider string `json:"sourceProvider,omitempty"`
}

type usageResponse struct {
	PromptTokens               int      `json:"promptTokens"`
	CompletionTokens           int      `json:"completionTokens"`
	TotalTokens                int      `json:"totalTokens"`
	ReasoningTokens            *int     `json:"reasoningTokens,omitempty"`
	CostMicrosUSD              *int     `json:"costMicrosUsd,omitempty"`
	ByokInferenceCostMicrosUSD *int     `json:"byokInferenceCostMicrosUsd,omitempty"`
	TokensPerSecond            *float64 `json:"tokensPerSecond,omitempty"`
	ModelID                    string   `json:"modelId,omitempty"`
	ProviderName               string   `json:"providerName,omitempty"`
}

type messageResponse struct {
	ID                  string             `json:"id"`
	ConversationID      string             `json:"conversationId"`
	Role                string             `json:"role"`
	Content             string             `json:"content"`
	ReasoningContent    *string            `json:"reasoningContent,omitempty"`
	ThinkingTrace       *thinkingTrace     `json:"thinkingTrace,omitempty"`
	ModelID             *string            `json:"modelId,omitempty"`
	Usage               *usageResponse     `json:"usage,omitempty"`
	GroundingEnabled    bool               `json:"groundingEnabled"`
	DeepResearchEnabled bool               `json:"deepResearchEnabled"`
	Citations           []citationResponse `json:"citations"`
	CreatedAt           string             `json:"createdAt"`
}

func (h Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
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
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
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
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
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
SELECT m.id, m.conversation_id, m.role, m.content, m.reasoning_content, m.thinking_trace_json, m.model_id, m.prompt_tokens, m.completion_tokens, m.total_tokens, m.reasoning_tokens, m.cost_microusd, m.byok_inference_cost_microusd, m.tokens_per_second, m.usage_model_id, m.usage_provider_name, m.grounding_enabled, m.deep_research_enabled, m.created_at
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = ? AND c.user_id = ?
ORDER BY m.created_at ASC, m.rowid ASC;
`, conversationID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read messages")
		return
	}
	defer rows.Close()

	messages := make([]messageResponse, 0, 32)
	for rows.Next() {
		var message messageResponse
		var reasoningContent sql.NullString
		var thinkingTraceJSON sql.NullString
		var modelID sql.NullString
		var promptTokens sql.NullInt64
		var completionTokens sql.NullInt64
		var totalTokens sql.NullInt64
		var reasoningTokens sql.NullInt64
		var costMicrosUSD sql.NullInt64
		var byokInferenceCostMicrosUSD sql.NullInt64
		var tokensPerSecond sql.NullFloat64
		var usageModelID sql.NullString
		var usageProviderName sql.NullString
		var groundingEnabled int
		var deepResearchEnabled int

		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&reasoningContent,
			&thinkingTraceJSON,
			&modelID,
			&promptTokens,
			&completionTokens,
			&totalTokens,
			&reasoningTokens,
			&costMicrosUSD,
			&byokInferenceCostMicrosUSD,
			&tokensPerSecond,
			&usageModelID,
			&usageProviderName,
			&groundingEnabled,
			&deepResearchEnabled,
			&message.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "failed to parse messages")
			return
		}

		message.ReasoningContent = nullableStringPointer(reasoningContent)
		if thinkingTraceJSON.Valid {
			if trace, ok := decodeThinkingTraceJSON(thinkingTraceJSON.String); ok {
				message.ThinkingTrace = trace
			}
		}
		message.ModelID = nullableStringPointer(modelID)
		if promptTokens.Valid && completionTokens.Valid && totalTokens.Valid {
			message.Usage = &usageResponse{
				PromptTokens:               int(promptTokens.Int64),
				CompletionTokens:           int(completionTokens.Int64),
				TotalTokens:                int(totalTokens.Int64),
				ReasoningTokens:            nullableIntPointer(reasoningTokens),
				CostMicrosUSD:              nullableIntPointer(costMicrosUSD),
				ByokInferenceCostMicrosUSD: nullableIntPointer(byokInferenceCostMicrosUSD),
				TokensPerSecond:            nullableFloatPointer(tokensPerSecond),
				ModelID:                    strings.TrimSpace(usageModelID.String),
				ProviderName:               normalizeProviderName(usageProviderName.String),
			}
		}
		message.GroundingEnabled = groundingEnabled == 1
		message.DeepResearchEnabled = deepResearchEnabled == 1
		message.Citations = make([]citationResponse, 0)
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to iterate messages")
		return
	}

	citationsByMessageID, err := h.listConversationCitations(r.Context(), user.ID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read citations")
		return
	}

	for i := range messages {
		if citations, ok := citationsByMessageID[messages[i].ID]; ok {
			messages[i].Citations = citations
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (h Handler) listConversationCitations(ctx context.Context, userID, conversationID string) (map[string][]citationResponse, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT c.message_id, c.url, c.title, c.snippet, c.source_provider
FROM citations c
JOIN messages m ON m.id = c.message_id
JOIN conversations v ON v.id = m.conversation_id
WHERE v.user_id = ? AND v.id = ?
ORDER BY c.created_at ASC, c.rowid ASC;
`, userID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]citationResponse)
	for rows.Next() {
		var messageID string
		var citation citationResponse
		var title sql.NullString
		var snippet sql.NullString
		var sourceProvider sql.NullString

		if err := rows.Scan(&messageID, &citation.URL, &title, &snippet, &sourceProvider); err != nil {
			return nil, err
		}

		if title.Valid {
			citation.Title = strings.TrimSpace(title.String)
		}
		if snippet.Valid {
			citation.Snippet = strings.TrimSpace(snippet.String)
		}
		if sourceProvider.Valid {
			citation.SourceProvider = strings.TrimSpace(sourceProvider.String)
		}

		out[messageID] = append(out[messageID], citation)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (h Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	conversationID := strings.TrimSpace(chi.URLParam(r, "id"))
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation id is required")
		return
	}

	candidates, err := h.listConversationBlobRefs(r.Context(), user.ID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to load attachment references")
		return
	}

	result, err := h.db.ExecContext(r.Context(), `
DELETE FROM conversations
WHERE id = ? AND user_id = ?;
`, conversationID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to delete conversation")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to delete conversation")
		return
	}
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, "conversation_not_found", "conversation not found")
		return
	}

	h.cleanupOrphanedFileBlobs(r.Context(), user.ID, candidates)

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h Handler) DeleteAllConversations(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	candidates, err := h.listAllUserConversationBlobRefs(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to load attachment references")
		return
	}

	if _, err := h.db.ExecContext(r.Context(), `
DELETE FROM conversations
WHERE user_id = ?;
`, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to delete conversations")
		return
	}

	h.cleanupOrphanedFileBlobs(r.Context(), user.ID, candidates)

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

type chatMessageRequest struct {
	ConversationID  string   `json:"conversationId"`
	Message         string   `json:"message"`
	ModelID         string   `json:"modelId"`
	ReasoningEffort string   `json:"reasoningEffort"`
	Grounding       *bool    `json:"grounding"`
	DeepResearch    *bool    `json:"deepResearch"`
	FileIDs         []string `json:"fileIds"`
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
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}

	files, normalizedFileIDs, err := h.resolveUserFiles(r.Context(), user.ID, req.FileIDs)
	if errors.Is(err, errTooManyFileIDs) {
		writeError(w, http.StatusBadRequest, "invalid_request", "a maximum of 5 attachments is supported")
		return
	}
	if errors.Is(err, errInvalidFileIDs) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve attachments")
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
	mode := "chat"
	if deepResearch {
		mode = "deep_research"
	}
	if _, err := h.persistModelSelection(r.Context(), user.ID, mode, modelID); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to persist model preferences")
		return
	}

	reasoningEffort, err := h.resolveReasoningEffort(r.Context(), user.ID, modelID, mode, req.ReasoningEffort)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidReasoningEffort):
			writeError(w, http.StatusBadRequest, "invalid_request", "reasoningEffort must be one of: low, medium, high")
		case errors.Is(err, errReasoningUnsupportedModel):
			writeError(w, http.StatusBadRequest, "invalid_request", "selected model does not support reasoning controls")
		default:
			writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve reasoning effort")
		}
		return
	}

	conversationID, err := h.resolveConversationID(r.Context(), user.ID, req.ConversationID, req.Message)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation_not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve conversation")
		return
	}

	historyMessages, err := h.listConversationPromptMessages(r.Context(), user.ID, conversationID, maxConversationHistoryMessages)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to load conversation history")
		return
	}

	userMessageID, err := h.insertUserMessageWithFiles(
		r.Context(),
		user.ID,
		conversationID,
		req.Message,
		modelID,
		grounding,
		deepResearch,
		normalizedFileIDs,
	)
	if err != nil {
		if errors.Is(err, errInvalidFileIDs) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "db_error", "failed to persist message")
		return
	}

	userPrompt := h.appendFileContextToPrompt(req.Message, files)
	timeSensitive := isTimeSensitivePrompt(req.Message)
	if deepResearch {
		h.streamDeepResearchResponse(r.Context(), w, flusher, deepResearchStreamInput{
			UserID:          user.ID,
			UserMessageID:   userMessageID,
			ConversationID:  conversationID,
			ModelID:         modelID,
			ReasoningEffort: reasoningEffort,
			Message:         req.Message,
			Prompt:          userPrompt,
			Grounding:       grounding,
			IsAnonymous:     user.GoogleSub == "anonymous",
			History:         historyMessages,
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	metadataEvent := map[string]any{
		"type":           "metadata",
		"grounding":      grounding,
		"deepResearch":   deepResearch,
		"modelId":        modelID,
		"conversationId": conversationID,
	}
	if reasoningEffort != "" {
		metadataEvent["reasoningEffort"] = reasoningEffort
	}
	if err := writeSSEEvent(w, metadataEvent); err != nil {
		writeError(w, http.StatusInternalServerError, "stream_error", "failed to start stream")
		return
	}
	flusher.Flush()

	traceCollector := newThinkingTraceCollector()

	groundingCitations, groundingWarning := h.resolveGroundingContext(
		r.Context(),
		req.Message,
		grounding,
		timeSensitive,
		func(progress research.Progress) {
			traceCollector.AppendProgress(progress)
			_ = writeSSEEvent(w, progressEventData(progress))
			flusher.Flush()
		},
	)
	if groundingWarning != "" {
		_ = writeSSEEvent(w, map[string]any{
			"type":    "warning",
			"scope":   "grounding",
			"message": groundingWarning,
		})
		flusher.Flush()
	}

	if grounding {
		synthesizingProgress := summarizedProgress(research.Progress{
			Phase:   research.PhaseSynthesizing,
			Message: "Preparing grounded response",
		}, research.ProgressSummaryInput{
			Phase: research.PhaseSynthesizing,
		})
		traceCollector.AppendProgress(synthesizingProgress)
		_ = writeSSEEvent(w, progressEventData(synthesizingProgress))
		flusher.Flush()
	}

	promptMessages := []openrouter.Message{
		{Role: "system", Content: buildSystemPrompt(grounding, false, len(groundingCitations) > 0, timeSensitive)},
	}
	if len(groundingCitations) > 0 {
		promptMessages = append(promptMessages, openrouter.Message{
			Role:    "system",
			Content: buildGroundingPrompt(groundingCitations, timeSensitive),
		})
	}
	promptMessages = append(promptMessages, historyMessages...)
	promptMessages = append(promptMessages, openrouter.Message{Role: "user", Content: userPrompt})

	started := true
	var assistantContent strings.Builder
	var reasoningContent strings.Builder
	var assistantUsage *openrouter.Usage
	var streamStartedAt time.Time
	var firstTokenAt time.Time

	markFirstTokenAt := func() {
		if firstTokenAt.IsZero() {
			firstTokenAt = time.Now()
		}
	}

	streamErr := h.openrouter.StreamChatCompletion(
		r.Context(),
		openrouter.StreamRequest{
			Model:     modelID,
			Messages:  promptMessages,
			Reasoning: openRouterReasoningConfig(reasoningEffort),
		},
		func() error {
			streamStartedAt = time.Now()
			return nil
		},
		func(delta string) error {
			assistantContent.WriteString(delta)
			markFirstTokenAt()

			if err := writeSSEEvent(w, map[string]any{
				"type":  "token",
				"delta": delta,
			}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
		func(reasoning string) error {
			reasoningContent.WriteString(reasoning)
			markFirstTokenAt()

			if err := writeSSEEvent(w, map[string]any{
				"type":  "reasoning",
				"delta": reasoning,
			}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
		func(usage openrouter.Usage) error {
			copied := usageWithLocalUsageFallbacks(usage, modelID, streamStartedAt, firstTokenAt)
			assistantUsage = &copied

			if err := writeSSEEvent(w, map[string]any{
				"type":  "usage",
				"usage": usageResponseFromOpenRouter(copied),
			}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
	)

	if grounding {
		finalizingProgress := summarizedProgress(research.Progress{
			Phase:   research.PhaseFinalizing,
			Message: "Finalizing citations and response",
		}, research.ProgressSummaryInput{
			Phase: research.PhaseFinalizing,
		})
		traceCollector.AppendProgress(finalizingProgress)
		_ = writeSSEEvent(w, progressEventData(finalizingProgress))
		flusher.Flush()
	}

	if streamErr != nil {
		traceCollector.MarkStopped("Stopped due to an error")
	} else {
		traceCollector.MarkDone()
	}

	var persistedCitations []citationResponse
	if assistantContent.Len() > 0 {
		persistedCitations = groundingCitations
		if len(persistedCitations) > maxNormalCitations {
			persistedCitations = persistedCitations[:maxNormalCitations]
		}
		assistantMessageID, err := h.insertMessageWithCitations(
			r.Context(),
			user.ID,
			conversationID,
			"assistant",
			assistantContent.String(),
			reasoningContent.String(),
			modelID,
			grounding,
			deepResearch,
			persistedCitations,
			traceCollector.Snapshot(),
			messageUsageFromOpenRouter(assistantUsage),
		)
		if err != nil {
			if !started {
				writeError(w, http.StatusInternalServerError, "db_error", "failed to persist assistant response")
				return
			}
			_ = writeSSEEvent(w, map[string]any{
				"type":    "error",
				"message": "failed to persist assistant response",
			})
			flusher.Flush()
		} else {
			if started && len(persistedCitations) > 0 {
				_ = writeSSEEvent(w, map[string]any{
					"type":      "citations",
					"citations": persistedCitations,
				})
				flusher.Flush()
			}
			if assistantUsage != nil {
				h.enrichAndPersistMessageUsageAsync(user.ID, assistantMessageID, modelID, *assistantUsage, streamStartedAt, firstTokenAt)
			}
		}
	}

	if streamErr != nil {
		if !started {
			status := http.StatusBadGateway
			code := "openrouter_error"
			message := fmt.Sprintf("failed to stream from OpenRouter: %v", streamErr)
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

const maxGroundingResults = 10
const maxConversationHistoryMessages = 24

func (h Handler) resolveGroundingContext(
	ctx context.Context,
	message string,
	enabled, timeSensitive bool,
	onProgress func(research.Progress),
) ([]citationResponse, string) {
	if !enabled {
		return nil, ""
	}

	if h.cfg.AgenticResearchChatEnabled {
		result, err := h.runResearchOrchestrator(ctx, research.ModeChat, message, timeSensitive, onProgress)
		if err == nil {
			citations := convertResearchCitations(result.Citations, maxGroundingResults)
			warning := researchWarning(result)
			if len(citations) > 0 || warning != "" {
				return citations, warning
			}
		}
	}

	return h.resolveGroundingContextLegacy(ctx, message, enabled, timeSensitive, onProgress)
}

func (h Handler) resolveGroundingContextLegacy(
	ctx context.Context,
	message string,
	enabled, timeSensitive bool,
	onProgress func(research.Progress),
) ([]citationResponse, string) {
	if !enabled {
		return nil, ""
	}

	if h.grounding == nil {
		return nil, "Grounding is unavailable for this response."
	}

	queries := []string{strings.TrimSpace(message)}
	if timeSensitive {
		queries = append(queries, strings.TrimSpace(message)+" official release notes changelog")
	}
	if onProgress != nil {
		onProgress(summarizedProgress(research.Progress{
			Phase:       research.PhasePlanning,
			Message:     fmt.Sprintf("Planned %d research passes", len(queries)),
			TotalPasses: len(queries),
		}, research.ProgressSummaryInput{
			Phase: research.PhasePlanning,
		}))
	}

	citations := make([]citationResponse, 0, maxGroundingResults)
	seenURLs := make(map[string]struct{}, maxGroundingResults)
	for idx, query := range queries {
		if query == "" {
			continue
		}
		if idx > 0 {
			if err := waitWithContext(ctx, braveFreeTierSpacing); err != nil {
				return citations, ""
			}
		}
		if onProgress != nil {
			onProgress(summarizedProgress(research.Progress{
				Phase:       research.PhaseSearching,
				Message:     fmt.Sprintf("Searching pass %d of %d", idx+1, len(queries)),
				Pass:        idx + 1,
				TotalPasses: len(queries),
			}, research.ProgressSummaryInput{
				Phase:      research.PhaseSearching,
				QueryCount: 1,
				Decision:   research.ProgressDecisionSearchMore,
			}))
		}

		results, err := h.grounding.Search(ctx, query, maxGroundingResults)
		if isBraveRateLimitError(err) {
			if waitErr := waitWithContext(ctx, braveFreeTierSpacing); waitErr == nil {
				results, err = h.grounding.Search(ctx, query, maxGroundingResults)
			}
		}
		if err != nil {
			if errors.Is(err, brave.ErrMissingAPIKey) {
				return nil, "Grounding is unavailable because BRAVE_API_KEY is not configured."
			}
			logGroundingSearchFailure("chat_legacy", idx+1, len(queries), query, err)
			if idx == 0 {
				return nil, "Grounding search failed. Continuing without web sources."
			}
			continue
		}

		for _, result := range results {
			rawURL := strings.TrimSpace(result.URL)
			if rawURL == "" {
				continue
			}
			if _, exists := seenURLs[rawURL]; exists {
				continue
			}
			seenURLs[rawURL] = struct{}{}

			citations = append(citations, citationResponse{
				URL:            rawURL,
				Title:          trimToRunes(strings.TrimSpace(result.Title), 240),
				Snippet:        trimToRunes(strings.TrimSpace(result.Snippet), 800),
				SourceProvider: "brave",
			})

			if len(citations) >= maxGroundingResults {
				return citations, ""
			}
		}
	}

	return citations, ""
}

func isBraveRateLimitError(err error) bool {
	var apiErr brave.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests
}

func braveStatusCode(err error) int {
	var apiErr brave.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

func logGroundingSearchFailure(scope string, pass, total int, query string, err error) {
	log.Printf(
		"grounding search failed: scope=%s pass=%d total=%d query_chars=%d status_code=%d err=%v",
		strings.TrimSpace(scope),
		pass,
		total,
		len([]rune(strings.TrimSpace(query))),
		braveStatusCode(err),
		err,
	)
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

func (h Handler) RequireModelSyncToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(h.cfg.ModelSyncBearerToken)
		if expected == "" {
			writeError(w, http.StatusServiceUnavailable, "sync_auth_not_configured", "model sync token is not configured")
			return
		}

		actual, err := readBearerToken(r)
		if err != nil || actual != expected {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}

		next.ServeHTTP(w, r)
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

func (h Handler) persistedSessionUser(ctx context.Context, user session.User) (session.User, error) {
	if h.cfg.AuthRequired {
		return user, nil
	}

	upserted, err := h.sessions.UpsertUser(ctx, user.GoogleSub, user.Email, user.Name, user.AvatarURL)
	if err != nil {
		return session.User{}, err
	}
	return upserted, nil
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

func (h Handler) listConversationPromptMessages(ctx context.Context, userID, conversationID string, limit int) ([]openrouter.Message, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := h.db.QueryContext(ctx, `
SELECT m.role, m.content
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = ?
  AND c.user_id = ?
  AND m.role IN ('user', 'assistant')
ORDER BY m.created_at DESC, m.rowid DESC
LIMIT ?;
`, conversationID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]openrouter.Message, 0, limit)
	for rows.Next() {
		var role string
		var content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		messages = append(messages, openrouter.Message{
			Role:    role,
			Content: content,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (h Handler) insertMessage(ctx context.Context, userID, conversationID, role, content, modelID string, groundingEnabled, deepResearchEnabled bool) error {
	_, err := h.insertMessageWithCitations(
		ctx,
		userID,
		conversationID,
		role,
		content,
		"", // no reasoning content for user messages
		modelID,
		groundingEnabled,
		deepResearchEnabled,
		nil,
		nil,
		nil,
	)
	return err
}

type messageUsage struct {
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	ReasoningTokens            *int
	CostMicrosUSD              *int
	ByokInferenceCostMicrosUSD *int
	TokensPerSecond            *float64
	ModelID                    string
	ProviderName               string
}

func (h Handler) insertMessageWithCitations(
	ctx context.Context,
	userID, conversationID, role, content, reasoningContent, modelID string,
	groundingEnabled, deepResearchEnabled bool,
	citations []citationResponse,
	thinkingTrace *thinkingTrace,
	usage *messageUsage,
) (string, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	nullableModelID, err := resolveNullableModelID(ctx, tx, modelID)
	if err != nil {
		return "", err
	}

	messageID := uuid.NewString()
	var promptTokensValue any
	var completionTokensValue any
	var totalTokensValue any
	var reasoningTokensValue any
	var costMicrosUSDValue any
	var byokInferenceCostMicrosUSDValue any
	var tokensPerSecondValue any
	var usageModelIDValue any
	var usageProviderNameValue any
	thinkingTraceJSON, err := encodeThinkingTraceJSON(thinkingTrace)
	if err != nil {
		return "", err
	}
	if usage != nil {
		promptTokensValue = usage.PromptTokens
		completionTokensValue = usage.CompletionTokens
		totalTokensValue = usage.TotalTokens
		reasoningTokensValue = usage.ReasoningTokens
		costMicrosUSDValue = usage.CostMicrosUSD
		byokInferenceCostMicrosUSDValue = usage.ByokInferenceCostMicrosUSD
		tokensPerSecondValue = usage.TokensPerSecond
		usageModelIDValue = nullableString(usage.ModelID)
		usageProviderNameValue = nullableString(usage.ProviderName)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (
  id,
  conversation_id,
  user_id,
  role,
  content,
  reasoning_content,
  thinking_trace_json,
  model_id,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  reasoning_tokens,
  cost_microusd,
  byok_inference_cost_microusd,
  tokens_per_second,
  usage_model_id,
  usage_provider_name,
  grounding_enabled,
  deep_research_enabled
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, messageID, conversationID, userID, role, content, nullableString(reasoningContent), thinkingTraceJSON, nullableModelID, promptTokensValue, completionTokensValue, totalTokensValue, reasoningTokensValue, costMicrosUSDValue, byokInferenceCostMicrosUSDValue, tokensPerSecondValue, usageModelIDValue, usageProviderNameValue, boolToInt(groundingEnabled), boolToInt(deepResearchEnabled)); err != nil {
		return "", err
	}

	for _, citation := range citations {
		rawURL := strings.TrimSpace(citation.URL)
		if rawURL == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO citations (
  id,
  message_id,
  url,
  title,
  snippet,
  source_provider
)
VALUES (?, ?, ?, ?, ?, ?);
`, uuid.NewString(), messageID, rawURL, nullableString(citation.Title), nullableString(citation.Snippet), nullableString(citation.SourceProvider)); err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;
`, conversationID, userID); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return messageID, nil
}

func (h Handler) persistModelSelection(ctx context.Context, userID, mode, modelID string) (modelPreferencesResponse, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return modelPreferencesResponse{}, err
	}
	defer tx.Rollback()

	resolvedModelID, err := ensureModelExists(ctx, tx, modelID)
	if err != nil {
		return modelPreferencesResponse{}, err
	}

	var lastUsedModelID sql.NullString
	var lastUsedDeepResearchModelID sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT last_used_model_id, last_used_deep_research_model_id
FROM user_model_preferences
WHERE user_id = ?
LIMIT 1;
`, userID).Scan(&lastUsedModelID, &lastUsedDeepResearchModelID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return modelPreferencesResponse{}, err
	}

	preferences := modelPreferencesResponse{
		LastUsedModelID:             strings.TrimSpace(lastUsedModelID.String),
		LastUsedDeepResearchModelID: strings.TrimSpace(lastUsedDeepResearchModelID.String),
	}

	switch mode {
	case "chat":
		preferences.LastUsedModelID = resolvedModelID
		if preferences.LastUsedDeepResearchModelID == "" {
			preferences.LastUsedDeepResearchModelID = resolvedModelID
		}
	case "deep_research":
		preferences.LastUsedDeepResearchModelID = resolvedModelID
		if preferences.LastUsedModelID == "" {
			preferences.LastUsedModelID = resolvedModelID
		}
	}

	if err := upsertUserModelPreferences(ctx, tx, userID, preferences); err != nil {
		return modelPreferencesResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return modelPreferencesResponse{}, err
	}

	return preferences, nil
}

func (h Handler) setModelFavorite(ctx context.Context, userID, modelID string, favorite bool) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	resolvedModelID, err := ensureModelExists(ctx, tx, modelID)
	if err != nil {
		return err
	}

	if favorite {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO user_model_favorites (user_id, model_id)
VALUES (?, ?)
ON CONFLICT(user_id, model_id) DO NOTHING;
`, userID, resolvedModelID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM user_model_favorites
WHERE user_id = ? AND model_id = ?;
`, userID, resolvedModelID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (h Handler) setReasoningPreset(ctx context.Context, userID, modelID, mode, effort string) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	resolvedModelID, err := ensureModelExists(ctx, tx, modelID)
	if err != nil {
		return err
	}

	supportsReasoning, err := modelSupportsReasoning(ctx, tx, resolvedModelID)
	if err != nil {
		return err
	}
	if !supportsReasoning {
		return errReasoningUnsupportedModel
	}

	if err := upsertUserReasoningPreset(ctx, tx, userID, resolvedModelID, mode, effort); err != nil {
		return err
	}

	return tx.Commit()
}

func (h Handler) listUserModelFavorites(ctx context.Context, userID string) ([]string, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT f.model_id
FROM user_model_favorites f
JOIN models m ON m.id = f.model_id
WHERE f.user_id = ?
  AND m.is_active = 1
ORDER BY f.created_at DESC, f.model_id ASC;
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	favorites := make([]string, 0, 16)
	for rows.Next() {
		var modelID string
		if err := rows.Scan(&modelID); err != nil {
			return nil, err
		}
		favorites = append(favorites, modelID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favorites, nil
}

func (h Handler) readUserModelPreferences(ctx context.Context, userID string) (modelPreferencesResponse, error) {
	var lastUsedModelID sql.NullString
	var lastUsedDeepResearchModelID sql.NullString
	err := h.db.QueryRowContext(ctx, `
SELECT last_used_model_id, last_used_deep_research_model_id
FROM user_model_preferences
WHERE user_id = ?
LIMIT 1;
`, userID).Scan(&lastUsedModelID, &lastUsedDeepResearchModelID)
	if errors.Is(err, sql.ErrNoRows) {
		return modelPreferencesResponse{}, nil
	}
	if err != nil {
		return modelPreferencesResponse{}, err
	}

	return modelPreferencesResponse{
		LastUsedModelID:             strings.TrimSpace(lastUsedModelID.String),
		LastUsedDeepResearchModelID: strings.TrimSpace(lastUsedDeepResearchModelID.String),
	}, nil
}

func (h Handler) listUserReasoningPresets(ctx context.Context, userID string) ([]reasoningPresetResponse, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT p.model_id, p.mode, p.effort
FROM user_model_reasoning_presets p
JOIN models m ON m.id = p.model_id
WHERE p.user_id = ?
  AND m.is_active = 1
ORDER BY p.updated_at DESC;
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	presets := make([]reasoningPresetResponse, 0, 16)
	for rows.Next() {
		var preset reasoningPresetResponse
		if err := rows.Scan(&preset.ModelID, &preset.Mode, &preset.Effort); err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return presets, nil
}

func (h Handler) resolveReasoningEffort(ctx context.Context, userID, modelID, mode, rawOverride string) (string, error) {
	override := strings.TrimSpace(rawOverride)
	if override != "" {
		effort, ok := normalizeReasoningEffort(override)
		if !ok {
			return "", errInvalidReasoningEffort
		}
		if err := h.setReasoningPreset(ctx, userID, modelID, mode, effort); err != nil {
			return "", err
		}
		return effort, nil
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	resolvedModelID, err := ensureModelExists(ctx, tx, modelID)
	if err != nil {
		return "", err
	}

	supportsReasoning, err := modelSupportsReasoning(ctx, tx, resolvedModelID)
	if err != nil {
		return "", err
	}
	if !supportsReasoning {
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return "", nil
	}

	var storedEffort sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT effort
FROM user_model_reasoning_presets
WHERE user_id = ?
  AND model_id = ?
  AND mode = ?
LIMIT 1;
`, userID, resolvedModelID, mode).Scan(&storedEffort)
	switch {
	case err == nil:
		if err := tx.Commit(); err != nil {
			return "", err
		}
		effort, ok := normalizeReasoningEffort(storedEffort.String)
		if !ok {
			return h.defaultReasoningEffortForMode(mode), nil
		}
		return effort, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return h.defaultReasoningEffortForMode(mode), nil
	default:
		return "", err
	}
}

func upsertUserModelPreferences(ctx context.Context, tx *sql.Tx, userID string, preferences modelPreferencesResponse) error {
	normalModelID := nullableString(preferences.LastUsedModelID)
	deepModelID := nullableString(preferences.LastUsedDeepResearchModelID)

	_, err := tx.ExecContext(ctx, `
INSERT INTO user_model_preferences (
  user_id,
  last_used_model_id,
  last_used_deep_research_model_id,
  updated_at
)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id) DO UPDATE SET
  last_used_model_id = excluded.last_used_model_id,
  last_used_deep_research_model_id = excluded.last_used_deep_research_model_id,
  updated_at = CURRENT_TIMESTAMP;
`, userID, normalModelID, deepModelID)
	return err
}

func upsertUserReasoningPreset(ctx context.Context, tx *sql.Tx, userID, modelID, mode, effort string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_model_reasoning_presets (
  user_id,
  model_id,
  mode,
  effort,
  updated_at
)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id, model_id, mode) DO UPDATE SET
  effort = excluded.effort,
  updated_at = CURRENT_TIMESTAMP;
`, userID, modelID, mode, effort)
	return err
}

func modelSupportsReasoning(ctx context.Context, tx *sql.Tx, modelID string) (bool, error) {
	var supportsReasoning int
	err := tx.QueryRowContext(ctx, `
SELECT supports_reasoning
FROM models
WHERE id = ?
LIMIT 1;
`, modelID).Scan(&supportsReasoning)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return supportsReasoning == 1, nil
}

func ensureModelExists(ctx context.Context, tx *sql.Tx, rawModelID string) (string, error) {
	modelID := strings.TrimSpace(rawModelID)
	if modelID == "" {
		return "", nil
	}

	var existingID string
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM models
WHERE id = ?
LIMIT 1;
`, modelID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO models (
  id,
  provider,
  display_name,
  context_window,
  prompt_price_microusd,
  completion_price_microusd,
  supported_parameters_json,
  supports_reasoning,
  curated,
  is_active
)
VALUES (?, 'openrouter', ?, 0, 0, 0, NULL, 0, 0, 1)
ON CONFLICT(id) DO UPDATE SET
  is_active = 1,
  updated_at = CURRENT_TIMESTAMP;
`, modelID, modelID); err != nil {
		return "", err
	}

	return modelID, nil
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

func readBearerToken(r *http.Request) (string, error) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return "", errors.New("missing authorization header")
	}

	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid authorization header")
	}

	return parts[1], nil
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

func normalizeModelPreferences(preferences modelPreferencesResponse, available map[string]struct{}, defaultModelID string) modelPreferencesResponse {
	normalizedDefault := strings.TrimSpace(defaultModelID)

	if _, ok := available[preferences.LastUsedModelID]; !ok {
		preferences.LastUsedModelID = normalizedDefault
	}
	if preferences.LastUsedModelID == "" {
		preferences.LastUsedModelID = normalizedDefault
	}

	if _, ok := available[preferences.LastUsedDeepResearchModelID]; !ok {
		preferences.LastUsedDeepResearchModelID = preferences.LastUsedModelID
	}
	if preferences.LastUsedDeepResearchModelID == "" {
		preferences.LastUsedDeepResearchModelID = preferences.LastUsedModelID
	}

	return preferences
}

func filterKnownModelIDs(modelIDs []string, available map[string]struct{}) []string {
	if len(modelIDs) == 0 {
		return make([]string, 0)
	}

	filtered := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		if _, ok := available[modelID]; ok {
			filtered = append(filtered, modelID)
		}
	}
	return filtered
}

func filterReasoningPresets(
	presets []reasoningPresetResponse,
	available map[string]struct{},
	reasoningSupported map[string]bool,
) []reasoningPresetResponse {
	if len(presets) == 0 {
		return make([]reasoningPresetResponse, 0)
	}

	filtered := make([]reasoningPresetResponse, 0, len(presets))
	seen := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		if _, ok := available[preset.ModelID]; !ok {
			continue
		}
		if !reasoningSupported[preset.ModelID] {
			continue
		}
		effort, ok := normalizeReasoningEffort(preset.Effort)
		if !ok {
			continue
		}
		key := preset.ModelID + "|" + preset.Mode
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, reasoningPresetResponse{
			ModelID: preset.ModelID,
			Mode:    preset.Mode,
			Effort:  effort,
		})
	}
	return filtered
}

func normalizeReasoningEffort(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low":
		return "low", true
	case "medium":
		return "medium", true
	case "high":
		return "high", true
	default:
		return "", false
	}
}

func (h Handler) defaultReasoningEffortForMode(mode string) string {
	if mode == "deep_research" {
		effort, ok := normalizeReasoningEffort(h.cfg.DefaultDeepReasoningEffort)
		if ok {
			return effort
		}
		return "high"
	}

	effort, ok := normalizeReasoningEffort(h.cfg.DefaultChatReasoningEffort)
	if ok {
		return effort
	}
	return "medium"
}

func openRouterReasoningConfig(effort string) *openrouter.ReasoningConfig {
	normalized, ok := normalizeReasoningEffort(effort)
	if !ok {
		return nil
	}
	return &openrouter.ReasoningConfig{Effort: normalized}
}

func (h Handler) usageWithOpenRouterMetrics(
	ctx context.Context,
	usage openrouter.Usage,
	requestedModelID string,
	streamStartedAt, firstTokenAt time.Time,
) openrouter.Usage {
	enriched := usageWithLocalUsageFallbacks(usage, requestedModelID, streamStartedAt, firstTokenAt)

	generationID := strings.TrimSpace(usage.GenerationID)
	if generationID != "" {
		if generation, err := h.getGenerationWithRetry(ctx, generationID); err == nil {
			if modelID := strings.TrimSpace(generation.ModelID); modelID != "" {
				enriched.ModelID = modelID
			}
			if providerName := strings.TrimSpace(generation.ProviderName); providerName != "" {
				enriched.ProviderName = normalizeProviderName(providerName)
			}
			if generation.UpstreamInferenceCostMicros != nil {
				enriched.ByokInferenceCostMicros = generation.UpstreamInferenceCostMicros
			}
			if tokensPerSecond := tokensPerSecondFromGeneration(generation, usage); tokensPerSecond != nil {
				enriched.TokensPerSecond = tokensPerSecond
			}
		} else {
			log.Printf("openrouter generation lookup failed: generation_id=%s err=%v", generationID, err)
		}
	}

	return enriched
}

func usageWithLocalUsageFallbacks(usage openrouter.Usage, requestedModelID string, streamStartedAt, firstTokenAt time.Time) openrouter.Usage {
	enriched := usage
	if strings.TrimSpace(enriched.ModelID) == "" {
		if modelID := strings.TrimSpace(requestedModelID); modelID != "" {
			enriched.ModelID = modelID
		}
	}
	if strings.TrimSpace(enriched.ProviderName) == "" {
		enriched.ProviderName = providerNameFromModelID(enriched.ModelID)
	}
	return usageWithTokensPerSecond(enriched, streamStartedAt, firstTokenAt)
}

func (h Handler) enrichAndPersistMessageUsageAsync(
	userID, messageID, requestedModelID string,
	usage openrouter.Usage,
	streamStartedAt, firstTokenAt time.Time,
) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(messageID) == "" {
		return
	}
	if strings.TrimSpace(usage.GenerationID) == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		enriched := h.usageWithOpenRouterMetrics(ctx, usage, requestedModelID, streamStartedAt, firstTokenAt)
		if err := h.updateMessageUsage(ctx, userID, messageID, messageUsageFromOpenRouter(&enriched)); err != nil {
			log.Printf("message usage enrichment persist failed: message_id=%s err=%v", messageID, err)
		}
	}()
}

func (h Handler) updateMessageUsage(ctx context.Context, userID, messageID string, usage *messageUsage) error {
	if usage == nil {
		return nil
	}

	var promptTokensValue any = usage.PromptTokens
	var completionTokensValue any = usage.CompletionTokens
	var totalTokensValue any = usage.TotalTokens
	var reasoningTokensValue any = usage.ReasoningTokens
	var costMicrosUSDValue any = usage.CostMicrosUSD
	var byokInferenceCostMicrosUSDValue any = usage.ByokInferenceCostMicrosUSD
	var tokensPerSecondValue any = usage.TokensPerSecond
	var usageModelIDValue any = nullableString(usage.ModelID)
	var usageProviderNameValue any = nullableString(usage.ProviderName)

	_, err := h.db.ExecContext(ctx, `
UPDATE messages
SET
  prompt_tokens = ?,
  completion_tokens = ?,
  total_tokens = ?,
  reasoning_tokens = ?,
  cost_microusd = ?,
  byok_inference_cost_microusd = ?,
  tokens_per_second = ?,
  usage_model_id = ?,
  usage_provider_name = ?
WHERE id = ? AND user_id = ?;
`, promptTokensValue, completionTokensValue, totalTokensValue, reasoningTokensValue, costMicrosUSDValue, byokInferenceCostMicrosUSDValue, tokensPerSecondValue, usageModelIDValue, usageProviderNameValue, messageID, userID)
	return err
}

func (h Handler) getGenerationWithRetry(ctx context.Context, generationID string) (openrouter.Generation, error) {
	var lastErr error
	retryDelays := []time.Duration{0, 900 * time.Millisecond, 2 * time.Second, 4 * time.Second}

	for _, delay := range retryDelays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return openrouter.Generation{}, ctx.Err()
			case <-timer.C:
			}
		}

		generation, err := h.openrouter.GetGeneration(ctx, generationID)
		if err == nil {
			return generation, nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = errors.New("generation lookup failed")
	}
	return openrouter.Generation{}, lastErr
}

func tokensPerSecondFromGeneration(generation openrouter.Generation, usage openrouter.Usage) *float64 {
	if generation.GenerationTimeMs == nil || *generation.GenerationTimeMs <= 0 {
		return nil
	}

	completionTokens := 0
	switch {
	case generation.NativeTokensCompletion != nil && *generation.NativeTokensCompletion > 0:
		completionTokens = *generation.NativeTokensCompletion
	case generation.TokensCompletion != nil && *generation.TokensCompletion > 0:
		completionTokens = *generation.TokensCompletion
	case usage.CompletionTokens > 0:
		completionTokens = usage.CompletionTokens
	}
	if completionTokens <= 0 {
		return nil
	}

	perSecond := float64(completionTokens) / (*generation.GenerationTimeMs / 1000)
	if perSecond <= 0 {
		return nil
	}
	return &perSecond
}

func usageWithTokensPerSecond(usage openrouter.Usage, streamStartedAt, firstTokenAt time.Time) openrouter.Usage {
	if usage.TokensPerSecond != nil {
		return usage
	}

	if usage.CompletionTokens <= 0 {
		return usage
	}

	startedAt := firstTokenAt
	if startedAt.IsZero() {
		startedAt = streamStartedAt
	}
	if startedAt.IsZero() {
		return usage
	}

	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		return usage
	}

	tokensPerSecond := float64(usage.CompletionTokens) / elapsed
	if tokensPerSecond <= 0 {
		return usage
	}

	usage.TokensPerSecond = &tokensPerSecond
	return usage
}

func usageResponseFromOpenRouter(usage openrouter.Usage) usageResponse {
	return usageResponse{
		PromptTokens:               usage.PromptTokens,
		CompletionTokens:           usage.CompletionTokens,
		TotalTokens:                usage.TotalTokens,
		ReasoningTokens:            usage.ReasoningTokens,
		CostMicrosUSD:              usage.CostMicrosUSD,
		ByokInferenceCostMicrosUSD: usage.ByokInferenceCostMicros,
		TokensPerSecond:            usage.TokensPerSecond,
		ModelID:                    usage.ModelID,
		ProviderName:               normalizeProviderName(usage.ProviderName),
	}
}

func messageUsageFromOpenRouter(usage *openrouter.Usage) *messageUsage {
	if usage == nil {
		return nil
	}
	return &messageUsage{
		PromptTokens:               usage.PromptTokens,
		CompletionTokens:           usage.CompletionTokens,
		TotalTokens:                usage.TotalTokens,
		ReasoningTokens:            usage.ReasoningTokens,
		CostMicrosUSD:              usage.CostMicrosUSD,
		ByokInferenceCostMicrosUSD: usage.ByokInferenceCostMicros,
		TokensPerSecond:            usage.TokensPerSecond,
		ModelID:                    usage.ModelID,
		ProviderName:               normalizeProviderName(usage.ProviderName),
	}
}

func normalizeProviderName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "Google" {
		return "Google Vertex"
	}
	return name
}

func providerNameFromModelID(modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return ""
	}

	prefix := strings.ToLower(trimmed)
	if idx := strings.Index(prefix, "/"); idx >= 0 {
		prefix = prefix[:idx]
	}

	switch prefix {
	case "google":
		return "Google Vertex"
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "x-ai", "xai":
		return "xAI"
	case "meta", "meta-llama":
		return "Meta"
	case "mistral", "mistralai":
		return "Mistral"
	case "cohere":
		return "Cohere"
	case "deepseek":
		return "DeepSeek"
	case "qwen":
		return "Qwen"
	case "perplexity":
		return "Perplexity"
	default:
		return ""
	}
}

func nullableString(raw string) sql.NullString {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: trimmed,
		Valid:  true,
	}
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

func nullableIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}

func nullableFloatPointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
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

func buildSystemPrompt(grounding, deepResearch, hasGroundingContext, timeSensitive bool) string {
	mode := "normal chat"
	if deepResearch {
		mode = "deep research"
	}

	timeSensitiveInstruction := ""
	if timeSensitive {
		timeSensitiveInstruction = " For time-sensitive questions (latest/current/today), do not assert an exact latest fact unless explicitly supported by the provided sources. If evidence is stale, missing dates, or conflicting, say you cannot verify the latest status."
	}

	if grounding && hasGroundingContext {
		return fmt.Sprintf(
			"You are a helpful assistant in %s mode. Use grounded, factual answers from the provided sources and call out uncertainty.%s",
			mode, timeSensitiveInstruction,
		)
	}
	if grounding {
		return fmt.Sprintf("You are a helpful assistant in %s mode. Use grounded, factual answers and call out uncertainty.%s", mode, timeSensitiveInstruction)
	}
	return fmt.Sprintf("You are a helpful assistant in %s mode.", mode)
}

func buildGroundingPrompt(citations []citationResponse, timeSensitive bool) string {
	if len(citations) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Grounding context from recent web search results:\n")
	if timeSensitive {
		builder.WriteString(fmt.Sprintf("Current date (UTC): %s\n", time.Now().UTC().Format("2006-01-02")))
		builder.WriteString("Prioritize sources with explicit recent dates and official release/changelog pages.\n")
	}
	for i, citation := range citations {
		label := strings.TrimSpace(citation.Title)
		if label == "" {
			label = citation.URL
		}

		builder.WriteString(fmt.Sprintf("\n[%d] %s\nURL: %s\n", i+1, label, citation.URL))
		if snippet := strings.TrimSpace(citation.Snippet); snippet != "" {
			builder.WriteString("Snippet: ")
			builder.WriteString(snippet)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nUse this context when relevant. If evidence is weak, say so clearly.")
	return strings.TrimSpace(builder.String())
}

func isTimeSensitivePrompt(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}

	keywords := []string{
		"latest",
		"newest",
		"current",
		"today",
		"right now",
		"as of",
		"recent",
		"this week",
		"this month",
		"breaking",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
