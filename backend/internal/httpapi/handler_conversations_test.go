package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/brave"
	"chat/backend/internal/config"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/session"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

func TestCreateAndListConversations(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(`{"title":"  First   Chat  "}`))
	createReq = requestWithSessionUser(createReq, user)
	createResp := httptest.NewRecorder()

	handler.CreateConversation(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createResp.Code)
	}

	var created struct {
		Conversation conversationResponse `json:"conversation"`
	}
	decodeJSONBody(t, createResp, &created)
	if created.Conversation.Title != "First Chat" {
		t.Fatalf("unexpected normalized title: %q", created.Conversation.Title)
	}
	if created.Conversation.ID == "" {
		t.Fatal("expected conversation id to be set")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations", nil)
	listReq = requestWithSessionUser(listReq, user)
	listResp := httptest.NewRecorder()

	handler.ListConversations(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listResp.Code)
	}

	var listed struct {
		Conversations []conversationResponse `json:"conversations"`
	}
	decodeJSONBody(t, listResp, &listed)
	if len(listed.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(listed.Conversations))
	}
	if listed.Conversations[0].ID != created.Conversation.ID {
		t.Fatalf("unexpected conversation id: %q", listed.Conversations[0].ID)
	}
}

func TestConversationLifecycleCreateListGetAndDelete(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(`{"title":"Lifecycle Chat"}`))
	createReq = requestWithSessionUser(createReq, user)
	createResp := httptest.NewRecorder()

	handler.CreateConversation(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createResp.Code)
	}

	var created struct {
		Conversation conversationResponse `json:"conversation"`
	}
	decodeJSONBody(t, createResp, &created)

	if err := handler.insertMessage(context.Background(), user.ID, created.Conversation.ID, "user", "hello lifecycle", "", true, false); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations", nil)
	listReq = requestWithSessionUser(listReq, user)
	listResp := httptest.NewRecorder()

	handler.ListConversations(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listResp.Code)
	}
	var listed struct {
		Conversations []conversationResponse `json:"conversations"`
	}
	decodeJSONBody(t, listResp, &listed)
	if len(listed.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(listed.Conversations))
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+created.Conversation.ID+"/messages", nil)
	getReq = requestWithSessionUser(getReq, user)
	getReq = requestWithConversationID(getReq, created.Conversation.ID)
	getResp := httptest.NewRecorder()

	handler.ListConversationMessages(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getResp.Code)
	}
	var listedMessages struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, getResp, &listedMessages)
	if len(listedMessages.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(listedMessages.Messages))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/conversations/"+created.Conversation.ID, nil)
	deleteReq = requestWithSessionUser(deleteReq, user)
	deleteReq = requestWithConversationID(deleteReq, created.Conversation.ID)
	deleteResp := httptest.NewRecorder()

	handler.DeleteConversation(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	}
	var deletePayload struct {
		Success bool `json:"success"`
	}
	decodeJSONBody(t, deleteResp, &deletePayload)
	if !deletePayload.Success {
		t.Fatalf("expected success=true, got %+v", deletePayload)
	}

	listReqAfterDelete := httptest.NewRequest(http.MethodGet, "/v1/conversations", nil)
	listReqAfterDelete = requestWithSessionUser(listReqAfterDelete, user)
	listRespAfterDelete := httptest.NewRecorder()

	handler.ListConversations(listRespAfterDelete, listReqAfterDelete)

	if listRespAfterDelete.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRespAfterDelete.Code)
	}
	var listedAfterDelete struct {
		Conversations []conversationResponse `json:"conversations"`
	}
	decodeJSONBody(t, listRespAfterDelete, &listedAfterDelete)
	if len(listedAfterDelete.Conversations) != 0 {
		t.Fatalf("expected 0 conversations, got %d", len(listedAfterDelete.Conversations))
	}

	getReqAfterDelete := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+created.Conversation.ID+"/messages", nil)
	getReqAfterDelete = requestWithSessionUser(getReqAfterDelete, user)
	getReqAfterDelete = requestWithConversationID(getReqAfterDelete, created.Conversation.ID)
	getRespAfterDelete := httptest.NewRecorder()

	handler.ListConversationMessages(getRespAfterDelete, getReqAfterDelete)

	if getRespAfterDelete.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, getRespAfterDelete.Code)
	}

	var messageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ?;`, created.Conversation.ID).Scan(&messageCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected messages to be deleted by cascade, got %d", messageCount)
	}
}

func TestDeleteAllConversationsScopedByUser(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user1 := session.User{ID: "user-1"}
	user2 := session.User{ID: "user-2"}
	seedUser(t, db, user1.ID, "user1@example.com")
	seedUser(t, db, user2.ID, "user2@example.com")

	conversation1, err := handler.insertConversation(context.Background(), user1.ID, "U1 Chat A")
	if err != nil {
		t.Fatalf("insert conversation 1: %v", err)
	}
	conversation2, err := handler.insertConversation(context.Background(), user1.ID, "U1 Chat B")
	if err != nil {
		t.Fatalf("insert conversation 2: %v", err)
	}
	otherConversation, err := handler.insertConversation(context.Background(), user2.ID, "U2 Chat")
	if err != nil {
		t.Fatalf("insert other conversation: %v", err)
	}

	if err := handler.insertMessage(context.Background(), user1.ID, conversation1.ID, "user", "message one", "", true, false); err != nil {
		t.Fatalf("insert message one: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user1.ID, conversation2.ID, "assistant", "message two", "", true, false); err != nil {
		t.Fatalf("insert message two: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user2.ID, otherConversation.ID, "user", "keep me", "", true, false); err != nil {
		t.Fatalf("insert other user message: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/conversations", nil)
	deleteReq = requestWithSessionUser(deleteReq, user1)
	deleteResp := httptest.NewRecorder()

	handler.DeleteAllConversations(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	}

	var user1ConversationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE user_id = ?;`, user1.ID).Scan(&user1ConversationCount); err != nil {
		t.Fatalf("count user1 conversations: %v", err)
	}
	if user1ConversationCount != 0 {
		t.Fatalf("expected user1 conversations to be deleted, got %d", user1ConversationCount)
	}

	var user1MessageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE user_id = ?;`, user1.ID).Scan(&user1MessageCount); err != nil {
		t.Fatalf("count user1 messages: %v", err)
	}
	if user1MessageCount != 0 {
		t.Fatalf("expected user1 messages to be deleted, got %d", user1MessageCount)
	}

	var user2ConversationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE user_id = ?;`, user2.ID).Scan(&user2ConversationCount); err != nil {
		t.Fatalf("count user2 conversations: %v", err)
	}
	if user2ConversationCount != 1 {
		t.Fatalf("expected user2 conversation to remain, got %d", user2ConversationCount)
	}

	var user2MessageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE user_id = ?;`, user2.ID).Scan(&user2MessageCount); err != nil {
		t.Fatalf("count user2 messages: %v", err)
	}
	if user2MessageCount != 1 {
		t.Fatalf("expected user2 message to remain, got %d", user2MessageCount)
	}
}

func TestDeleteConversationNotOwnedReturnsNotFound(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	owner := session.User{ID: "owner-1"}
	other := session.User{ID: "other-1"}
	seedUser(t, db, owner.ID, "owner@example.com")
	seedUser(t, db, other.ID, "other@example.com")

	conversation, err := handler.insertConversation(context.Background(), owner.ID, "Owner Chat")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/conversations/"+conversation.ID, nil)
	deleteReq = requestWithSessionUser(deleteReq, other)
	deleteReq = requestWithConversationID(deleteReq, conversation.ID)
	deleteResp := httptest.NewRecorder()

	handler.DeleteConversation(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, deleteResp.Code)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE id = ?;`, conversation.ID).Scan(&count); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected conversation to remain, got %d", count)
	}
}

