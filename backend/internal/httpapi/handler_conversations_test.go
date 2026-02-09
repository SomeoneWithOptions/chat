package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chat/backend/internal/auth"
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
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	cfg := config.Config{
		AuthRequired:           true,
		SessionCookieName:      "chat_session",
		OpenRouterDefaultModel: "openrouter/free",
	}

	handler := NewHandler(cfg, db, session.NewStore(db), auth.NewVerifier(cfg), streamer)
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
INSERT INTO models (id, provider, display_name, context_window, prompt_price_microusd, completion_price_microusd, curated, is_active)
VALUES (?, 'openrouter', 'OpenRouter Free', 0, 0, 0, 1, 1);
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

type stubStreamer struct {
	tokens []string
	err    error
}

func (s stubStreamer) StreamChatCompletion(_ context.Context, _ openrouter.StreamRequest, onStart func() error, onDelta func(string) error) error {
	if err := onStart(); err != nil {
		return err
	}
	for _, token := range s.tokens {
		if err := onDelta(token); err != nil {
			return err
		}
	}
	return s.err
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
  curated INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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
  model_id TEXT,
  grounding_enabled INTEGER NOT NULL DEFAULT 1,
  deep_research_enabled INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE SET NULL
);
`
