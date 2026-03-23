package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chat/backend/internal/brave"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/research"
	"chat/backend/internal/session"

	"github.com/google/uuid"
)

func TestExecuteAgentRunDispatchesCouncilWorkflowAndPersistsPublicStatus(t *testing.T) {
	requests := make([]openrouter.StreamRequest, 0, 4)
	handler, db := newTestHandler(t, councilTestStreamer{onRequest: func(req openrouter.StreamRequest) {
		requests = append(requests, req)
	}})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.CouncilTargetReadableSourcesPerModel = 15
	handler.cfg.CouncilSearchResultsPerQuery = 15
	handler.cfg.CouncilMaxSearchQueriesPerModel = 1
	handler.cfg.CouncilTimeoutSeconds = 60
	handler.cfg.BraveMonthlyQueryLimit = 200
	handler.cfg.BraveMonthlyQueryReserve = 0
	handler.grounding = stubGrounder{results: councilSearchResults(15)}
	handler.researchReader = stubResearchReader{responses: councilReadResults(15)}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "legacy-model")
	seedModel(t, db, "source-a")
	seedModel(t, db, "source-b")
	seedModel(t, db, "fusion-model")

	conversation, err := handler.insertConversation(context.Background(), user.ID, "Council")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user.ID, conversation.ID, "user", "Compare the options", "", true, false, "agent"); err != nil {
		t.Fatalf("insert user message: %v", err)
	}

	var userMessageID string
	if err := db.QueryRow(`SELECT id FROM messages WHERE conversation_id = ? AND role = 'user' LIMIT 1`, conversation.ID).Scan(&userMessageID); err != nil {
		t.Fatalf("load user message id: %v", err)
	}
	assistantMessageID, err := handler.insertMessageWithCitations(
		context.Background(),
		user.ID,
		conversation.ID,
		"assistant",
		"",
		"",
		"fusion-model",
		true,
		false,
		"agent",
		nil,
		newThinkingTraceCollector().Snapshot(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("insert assistant message: %v", err)
	}

	configJSON, _ := json.Marshal(CouncilRunConfig{
		SourceModels: []CouncilSourceSpec{
			{ModelID: "source-a"},
			{ModelID: "source-b"},
		},
		FusionModel: CouncilSourceSpec{ModelID: "fusion-model"},
		Grounding:   true,
	})

	runID := uuid.NewString()
	if err := handler.insertAgentRun(context.Background(), agentRunRecord{
		ID:                 runID,
		UserID:             user.ID,
		ConversationID:     conversation.ID,
		UserMessageID:      userMessageID,
		AssistantMessageID: assistantMessageID,
		ModelID:            "legacy-model",
		Status:             "queued",
		SearchBudget:       2,
		WorkflowType:       "council_fusion",
		FusionModelID:      "fusion-model",
		GroundingEnabled:   true,
		CouncilConfigJSON:  string(configJSON),
	}); err != nil {
		t.Fatalf("insert agent run: %v", err)
	}

	if err := handler.executeAgentRun(context.Background(), runID); err != nil {
		t.Fatalf("execute agent run: %v", err)
	}

	var (
		runStatus         string
		sourceResultsJSON string
		publicStatusJSON  string
		completedSources  int
		degradedSources   int
		failedSources     int
	)
	if err := db.QueryRow(`
SELECT status, COALESCE(source_results_json, ''), COALESCE(public_status_json, ''), completed_sources, degraded_sources, failed_sources
FROM agent_runs
WHERE id = ?
`, runID).Scan(&runStatus, &sourceResultsJSON, &publicStatusJSON, &completedSources, &degradedSources, &failedSources); err != nil {
		t.Fatalf("load persisted council run: %v", err)
	}
	if runStatus != "completed" {
		t.Fatalf("expected completed council run, got %q", runStatus)
	}
	if completedSources != 2 || degradedSources != 0 || failedSources != 0 {
		t.Fatalf("unexpected source counters: completed=%d degraded=%d failed=%d", completedSources, degradedSources, failedSources)
	}

	var sources []CouncilSourceResult
	if err := json.Unmarshal([]byte(sourceResultsJSON), &sources); err != nil {
		t.Fatalf("unmarshal source results: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 source results, got %d", len(sources))
	}
	for i, source := range sources {
		if source.Status != "complete" {
			t.Fatalf("expected source %d complete, got %q", i, source.Status)
		}
		if source.SearchQueries != 1 {
			t.Fatalf("expected source %d to use one Brave query, got %d", i, source.SearchQueries)
		}
		if source.ReadableSources < 15 {
			t.Fatalf("expected source %d to reach 15 readable sources, got %d", i, source.ReadableSources)
		}
	}

	if len(requests) != 4 {
		t.Fatalf("expected 4 OpenRouter calls (2 sources + analysis + result), got %d", len(requests))
	}
	if requests[0].Model != "source-a" || requests[1].Model != "source-b" || requests[2].Model != "fusion-model" || requests[3].Model != "fusion-model" {
		t.Fatalf("unexpected request model order: %+v", []string{requests[0].Model, requests[1].Model, requests[2].Model, requests[3].Model})
	}
	assertNoCouncilPlannerOrDebateRequests(t, requests)

	var publicStatus AgentRunStatusResponse
	if err := json.Unmarshal([]byte(publicStatusJSON), &publicStatus); err != nil {
		t.Fatalf("unmarshal public status: %v", err)
	}
	if publicStatus.Status != "completed" {
		t.Fatalf("expected public status completed, got %q", publicStatus.Status)
	}
	if len(publicStatus.SourceResults) != 2 {
		t.Fatalf("expected public payload source results, got %d", len(publicStatus.SourceResults))
	}
	if publicStatus.Result == nil || !strings.Contains(publicStatus.Result.Response, "Final council answer") {
		t.Fatalf("unexpected final result payload: %+v", publicStatus.Result)
	}

	var assistant messageResponse
	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversation.ID+"/messages", nil)
	listReq = requestWithSessionUser(listReq, user)
	listReq = requestWithConversationID(listReq, conversation.ID)
	listResp := httptest.NewRecorder()
	handler.ListConversationMessages(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list messages status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var payload struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, listResp, &payload)
	assistant = payload.Messages[len(payload.Messages)-1]
	if assistant.AgentRunID != runID {
		t.Fatalf("expected persisted agent run id %q, got %q", runID, assistant.AgentRunID)
	}
	if len(assistant.AgentSources) != 2 {
		t.Fatalf("expected persisted agent sources, got %d", len(assistant.AgentSources))
	}
	if assistant.AgentAnalysis == nil {
		t.Fatalf("expected persisted agent analysis")
	}
	if assistant.AgentResultModelID != "fusion-model" {
		t.Fatalf("unexpected result model id: %q", assistant.AgentResultModelID)
	}
	if assistant.AgentResultUsage == nil {
		t.Fatalf("expected persisted result usage")
	}
}