func TestListConversationMessagesScopedByUser(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user1 := session.User{ID: "user-1"}
	user2 := session.User{ID: "user-2"}
	seedUser(t, db, user1.ID, "user1@example.com")
	seedUser(t, db, user2.ID, "user2@example.com")

	conversation, err := handler.insertConversation(context.Background(), user1.ID, "Alpha")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user1.ID, conversation.ID, "user", "hello", "", true, false); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	ownerReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversation.ID+"/messages", nil)
	ownerReq = requestWithSessionUser(ownerReq, user1)
	ownerReq = requestWithConversationID(ownerReq, conversation.ID)
	ownerResp := httptest.NewRecorder()

	handler.ListConversationMessages(ownerResp, ownerReq)

	if ownerResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, ownerResp.Code)
	}
	var ownerPayload struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, ownerResp, &ownerPayload)
	if len(ownerPayload.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(ownerPayload.Messages))
	}
	if ownerPayload.Messages[0].Content != "hello" {
		t.Fatalf("unexpected message content: %q", ownerPayload.Messages[0].Content)
	}

	otherReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversation.ID+"/messages", nil)
	otherReq = requestWithSessionUser(otherReq, user2)
	otherReq = requestWithConversationID(otherReq, conversation.ID)
	otherResp := httptest.NewRecorder()

	handler.ListConversationMessages(otherResp, otherReq)

	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, otherResp.Code)
	}
}

func TestListModelsIncludesCatalogFavoritesAndPreferences(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{
		catalog: []openrouter.Model{
			{
				ID:                       "openai/gpt-4o-mini",
				Name:                     "GPT-4o mini",
				ContextWindow:            128000,
				PromptPriceMicrosUSD:     150,
				CompletionPriceMicrosUSD: 600,
			},
		},
	})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	if _, err := db.Exec(`
INSERT INTO models (id, provider, display_name, context_window, prompt_price_microusd, completion_price_microusd, curated, is_active)
VALUES ('anthropic/claude-3.5-haiku', 'openrouter', 'Claude Haiku', 200000, 800, 1200, 0, 1);
`); err != nil {
		t.Fatalf("seed additional model: %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO user_model_favorites (user_id, model_id)
VALUES (?, ?);
`, user.ID, "anthropic/claude-3.5-haiku"); err != nil {
		t.Fatalf("seed favorite: %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO user_model_preferences (user_id, last_used_model_id, last_used_deep_research_model_id)
VALUES (?, ?, ?);
`, user.ID, "anthropic/claude-3.5-haiku", "openrouter/free"); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	syncReq := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	syncReq = requestWithSessionUser(syncReq, user)
	syncResp := httptest.NewRecorder()
	handler.SyncModels(syncResp, syncReq)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, syncResp.Code, syncResp.Body.String())
	}

	handler.ListModels(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var payload listModelsResponse
	decodeJSONBody(t, resp, &payload)

	if len(payload.Models) < 3 {
		t.Fatalf("expected synced models to be returned, got %d", len(payload.Models))
	}

	foundSynced := false
	for _, model := range payload.Models {
		if model.ID == "openai/gpt-4o-mini" {
			foundSynced = true
			break
		}
	}
	if !foundSynced {
		t.Fatalf("expected synced model in response: %+v", payload.Models)
	}

	if len(payload.Curated) == 0 || payload.Curated[0].ID != "openrouter/free" {
		t.Fatalf("expected curated list to include default seeded model, got %+v", payload.Curated)
	}

	if len(payload.Favorites) != 1 || payload.Favorites[0] != "anthropic/claude-3.5-haiku" {
		t.Fatalf("unexpected favorites: %+v", payload.Favorites)
	}

	if payload.Preferences.LastUsedModelID != "anthropic/claude-3.5-haiku" {
		t.Fatalf("unexpected last used model id: %q", payload.Preferences.LastUsedModelID)
	}
	if payload.Preferences.LastUsedDeepResearchModelID != "openrouter/free" {
		t.Fatalf("unexpected last used deep research model id: %q", payload.Preferences.LastUsedDeepResearchModelID)
	}
}

