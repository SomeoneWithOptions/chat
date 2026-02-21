package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")

	unsetIfSet(t, "SESSION_TTL_HOURS")
	unsetIfSet(t, "ALLOWED_GOOGLE_EMAILS")
	unsetIfSet(t, "CORS_ALLOWED_ORIGINS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.SessionTTL.Hours() != 720 {
		t.Fatalf("expected default 720h session ttl, got %v", cfg.SessionTTL)
	}

	if _, ok := cfg.AllowedGoogleEmails["acastesol@gmail.com"]; !ok {
		t.Fatalf("default allowlist missing acastesol@gmail.com")
	}

	if cfg.OpenRouterDefaultModel != "openrouter/free" {
		t.Fatalf("unexpected default model: %s", cfg.OpenRouterDefaultModel)
	}

	if cfg.OpenRouterBaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("unexpected openrouter base url: %s", cfg.OpenRouterBaseURL)
	}
	if cfg.DefaultChatReasoningEffort != "medium" {
		t.Fatalf("unexpected default chat reasoning effort: %s", cfg.DefaultChatReasoningEffort)
	}
	if cfg.DefaultDeepReasoningEffort != "high" {
		t.Fatalf("unexpected default deep research reasoning effort: %s", cfg.DefaultDeepReasoningEffort)
	}

	if cfg.BraveBaseURL != "https://api.search.brave.com/res/v1" {
		t.Fatalf("unexpected brave base url: %s", cfg.BraveBaseURL)
	}

	if cfg.GCSUploadPrefix != "chat-uploads" {
		t.Fatalf("unexpected gcs upload prefix: %s", cfg.GCSUploadPrefix)
	}

	if cfg.DeepResearchTimeoutSeconds != 150 {
		t.Fatalf("unexpected deep research timeout default: %d", cfg.DeepResearchTimeoutSeconds)
	}
	if !cfg.AgenticResearchChatEnabled || !cfg.AgenticResearchDeepEnabled {
		t.Fatalf("expected agentic research flags enabled by default")
	}
	if cfg.ChatResearchMaxLoops != 2 {
		t.Fatalf("unexpected chat max loops default: %d", cfg.ChatResearchMaxLoops)
	}
	if cfg.ChatResearchMaxSourcesRead != 4 {
		t.Fatalf("unexpected chat max sources read default: %d", cfg.ChatResearchMaxSourcesRead)
	}
	if cfg.ChatResearchMaxSearchQ != 4 {
		t.Fatalf("unexpected chat max search queries default: %d", cfg.ChatResearchMaxSearchQ)
	}
	if cfg.ChatResearchTimeoutSeconds != 20 {
		t.Fatalf("unexpected chat research timeout default: %d", cfg.ChatResearchTimeoutSeconds)
	}
	if cfg.DeepResearchMaxLoops != 6 {
		t.Fatalf("unexpected deep max loops default: %d", cfg.DeepResearchMaxLoops)
	}
	if cfg.DeepResearchMaxSourcesRead != 16 {
		t.Fatalf("unexpected deep max sources read default: %d", cfg.DeepResearchMaxSourcesRead)
	}
	if cfg.DeepResearchMaxSearchQ != 18 {
		t.Fatalf("unexpected deep max search queries default: %d", cfg.DeepResearchMaxSearchQ)
	}
	if cfg.ResearchSourceTimeoutSecs != 8 {
		t.Fatalf("unexpected source timeout default: %d", cfg.ResearchSourceTimeoutSecs)
	}
	if cfg.ResearchSourceMaxBytes != 1_500_000 {
		t.Fatalf("unexpected source max bytes default: %d", cfg.ResearchSourceMaxBytes)
	}
	if cfg.ResearchMaxCitationsChat != 8 {
		t.Fatalf("unexpected chat max citations default: %d", cfg.ResearchMaxCitationsChat)
	}
	if cfg.ResearchMaxCitationsDeep != 12 {
		t.Fatalf("unexpected deep max citations default: %d", cfg.ResearchMaxCitationsDeep)
	}

	if cfg.ModelSyncBearerToken != "" {
		t.Fatalf("expected empty model sync bearer token by default")
	}
}

func TestLoadRequiresGoogleClientIDWhenVerificationEnabled(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when GOOGLE_CLIENT_ID is missing")
	}
}

func TestLoadAllowsMissingGoogleClientIDInInsecureMode(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "true")

	if _, err := Load(); err != nil {
		t.Fatalf("expected insecure mode to load without GOOGLE_CLIENT_ID: %v", err)
	}
}

func TestLoadAllowsMissingGoogleClientIDWhenAuthDisabled(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("AUTH_REQUIRED", "false")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")

	if _, err := Load(); err != nil {
		t.Fatalf("expected auth-disabled mode to load without GOOGLE_CLIENT_ID: %v", err)
	}
}

func TestLoadReadsModelSyncBearerToken(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")
	t.Setenv("MODEL_SYNC_BEARER_TOKEN", "sync-token-123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ModelSyncBearerToken != "sync-token-123" {
		t.Fatalf("unexpected model sync bearer token: %q", cfg.ModelSyncBearerToken)
	}
}

func TestLoadRejectsInvalidReasoningEffort(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")
	t.Setenv("DEFAULT_CHAT_REASONING_EFFORT", "max")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DEFAULT_CHAT_REASONING_EFFORT")
	}
}

func TestLoadClampsInvalidResearchBudgetsToDefaults(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "file:local.db")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", "false")
	t.Setenv("CHAT_RESEARCH_MAX_LOOPS", "-1")
	t.Setenv("CHAT_RESEARCH_MAX_SOURCES_READ", "0")
	t.Setenv("CHAT_RESEARCH_MAX_SEARCH_QUERIES", "-3")
	t.Setenv("CHAT_RESEARCH_TIMEOUT_SECONDS", "0")
	t.Setenv("DEEP_RESEARCH_MAX_LOOPS", "-1")
	t.Setenv("DEEP_RESEARCH_MAX_SOURCES_READ", "-2")
	t.Setenv("DEEP_RESEARCH_MAX_SEARCH_QUERIES", "0")
	t.Setenv("RESEARCH_SOURCE_FETCH_TIMEOUT_SECONDS", "-1")
	t.Setenv("RESEARCH_SOURCE_MAX_BYTES", "0")
	t.Setenv("RESEARCH_MAX_CITATIONS_CHAT", "0")
	t.Setenv("RESEARCH_MAX_CITATIONS_DEEP", "-4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ChatResearchMaxLoops != 2 ||
		cfg.ChatResearchMaxSourcesRead != 4 ||
		cfg.ChatResearchMaxSearchQ != 4 ||
		cfg.ChatResearchTimeoutSeconds != 20 ||
		cfg.DeepResearchMaxLoops != 6 ||
		cfg.DeepResearchMaxSourcesRead != 16 ||
		cfg.DeepResearchMaxSearchQ != 18 ||
		cfg.ResearchSourceTimeoutSecs != 8 ||
		cfg.ResearchSourceMaxBytes != 1_500_000 ||
		cfg.ResearchMaxCitationsChat != 8 ||
		cfg.ResearchMaxCitationsDeep != 12 {
		t.Fatalf("expected invalid budgets to clamp to defaults, got %+v", cfg)
	}
}

func unsetIfSet(t *testing.T, key string) {
	t.Helper()
	if _, ok := os.LookupEnv(key); ok {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset env %s: %v", key, err)
		}
	}
}