func TestExecuteAgentRunMarksGroundedSourceDegradedBelowReadableTarget(t *testing.T) {
	requests := make([]openrouter.StreamRequest, 0, 3)
	handler, db := newTestHandler(t, councilTestStreamer{onRequest: func(req openrouter.StreamRequest) {
		requests = append(requests, req)
	}})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.CouncilTargetReadableSourcesPerModel = 15
	handler.cfg.CouncilSearchResultsPerQuery = 15
	handler.cfg.BraveMonthlyQueryLimit = 200
	handler.cfg.BraveMonthlyQueryReserve = 0
	handler.grounding = stubGrounder{results: councilSearchResults(15)}
	handler.researchReader = stubResearchReader{responses: councilReadResults(7)}

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "source-a")
	seedModel(t, db, "fusion-model")

	conversation, err := handler.insertConversation(context.Background(), user.ID, "Council degraded")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if err := handler.insertMessage(context.Background(), user.ID, conversation.ID, "user", "What changed recently?", "", true, false, "agent"); err != nil {
		t.Fatalf("insert user message: %v", err)
	}

	var userMessageID string
	if err := db.QueryRow(`SELECT id FROM messages WHERE conversation_id = ? AND role = 'user' LIMIT 1`, conversation.ID).Scan(&userMessageID); err != nil {
		t.Fatalf("load user message id: %v", err)
	}
	assistantMessageID, err := handler.insertMessageWithCitations(
		context.Background(),
		user.ID,
		conversation.ID,
		"assistant",
		"",
		"",
		"fusion-model",
		true,
		false,
		"agent",
		nil,
		newThinkingTraceCollector().Snapshot(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("insert assistant message: %v", err)
	}

	configJSON, _ := json.Marshal(CouncilRunConfig{
		SourceModels: []CouncilSourceSpec{{ModelID: "source-a"}},
		FusionModel:  CouncilSourceSpec{ModelID: "fusion-model"},
		Grounding:    true,
	})

	runID := uuid.NewString()
	if err := handler.insertAgentRun(context.Background(), agentRunRecord{
		ID:                 runID,
		UserID:             user.ID,
		ConversationID:     conversation.ID,
		UserMessageID:      userMessageID,
		AssistantMessageID: assistantMessageID,
		ModelID:            "source-a",
		Status:             "queued",
		SearchBudget:       1,
		WorkflowType:       "council_fusion",
		FusionModelID:      "fusion-model",
		GroundingEnabled:   true,
		CouncilConfigJSON:  string(configJSON),
	}); err != nil {
		t.Fatalf("insert agent run: %v", err)
	}

	if err := handler.executeAgentRun(context.Background(), runID); err != nil {
		t.Fatalf("execute agent run: %v", err)
	}

	var sourceResultsJSON, finalResultJSON, publicStatusJSON string
	if err := db.QueryRow(`
SELECT COALESCE(source_results_json, ''), COALESCE(fusion_result_json, ''), COALESCE(public_status_json, '')
FROM agent_runs
WHERE id = ?
`, runID).Scan(&sourceResultsJSON, &finalResultJSON, &publicStatusJSON); err != nil {
		t.Fatalf("load council payloads: %v", err)
	}

	var sources []CouncilSourceResult
	if err := json.Unmarshal([]byte(sourceResultsJSON), &sources); err != nil {
		t.Fatalf("unmarshal source results: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source result, got %d", len(sources))
	}
	if sources[0].Status != "degraded" {
		t.Fatalf("expected degraded source status, got %q", sources[0].Status)
	}
	if sources[0].SearchQueries != 1 {
		t.Fatalf("expected one Brave query, got %d", sources[0].SearchQueries)
	}
	if sources[0].ReadableSources != 7 {
		t.Fatalf("expected 7 readable sources, got %d", sources[0].ReadableSources)
	}
	if len(sources[0].Warnings) == 0 {
		t.Fatalf("expected degraded source warning, got none")
	}

	var finalResult CouncilFinalResult
	if err := json.Unmarshal([]byte(finalResultJSON), &finalResult); err != nil {
		t.Fatalf("unmarshal final result: %v", err)
	}
	if !strings.Contains(finalResult.Response, "Warning: evidence quality was below target") {
		t.Fatalf("expected degraded evidence warning in final result, got %q", finalResult.Response)
	}

	var publicStatus AgentRunStatusResponse
	if err := json.Unmarshal([]byte(publicStatusJSON), &publicStatus); err != nil {
		t.Fatalf("unmarshal public status: %v", err)
	}
	if publicStatus.DegradedSources != 1 || publicStatus.CompletedSources != 0 || publicStatus.FailedSources != 0 {
		t.Fatalf("unexpected source counters: %+v", publicStatus)
	}
	assertNoCouncilPlannerOrDebateRequests(t, requests)
}