func TestListModelsDoesNotAutoSyncCatalog(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{
		catalog: []openrouter.Model{
			{
				ID:                       "openai/gpt-4o-mini",
				Name:                     "GPT-4o mini",
				ContextWindow:            128000,
				PromptPriceMicrosUSD:     150,
				CompletionPriceMicrosUSD: 600,
			},
		},
	})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ListModels(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var payload listModelsResponse
	decodeJSONBody(t, resp, &payload)

	for _, model := range payload.Models {
		if model.ID == "openai/gpt-4o-mini" {
			t.Fatalf("expected catalog model to be absent before manual sync, got %+v", payload.Models)
		}
	}
}

func TestSyncModelsReturnsSyncedCount(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{
		catalog: []openrouter.Model{
			{
				ID:                       "openai/gpt-4o-mini",
				Name:                     "GPT-4o mini",
				ContextWindow:            128000,
				PromptPriceMicrosUSD:     150,
				CompletionPriceMicrosUSD: 600,
			},
			{
				ID:                       "anthropic/claude-3.5-haiku",
				Name:                     "Claude Haiku",
				ContextWindow:            200000,
				PromptPriceMicrosUSD:     800,
				CompletionPriceMicrosUSD: 1200,
			},
		},
	})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")

	req := httptest.NewRequest(http.MethodPost, "/v1/models/sync", nil)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.SyncModels(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var payload syncModelsResponse
	decodeJSONBody(t, resp, &payload)
	if payload.Synced != 2 {
		t.Fatalf("expected 2 synced models, got %d", payload.Synced)
	}
}

func TestUpdateModelFavoritePersistsFavorite(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	addReq := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/favorites",
		strings.NewReader(`{"modelId":"openrouter/free","favorite":true}`),
	)
	addReq = requestWithSessionUser(addReq, user)
	addResp := httptest.NewRecorder()

	handler.UpdateModelFavorite(addResp, addReq)

	if addResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, addResp.Code, addResp.Body.String())
	}
	var addPayload struct {
		Favorites []string `json:"favorites"`
	}
	decodeJSONBody(t, addResp, &addPayload)
	if len(addPayload.Favorites) != 1 || addPayload.Favorites[0] != "openrouter/free" {
		t.Fatalf("unexpected favorites payload: %+v", addPayload.Favorites)
	}

	var count int
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM user_model_favorites
WHERE user_id = ? AND model_id = ?;
`, user.ID, "openrouter/free").Scan(&count); err != nil {
		t.Fatalf("count favorites: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected favorite to be persisted, got %d", count)
	}

	removeReq := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/favorites",
		strings.NewReader(`{"modelId":"openrouter/free","favorite":false}`),
	)
	removeReq = requestWithSessionUser(removeReq, user)
	removeResp := httptest.NewRecorder()

	handler.UpdateModelFavorite(removeResp, removeReq)

	if removeResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, removeResp.Code, removeResp.Body.String())
	}
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM user_model_favorites
WHERE user_id = ? AND model_id = ?;
`, user.ID, "openrouter/free").Scan(&count); err != nil {
		t.Fatalf("count favorites after remove: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected favorite to be removed, got %d", count)
	}
}

func TestUpdateModelPreferencesPersistsModeSpecificSelection(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")
	if _, err := db.Exec(`
INSERT INTO models (id, provider, display_name, context_window, prompt_price_microusd, completion_price_microusd, curated, is_active)
VALUES ('openai/gpt-4o-mini', 'openrouter', 'GPT-4o mini', 128000, 150, 600, 0, 1);
`); err != nil {
		t.Fatalf("seed second model: %v", err)
	}

	chatReq := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/preferences",
		strings.NewReader(`{"mode":"chat","modelId":"openrouter/free"}`),
	)
	chatReq = requestWithSessionUser(chatReq, user)
	chatResp := httptest.NewRecorder()

	handler.UpdateModelPreferences(chatResp, chatReq)

	if chatResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, chatResp.Code, chatResp.Body.String())
	}

	deepReq := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/preferences",
		strings.NewReader(`{"mode":"deep_research","modelId":"openai/gpt-4o-mini"}`),
	)
	deepReq = requestWithSessionUser(deepReq, user)
	deepResp := httptest.NewRecorder()

	handler.UpdateModelPreferences(deepResp, deepReq)

	if deepResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, deepResp.Code, deepResp.Body.String())
	}

	var payload struct {
		Preferences modelPreferencesResponse `json:"preferences"`
	}
	decodeJSONBody(t, deepResp, &payload)
	if payload.Preferences.LastUsedModelID != "openrouter/free" {
		t.Fatalf("unexpected last used model id: %q", payload.Preferences.LastUsedModelID)
	}
	if payload.Preferences.LastUsedDeepResearchModelID != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected deep research model id: %q", payload.Preferences.LastUsedDeepResearchModelID)
	}
}

func TestUpdateModelReasoningPresetPersistsByModelAndMode(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/reasoning-presets",
		strings.NewReader(`{"modelId":"openrouter/free","mode":"chat","effort":"high"}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.UpdateModelReasoningPreset(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var payload struct {
		ReasoningPresets []reasoningPresetResponse `json:"reasoningPresets"`
	}
	decodeJSONBody(t, resp, &payload)
	if len(payload.ReasoningPresets) != 1 {
		t.Fatalf("expected 1 reasoning preset, got %d", len(payload.ReasoningPresets))
	}
	if payload.ReasoningPresets[0].Effort != "high" {
		t.Fatalf("unexpected effort: %+v", payload.ReasoningPresets[0])
	}

	var effort string
	if err := db.QueryRow(`
SELECT effort
FROM user_model_reasoning_presets
WHERE user_id = ? AND model_id = ? AND mode = ?;
`, user.ID, "openrouter/free", "chat").Scan(&effort); err != nil {
		t.Fatalf("query reasoning preset: %v", err)
	}
	if effort != "high" {
		t.Fatalf("unexpected stored effort: %q", effort)
	}
}

func TestUpdateModelReasoningPresetRejectsUnsupportedModel(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")

	if _, err := db.Exec(`
INSERT INTO models (id, provider, display_name, context_window, prompt_price_microusd, completion_price_microusd, supports_reasoning, curated, is_active)
VALUES ('example/basic-model', 'openrouter', 'Basic', 32000, 0, 0, 0, 0, 1);
`); err != nil {
		t.Fatalf("seed model: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/v1/models/reasoning-presets",
		strings.NewReader(`{"modelId":"example/basic-model","mode":"chat","effort":"high"}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.UpdateModelReasoningPreset(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusBadRequest, resp.Code, resp.Body.String())
	}
}

func TestChatMessagesPersistsConversationAndMessages(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Hi", " there"}})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/messages", strings.NewReader(`{"message":"Hello","modelId":"openrouter/free"}`))
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"type":"metadata"`) {
		t.Fatalf("expected metadata event in stream body: %s", body)
	}
	if !strings.Contains(body, `"conversationId":"`) {
		t.Fatalf("expected conversationId in metadata event: %s", body)
	}

	var conversationID string
	if err := db.QueryRow(`SELECT id FROM conversations WHERE user_id = ? LIMIT 1;`, user.ID).Scan(&conversationID); err != nil {
		t.Fatalf("query conversation: %v", err)
	}

	rows, err := db.Query(`
SELECT role, content
FROM messages
WHERE conversation_id = ?
ORDER BY rowid ASC;
`, conversationID)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	type storedMessage struct {
		Role    string
		Content string
	}

	messages := make([]storedMessage, 0, 2)
	for rows.Next() {
		var message storedMessage
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			t.Fatalf("scan message: %v", err)
		}
		messages = append(messages, message)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Hi there" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}

	var lastUsedModelID sql.NullString
	var lastUsedDeepResearchModelID sql.NullString
	if err := db.QueryRow(`
SELECT last_used_model_id, last_used_deep_research_model_id
FROM user_model_preferences
WHERE user_id = ?;
`, user.ID).Scan(&lastUsedModelID, &lastUsedDeepResearchModelID); err != nil {
		t.Fatalf("query model preferences: %v", err)
	}
	if !lastUsedModelID.Valid || lastUsedModelID.String != "openrouter/free" {
		t.Fatalf("unexpected last_used_model_id: %+v", lastUsedModelID)
	}
	if !lastUsedDeepResearchModelID.Valid || lastUsedDeepResearchModelID.String != "openrouter/free" {
		t.Fatalf("unexpected last_used_deep_research_model_id: %+v", lastUsedDeepResearchModelID)
	}
}

func TestChatMessagesAppliesReasoningEffortOverrideAndPersistsPreset(t *testing.T) {
	var capturedRequest openrouter.StreamRequest
	handler, db := newTestHandler(t, stubStreamer{
		tokens: []string{"Hi"},
		onRequest: func(req openrouter.StreamRequest) {
			capturedRequest = req
		},
	})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/messages", strings.NewReader(`{"message":"Hello","modelId":"openrouter/free","reasoningEffort":"high"}`))
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	if capturedRequest.Reasoning == nil || capturedRequest.Reasoning.Effort != "high" {
		t.Fatalf("expected reasoning effort high, got %+v", capturedRequest.Reasoning)
	}

	var effort string
	if err := db.QueryRow(`
SELECT effort
FROM user_model_reasoning_presets
WHERE user_id = ? AND model_id = ? AND mode = ?;
`, user.ID, "openrouter/free", "chat").Scan(&effort); err != nil {
		t.Fatalf("query reasoning preset: %v", err)
	}
	if effort != "high" {
		t.Fatalf("unexpected stored effort: %q", effort)
	}
}