func TestChatMessagesCouncilRespectsUngroundedRequest(t *testing.T) {
	handler, db := newTestHandler(t, councilTestStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	handler.cfg.BraveMonthlyQueryLimit = 1
	handler.cfg.BraveMonthlyQueryReserve = 0
	handler.cfg.InternalWorkerBearerToken = "test-worker-token"

	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runID := strings.TrimPrefix(r.URL.Path, "/internal/agent-runs/")
		r = requestWithRouteParam(r, "id", runID)
		handler.InternalRunAgent(w, r)
	}))
	defer workerServer.Close()
	handler.cfg.InternalWorkerBaseURL = workerServer.URL

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "openrouter/free")
	seedModel(t, db, "source-a")
	seedModel(t, db, "fusion-model")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/messages",
		strings.NewReader(`{"message":"Run an ungrounded council pass","modelId":"openrouter/free","mode":"agent","grounding":false,"sourceModels":[{"modelId":"source-a"}],"fusionModel":{"modelId":"fusion-model"}}`),
	)
	req = requestWithSessionUser(req, user)
	resp := httptest.NewRecorder()

	handler.ChatMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	var (
		runID             string
		status            string
		councilConfigJSON string
		searchBudget      int
		searchesUsed      int
	)
	for {
		err := db.QueryRow(`
SELECT id, status, COALESCE(council_config_json, ''), search_budget, searches_used
FROM agent_runs
LIMIT 1
`).Scan(&runID, &status, &councilConfigJSON, &searchBudget, &searchesUsed)
		if err != nil {
			t.Fatalf("load council run: %v", err)
		}
		if status == "completed" || status == "failed" || time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if status != "completed" {
		t.Fatalf("expected completed council run, got %q", status)
	}
	var persistedConfig CouncilRunConfig
	if err := json.Unmarshal([]byte(councilConfigJSON), &persistedConfig); err != nil {
		t.Fatalf("unmarshal persisted council config: %v", err)
	}
	if persistedConfig.Grounding {
		t.Fatalf("expected council config grounding to remain disabled")
	}
	if searchBudget != 0 || searchesUsed != 0 {
		t.Fatalf("expected ungrounded council run to avoid Brave budget, got budget=%d searches=%d", searchBudget, searchesUsed)
	}

	var monthlyUsageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM brave_monthly_usage`).Scan(&monthlyUsageCount); err != nil {
		t.Fatalf("count brave usage rows: %v", err)
	}
	if monthlyUsageCount != 0 {
		t.Fatalf("expected no Brave usage rows for ungrounded council, got %d", monthlyUsageCount)
	}
}

func TestListConversationMessagesIncludesCouncilFields(t *testing.T) {
	handler, db := newTestHandler(t, stubStreamer{})
	t.Cleanup(func() { _ = db.Close() })

	user := session.User{ID: "user-1"}
	seedUser(t, db, user.ID, "user1@example.com")
	seedModel(t, db, "fusion-model")
	conversation, err := handler.insertConversation(context.Background(), user.ID, "Council reload")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	sourceResultsJSON, _ := json.Marshal([]CouncilSourceResult{{ModelID: "source-a", Status: "degraded", ReadableSources: 7}})
	analysisJSON, _ := json.Marshal(CouncilAnalysis{
		Agreement: []CouncilAnalysisItem{{Point: "Shared point", SourceModels: []string{"source-a"}}},
	})
	usageJSON, _ := json.Marshal(usageResponse{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20})
	messageID := uuid.NewString()
	if _, err := db.Exec(`
INSERT INTO messages (
  id, conversation_id, user_id, role, content, model_id, grounding_enabled, deep_research_enabled, response_mode,
  agent_sources_json, agent_analysis_json, agent_result_model_id, agent_result_usage_json, agent_run_id
)
VALUES (?, ?, ?, 'assistant', 'Final council answer', 'fusion-model', 1, 0, 'agent', ?, ?, 'fusion-model', ?, 'run-123')
`, messageID, conversation.ID, user.ID, string(sourceResultsJSON), string(analysisJSON), string(usageJSON)); err != nil {
		t.Fatalf("insert council message: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/conversations/"+conversation.ID+"/messages", nil)
	req = requestWithSessionUser(req, user)
	req = requestWithConversationID(req, conversation.ID)
	resp := httptest.NewRecorder()
	handler.ListConversationMessages(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, resp.Code, resp.Body.String())
	}

	var payload struct {
		Messages []messageResponse `json:"messages"`
	}
	decodeJSONBody(t, resp, &payload)
	if len(payload.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(payload.Messages))
	}
	message := payload.Messages[0]
	if len(message.AgentSources) != 1 || message.AgentSources[0].ModelID != "source-a" {
		t.Fatalf("unexpected agent sources payload: %+v", message.AgentSources)
	}
	if message.AgentAnalysis == nil || len(message.AgentAnalysis.Agreement) != 1 {
		t.Fatalf("unexpected agent analysis payload: %+v", message.AgentAnalysis)
	}
	if message.AgentResultModelID != "fusion-model" {
		t.Fatalf("unexpected result model id: %q", message.AgentResultModelID)
	}
	if message.AgentResultUsage == nil || message.AgentResultUsage.TotalTokens != 20 {
		t.Fatalf("unexpected result usage: %+v", message.AgentResultUsage)
	}
	if message.AgentRunID != "run-123" {
		t.Fatalf("unexpected agent run id: %q", message.AgentRunID)
	}
}

type councilTestStreamer struct {
	onRequest func(openrouter.StreamRequest)
}

func (s councilTestStreamer) StreamChatCompletion(_ context.Context, req openrouter.StreamRequest, onStart func() error, onDelta func(string) error, _ func(string) error, onUsage func(openrouter.Usage) error) error {
	if s.onRequest != nil {
		s.onRequest(req)
	}
	if onStart != nil {
		if err := onStart(); err != nil {
			return err
		}
	}
	prompt := req.Messages[len(req.Messages)-1].Content
	response := "Answer"
	switch {
	case len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "You are an analytical AI."):
		response = `{"agreement":[{"point":"Shared point","sourceModels":["source-a","source-b"]}],"keyDifferences":[],"partialCoverage":[],"uniqueInsights":[],"blindSpots":[]}`
	case strings.Contains(prompt, "Write the best possible final answer for the user by fusing"):
		response = "Final council answer [1]"
	default:
		response = fmt.Sprintf("Source answer from %s [1]", req.Model)
	}
	if onUsage != nil {
		if err := onUsage(openrouter.Usage{
			PromptTokens:     30,
			CompletionTokens: 12,
			TotalTokens:      42,
			ModelID:          req.Model,
			ProviderName:     "openrouter",
		}); err != nil {
			return err
		}
	}
	return onDelta(response)
}

func (councilTestStreamer) GetGeneration(context.Context, string) (openrouter.Generation, error) {
	return openrouter.Generation{}, fmt.Errorf("not implemented")
}

func assertNoCouncilPlannerOrDebateRequests(t *testing.T, requests []openrouter.StreamRequest) {
	t.Helper()
	for _, req := range requests {
		if len(req.Messages) == 0 {
			continue
		}
		system := strings.TrimSpace(req.Messages[0].Content)
		if strings.Contains(system, "You are a research planner.") {
			t.Fatalf("unexpected planner request during council source pass: %+v", req)
		}
		if system == "You are a structured JSON generator." {
			t.Fatalf("unexpected role-debate request during council source pass: %+v", req)
		}
	}
}

func councilSearchResults(count int) []brave.SearchResult {
	results := make([]brave.SearchResult, 0, count)
	for i := 0; i < count; i++ {
		results = append(results, brave.SearchResult{
			URL:     fmt.Sprintf("https://example.com/source-%02d", i+1),
			Title:   fmt.Sprintf("Source %02d", i+1),
			Snippet: fmt.Sprintf("Snippet %02d", i+1),
		})
	}
	return results
}

func councilReadResults(count int) map[string]research.ReadResult {
	results := make(map[string]research.ReadResult, count)
	for i := 0; i < count; i++ {
		rawURL := fmt.Sprintf("https://example.com/source-%02d", i+1)
		results[rawURL] = research.ReadResult{
			URL:         rawURL,
			FinalURL:    rawURL,
			Title:       fmt.Sprintf("Source %02d", i+1),
			ContentType: "text/html",
			Text:        fmt.Sprintf("Full readable source text %02d", i+1),
			Snippet:     fmt.Sprintf("Snippet %02d", i+1),
			FetchStatus: "ok",
			FetchedAt:   time.Now().UTC(),
		}
	}
	return results
}