func TestChatMessagesIncludesConversationHistoryInPrompt(t *testing.T) {
	capturedRequests := make([]openrouter.StreamRequest, 0, 2)
	streamer := stubStreamer{
		tokens: []string{"Ack"},
		onRequest: func(req openrouter.StreamRequest) {
			capturedRequests = append(capturedRequests, req)
		},
	}
	handler, db := newTestHandler(t, streamer)
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	firstReq := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"I love mangoes.","modelId":"openrouter/free"}`),
	)
	firstReq = requestWithSessionUser(firstReq, user)
	firstResp := httptest.NewRecorder()

	handler.ChatMessages(firstResp, firstReq)

	if firstResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, firstResp.Code, firstResp.Body.String())
	}

	var conversationID string
	if err := db.QueryRow(`SELECT id FROM conversations WHERE user_id = ? LIMIT 1;`, user.ID).Scan(&conversationID); err != nil {
		t.Fatalf("query conversation: %v", err)
	}

	secondReq := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"conversationId":"`+conversationID+`","message":"What fruit do I love?","modelId":"openrouter/free"}`),
	)
	secondReq = requestWithSessionUser(secondReq, user)
	secondResp := httptest.NewRecorder()

	handler.ChatMessages(secondResp, secondReq)

	if secondResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, secondResp.Code, secondResp.Body.String())
	}
	if len(capturedRequests) != 2 {
		t.Fatalf("expected 2 streamed requests, got %d", len(capturedRequests))
	}

	secondPrompt := capturedRequests[1].Messages
	if len(secondPrompt) < 4 {
		t.Fatalf("expected history messages in second prompt, got %+v", secondPrompt)
	}

	historyStart := -1
	for i, message := range secondPrompt {
		if message.Role == "user" && message.Content == "I love mangoes." {
			historyStart = i
			break
		}
	}
	if historyStart == -1 {
		t.Fatalf("expected prior user message in second prompt, got %+v", secondPrompt)
	}
	if historyStart+1 >= len(secondPrompt) || secondPrompt[historyStart+1].Role != "assistant" || secondPrompt[historyStart+1].Content != "Ack" {
		t.Fatalf("expected prior assistant message after prior user message, got %+v", secondPrompt)
	}

	current := secondPrompt[len(secondPrompt)-1]
	if current.Role != "user" || current.Content != "What fruit do I love?" {
		t.Fatalf("unexpected current message in second prompt: %+v", current)
	}
}

func TestChatMessagesPersistsGroundingCitationsAndStreamsCitationEvent(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Grounded", " answer"}})
	t.Cleanup(func() { _ = db.Close() })

	handler.grounding = stubGrounder{
		results: []brave.SearchResult{
			{
				URL:     "https://example.com/one",
				Title:   "Example One",
				Snippet: "First snippet",
			},
			{
				URL:     "https://example.com/two",
				Title:   "Example Two",
				Snippet: "Second snippet",
			},
		},
	}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/messages", strings.NewReader(`{"message":"What happened?","modelId":"openrouter/free"}`))
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"type":"citations"`) {
		t.Fatalf("expected citations event, got: %s", resp.Body.String())
	}

	var citationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM citations;`).Scan(&citationCount); err != nil {
		t.Fatalf("count citations: %v", err)
	}
	if citationCount != 2 {
		t.Fatalf("expected 2 persisted citations, got %d", citationCount)
	}

	var conversationID string
	if err := db.QueryRow(`SELECT id FROM conversations WHERE user_id = ? LIMIT 1;`, user.ID).Scan(&conversationID); err != nil {
		t.Fatalf("query conversation: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversationID+"/messages", nil)
	listReq = requestWithSessionUser(listReq, user)
	listReq = requestWithConversationID(listReq, conversationID)
	listResp := httptest.NewRecorder()

	handler.ListConversationMessages(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, listResp.Code, listResp.Body.String())
	}

	var payload struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, listResp, &payload)

	if len(payload.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(payload.Messages))
	}
	assistant := payload.Messages[1]
	if assistant.Role != "assistant" {
		t.Fatalf("expected second message to be assistant, got %s", assistant.Role)
	}
	if len(assistant.Citations) != 2 {
		t.Fatalf("expected 2 assistant citations, got %d", len(assistant.Citations))
	}
}

func TestChatMessagesGroundingFailureStreamsWarningAndContinues(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Still", " works"}})
	t.Cleanup(func() { _ = db.Close() })

	handler.grounding = stubGrounder{err: errors.New("brave unavailable")}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/messages", strings.NewReader(`{"message":"Hello","modelId":"openrouter/free"}`))
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"type":"warning"`) {
		t.Fatalf("expected warning event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"token"`) {
		t.Fatalf("expected token events in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event in stream body: %s", body)
	}
}

func TestChatMessagesDeepResearchStreamsProgressInOrder(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Deep", " answer [1]"}})
	t.Cleanup(func() { _ = db.Close() })

	handler.grounding = stubGrounder{
		results: []brave.SearchResult{
			{URL: "https://gov.example.gov/report", Title: "Official report", Snippet: "Detailed 2026 update."},
			{URL: "https://docs.example.com/changelog", Title: "Changelog", Snippet: "Release notes and changes."},
		},
	}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Research this","modelId":"openrouter/free","deepResearch":true}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	events := decodeSSEEvents(t, resp.Body.String())
	phases := make([]string, 0, 8)
	for _, event := range events {
		if event.Type == "progress" {
			if phase, ok := event.Data["phase"].(string); ok {
				phases = append(phases, phase)
			}
		}
	}

	position := -1
	position = assertContainsPhaseInOrder(t, phases, "planning", position)
	position = assertContainsPhaseInOrder(t, phases, "searching", position)
	position = assertContainsPhaseInOrder(t, phases, "synthesizing", position)
	_ = assertContainsPhaseInOrder(t, phases, "finalizing", position)
}

func TestChatMessagesDeepResearchTimeoutUsesConfig(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"This should never stream"}})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.DeepResearchTimeoutSeconds = 1
	handler.grounding = stubGrounder{waitForContext: true}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Long running request","modelId":"openrouter/free","deepResearch":true}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	started := time.Now()
	handler.ChatMessages(resp, req)
	elapsed := time.Since(started)

	if elapsed > 3*time.Second {
		t.Fatalf("expected timeout close to configured value, elapsed=%v", elapsed)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"deep research timed out after 1 seconds"`) {
		t.Fatalf("expected timeout error in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event in stream body: %s", body)
	}
}

func TestChatMessagesDeepResearchFallbackOnSearchFailure(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Fallback", " synthesis"}})
	t.Cleanup(func() { _ = db.Close() })

	handler.grounding = stubGrounder{err: errors.New("brave unavailable")}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Research anyway","modelId":"openrouter/free","deepResearch":true}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"scope":"research"`) {
		t.Fatalf("expected research warning in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"token"`) {
		t.Fatalf("expected token events in stream body: %s", body)
	}
}

func TestChatMessagesDeepResearchCitationPersistenceOrderedByClaims(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Summary [2] then [1]."}})
	t.Cleanup(func() { _ = db.Close() })

	handler.grounding = stubGrounder{
		results: []brave.SearchResult{
			{URL: "https://gov.example.gov/report", Title: "Official report", Snippet: "Comprehensive official update with 2026 findings."},
			{URL: "https://research.example.edu/changelog", Title: "Academic changelog analysis", Snippet: "Detailed release timeline, methodology, and deployment notes with cited publication dates from 2026 and 2025."},
		},
	}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Rank sources","modelId":"openrouter/free","deepResearch":true}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var conversationID string
	if err := db.QueryRow(`SELECT id FROM conversations WHERE user_id = ? LIMIT 1;`, user.ID).Scan(&conversationID); err != nil {
		t.Fatalf("query conversation: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversationID+"/messages", nil)
	listReq = requestWithSessionUser(listReq, user)
	listReq = requestWithConversationID(listReq, conversationID)
	listResp := httptest.NewRecorder()

	handler.ListConversationMessages(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, listResp.Code, listResp.Body.String())
	}

	var payload struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, listResp, &payload)

	if len(payload.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(payload.Messages))
	}
	assistant := payload.Messages[1]
	if len(assistant.Citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(assistant.Citations))
	}
}

func TestOrderCitationsByClaimsFollowsBracketOrder(t *testing.T) {
	ordered := orderCitationsByClaims([]citationResponse{
		{URL: "https://one.example.com"},
		{URL: "https://two.example.com"},
		{URL: "https://three.example.com"},
	}, "Prioritize [2], then [1], and finally [3].")

	if len(ordered) != 3 {
		t.Fatalf("expected 3 citations, got %d", len(ordered))
	}
	if ordered[0].URL != "https://two.example.com" {
		t.Fatalf("expected first citation from [2], got %s", ordered[0].URL)
	}
	if ordered[1].URL != "https://one.example.com" {
		t.Fatalf("expected second citation from [1], got %s", ordered[1].URL)
	}
	if ordered[2].URL != "https://three.example.com" {
		t.Fatalf("expected third citation from [3], got %s", ordered[2].URL)
	}
}

func TestUploadFileStoresMetadataAndBlob(t *testing.T) {
	store := &stubFileStore{objects: make(map[string][]byte)}
	handler, db := newTestHandlerWithFileStore(t, stubStreamer{}, store)
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "notes.md")
	if err != nil {
		t.Fatalf("create multipart form file: %v", err)
	}
	if _, err := part.Write([]byte("# Notes\n\nAttachment text")); err != nil {
		t.Fatalf("write multipart payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/files", &body)
	req = requestWithSessionUser(req, user)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()

	handler.UploadFile(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, resp.Code, resp.Body.String())
	}

	var payload struct {
		File fileResponse `json:"file"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.File.ID == "" {
		t.Fatal("expected uploaded file id")
	}
	if payload.File.Filename != "notes.md" {
		t.Fatalf("unexpected filename: %s", payload.File.Filename)
	}

	var storageBackend string
	var storagePath string
	var extractedText string
	if err := db.QueryRow(`
SELECT storage_backend, storage_path, extracted_text
FROM files
WHERE id = ?;
`, payload.File.ID).Scan(&storageBackend, &storagePath, &extractedText); err != nil {
		t.Fatalf("query file metadata: %v", err)
	}
	if storageBackend != "gcs" {
		t.Fatalf("unexpected storage backend: %s", storageBackend)
	}
	if storagePath == "" {
		t.Fatal("expected non-empty storage path")
	}
	if !strings.Contains(extractedText, "Attachment text") {
		t.Fatalf("expected extracted text to include file content, got: %q", extractedText)
	}

	if _, ok := store.objects[storagePath]; !ok {
		t.Fatalf("expected uploaded blob at %s", storagePath)
	}
}

func TestChatMessagesPersistsMessageFilesAndUsesAttachmentPrompt(t *testing.T) {
	var capturedRequest openrouter.StreamRequest
	streamer := stubStreamer{
		tokens: []string{"ok"},
		onRequest: func(req openrouter.StreamRequest) {
			capturedRequest = req
		},
	}
	handler, db := newTestHandler(t, streamer)
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	if _, err := db.Exec(`
INSERT INTO files (
  id,
  user_id,
  filename,
  media_type,
  size_bytes,
  storage_backend,
  storage_path,
  extracted_text
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, "file-1", user.ID, "notes.md", "text/markdown", 42, "gcs", "chat-uploads/users/user-1/file-1/notes.md", "Attached facts go here."); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Summarize this","modelId":"openrouter/free","fileIds":["file-1"]}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var linkedCount int
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM message_files mf
JOIN messages m ON m.id = mf.message_id
WHERE mf.file_id = ? AND m.role = 'user';
`, "file-1").Scan(&linkedCount); err != nil {
		t.Fatalf("count message files: %v", err)
	}
	if linkedCount != 1 {
		t.Fatalf("expected 1 message-file link, got %d", linkedCount)
	}

	if len(capturedRequest.Messages) < 2 {
		t.Fatalf("expected at least 2 prompt messages, got %+v", capturedRequest.Messages)
	}
	if !strings.Contains(capturedRequest.Messages[1].Content, "Attached facts go here.") {
		t.Fatalf("expected attachment text in prompt, got: %q", capturedRequest.Messages[1].Content)
	}
}

func TestDeleteConversationCleansAttachmentBlobAndMetadata(t *testing.T) {
	store := &stubFileStore{objects: make(map[string][]byte)}
	handler, db := newTestHandlerWithFileStore(t, stubStreamer{}, store)
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")

	conversation, err := handler.insertConversation(context.Background(), user.ID, "With Attachments")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user.ID, conversation.ID, "user", "hello", "openrouter/free", true, false); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	var messageID string
	if err := db.QueryRow(`
SELECT id
FROM messages
WHERE conversation_id = ?
LIMIT 1;
`, conversation.ID).Scan(&messageID); err != nil {
		t.Fatalf("query message id: %v", err)
	}

	storagePath := "chat-uploads/users/user-1/file-1/notes.md"
	store.objects[storagePath] = []byte("blob-data")
	if _, err := db.Exec(`
INSERT INTO files (
  id,
  user_id,
  filename,
  media_type,
  size_bytes,
  storage_backend,
  storage_path,
  extracted_text
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, "file-1", user.ID, "notes.md", "text/markdown", 123, "gcs", storagePath, "attachment text"); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO message_files (message_id, file_id)
VALUES (?, ?);
`, messageID, "file-1"); err != nil {
		t.Fatalf("seed message_files: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/conversations/"+conversation.ID, nil)
	req = requestWithSessionUser(req, user)
	req = requestWithConversationID(req, conversation.ID)
	resp := httptest.NewRecorder()

	handler.DeleteConversation(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var fileCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM files WHERE id = ?;`, "file-1").Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected file metadata deletion, got %d rows", fileCount)
	}

	if len(store.deletedPaths) != 1 || store.deletedPaths[0] != storagePath {
		t.Fatalf("expected blob delete path %q, got %+v", storagePath, store.deletedPaths)
	}
}

func TestChatMessagesConversationOwnershipEnforced(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{tokens: []string{"Should", " not", " stream"}})
	t.Cleanup(func() { _ = db.Close() })

	owner := session.User{ID: "owner-1"}
	other := session.User{ID: "other-1"}
	seedUser(t, db, owner.ID, "owner@example.com")
	seedUser(t, db, other.ID, "other@example.com")
	seedModel(t, db, "openrouter/free")

	conversation, err := handler.insertConversation(context.Background(), owner.ID, "Owner Conversation")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"conversationId":"`+conversation.ID+`","message":"Hello","modelId":"openrouter/free"}`),
	)
	req = requestWithSessionUser(req, other)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusNotFound, resp.Code, resp.Body.String())
	}

	var conversationMessageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ?;`, conversation.ID).Scan(&conversationMessageCount); err != nil {
		t.Fatalf("count conversation messages: %v", err)
	}
	if conversationMessageCount != 0 {
		t.Fatalf("expected no messages to be persisted, got %d", conversationMessageCount)
	}

	var otherConversationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE user_id = ?;`, other.ID).Scan(&otherConversationCount); err != nil {
		t.Fatalf("count other conversations: %v", err)
	}
	if otherConversationCount != 0 {
		t.Fatalf("expected no conversation to be auto-created, got %d", otherConversationCount)
	}
}

func TestCreateConversationInAuthDisabledMode(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.AuthRequired = false

	req := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(`{"title":"Anon Chat"}`))
	req = requestWithSessionUser(req, anonymousUser())
	resp := httptest.NewRecorder()

	handler.CreateConversation(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, resp.Code, resp.Body.String())
	}

	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE google_sub = ?;`, "anonymous").Scan(&userCount); err != nil {
		t.Fatalf("count anonymous user: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected anonymous user to be persisted, got %d", userCount)
	}
}

func newTestHandler(t *testing.T, streamer chatStreamer) (Handler, *sql.DB) {
	return newTestHandlerWithFileStore(t, streamer, nil)
}

func newTestHandlerWithFileStore(t *testing.T, streamer chatStreamer, fileStore fileObjectStore) (Handler, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	cfg := config.Config{
		AuthRequired:               true,
		SessionCookieName:          "chat_session",
		OpenRouterDefaultModel:     "openrouter/free",
		DefaultChatReasoningEffort: "medium",
		DefaultDeepReasoningEffort: "high",
	}

	handler := NewHandlerWithFileStore(cfg, db, session.NewStore(db), auth.NewVerifier(cfg), streamer, fileStore)
	handler.grounding = stubGrounder{}
	return handler, db
}

func seedUser(t *testing.T, db *sql.DB, id, email string) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO users (id, google_sub, email, display_name)
VALUES (?, ?, ?, ?);
`, id, id+"-sub", email, "Test User"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func seedModel(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`
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
VALUES (?, 'openrouter', 'OpenRouter Free', 0, 0, 0, '["reasoning"]', 1, 1, 1);
`, id); err != nil {
		t.Fatalf("seed model: %v", err)
	}
}

func requestWithSessionUser(req *http.Request, user session.User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), sessionUserContextKey, user))
}

func requestWithConversationID(req *http.Request, conversationID string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", conversationID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
}

func decodeJSONBody(t *testing.T, resp *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, resp.Body.String())
	}
}

type sseEvent struct {
	Type string
	Data map[string]any
}

func decodeSSEEvents(t *testing.T, raw string) []sseEvent {
	t.Helper()

	events := make([]sseEvent, 0, 8)
	chunks := strings.Split(raw, "\n\n")
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		lines := strings.Split(chunk, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			data := make(map[string]any)
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				t.Fatalf("decode sse payload: %v (payload=%s)", err, payload)
			}
			typeValue, _ := data["type"].(string)
			events = append(events, sseEvent{
				Type: typeValue,
				Data: data,
			})
		}
	}
	return events
}

func assertContainsPhaseInOrder(t *testing.T, phases []string, phase string, after int) int {
	t.Helper()
	for index, item := range phases {
		if index <= after {
			continue
		}
		if item == phase {
			return index
		}
	}
	t.Fatalf("expected phase %q in %+v", phase, phases)
	return -1
}

type stubStreamer struct {
	tokens          []string
	reasoningTokens []string
	err             error
	catalog         []openrouter.Model
	catalogErr      error
	onRequest       func(openrouter.StreamRequest)
}

type stubGrounder struct {
	results        []brave.SearchResult
	err            error
	waitForContext bool
}

func (s stubGrounder) Search(ctx context.Context, _ string, _ int) ([]brave.SearchResult, error) {
	if s.waitForContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func (s stubStreamer) StreamChatCompletion(_ context.Context, req openrouter.StreamRequest, onStart func() error, onDelta func(string) error, onReasoning func(string) error) error {
	if s.onRequest != nil {
		s.onRequest(req)
	}
	if onStart != nil {
		if err := onStart(); err != nil {
			return err
		}
	}
	// Send reasoning tokens first (like the real API does)
	for _, reasoning := range s.reasoningTokens {
		if onReasoning != nil {
			if err := onReasoning(reasoning); err != nil {
				return err
			}
		}
	}
	for _, token := range s.tokens {
		if err := onDelta(token); err != nil {
			return err
		}
	}
	return s.err
}

func (s stubStreamer) ListModels(_ context.Context) ([]openrouter.Model, error) {
	if s.catalogErr != nil {
		return nil, s.catalogErr
	}
	return s.catalog, nil
}

type stubFileStore struct {
	objects      map[string][]byte
	deletedPaths []string
}

func (s *stubFileStore) Backend() string {
	return "gcs"
}

func (s *stubFileStore) PutObject(_ context.Context, objectPath, _ string, data []byte) error {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[objectPath] = append([]byte(nil), data...)
	return nil
}

func (s *stubFileStore) DeleteObject(_ context.Context, objectPath string) error {
	s.deletedPaths = append(s.deletedPaths, objectPath)
	delete(s.objects, objectPath)
	return nil
}

const testSchema = `
PRAGMA foreign_keys = ON;

CREATE TABLE users (
  id TEXT PRIMARY KEY,
  google_sub TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT,
  avatar_url TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE models (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  display_name TEXT NOT NULL,
  context_window INTEGER NOT NULL DEFAULT 0,
  prompt_price_microusd INTEGER NOT NULL DEFAULT 0,
  completion_price_microusd INTEGER NOT NULL DEFAULT 0,
  supported_parameters_json TEXT,
  supports_reasoning INTEGER NOT NULL DEFAULT 0,
  curated INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_model_preferences (
  user_id TEXT PRIMARY KEY,
  last_used_model_id TEXT,
  last_used_deep_research_model_id TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (last_used_model_id) REFERENCES models(id) ON DELETE SET NULL,
  FOREIGN KEY (last_used_deep_research_model_id) REFERENCES models(id) ON DELETE SET NULL
);

CREATE TABLE user_model_favorites (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

CREATE TABLE user_model_reasoning_presets (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('chat', 'deep_research')),
  effort TEXT NOT NULL CHECK (effort IN ('low', 'medium', 'high')),
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id, mode),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

CREATE TABLE conversations (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT 'New Chat',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
  content TEXT NOT NULL,
  reasoning_content TEXT,
  model_id TEXT,
  grounding_enabled INTEGER NOT NULL DEFAULT 1,
  deep_research_enabled INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE SET NULL
);

CREATE TABLE citations (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  url TEXT NOT NULL,
  title TEXT,
  snippet TEXT,
  source_provider TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE TABLE files (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  storage_backend TEXT NOT NULL CHECK (storage_backend IN ('local', 'gcs')),
  storage_path TEXT NOT NULL,
  extracted_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE message_files (
  message_id TEXT NOT NULL,
  file_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (message_id, file_id),
  FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
  FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);
`
