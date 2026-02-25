package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort                = "8080"
	defaultSessionCookieName   = "chat_session"
	defaultSessionTTLHours     = 720
	defaultDefaultModel        = "openrouter/free"
	defaultChatReasoningEffort = "medium"
	defaultDeepReasoningEffort = "high"
	defaultOpenRouterBaseURL   = "https://openrouter.ai/api/v1"
	defaultBraveBaseURL        = "https://api.search.brave.com/res/v1"
	defaultFrontendOrigin      = "https://chat.sanetomore.com"
	defaultUploadDir           = "/tmp/chat-uploads"
	defaultGCSUploadPrefix     = "chat-uploads"
	defaultResearchTimeoutSecs = 150
	defaultChatResearchTimeout = 20
	defaultSourceFetchTimeout  = 12
	defaultSourceMaxBytes      = 1_500_000
	defaultChatMaxLoops        = 2
	defaultChatMaxSourcesRead  = 4
	defaultChatMaxSearchQ      = 4
	defaultDeepMaxLoops        = 6
	defaultDeepMaxSourcesRead  = 16
	defaultDeepMaxSearchQ      = 18
	defaultChatMaxCitations    = 8
	defaultDeepMaxCitations    = 12
)

type Config struct {
	Port                       string
	Environment                string
	FrontendOrigin             string
	AllowedOrigins             []string
	AuthRequired               bool
	ModelSyncBearerToken       string
	CookieSecure               bool
	SessionCookieName          string
	SessionTTL                 time.Duration
	AllowedGoogleEmails        map[string]struct{}
	GoogleClientID             string
	InsecureSkipGoogleVerify   bool
	TursoDatabaseURL           string
	TursoAuthToken             string
	OpenRouterAPIKey           string
	OpenRouterBaseURL          string
	OpenRouterDefaultModel     string
	DefaultChatReasoningEffort string
	DefaultDeepReasoningEffort string
	BraveAPIKey                string
	BraveBaseURL               string
	LocalUploadDir             string
	GCSUploadBucket            string
	GCSUploadPrefix            string
	DeepResearchTimeoutSeconds int
	AgenticResearchChatEnabled bool
	AgenticResearchDeepEnabled bool
	ChatResearchMaxLoops       int
	ChatResearchMaxSourcesRead int
	ChatResearchMaxSearchQ     int
	ChatResearchTimeoutSeconds int
	DeepResearchMaxLoops       int
	DeepResearchMaxSourcesRead int
	DeepResearchMaxSearchQ     int
	ResearchSourceTimeoutSecs  int
	ResearchSourceMaxBytes     int
	ResearchMaxCitationsChat   int
	ResearchMaxCitationsDeep   int
}

func (c Config) ListenAddress() string {
	return fmt.Sprintf(":%s", c.Port)
}

func Load() (Config, error) {
	cfg := Config{
		Port:                       envOrDefault("PORT", defaultPort),
		Environment:                envOrDefault("APP_ENV", "development"),
		FrontendOrigin:             envOrDefault("FRONTEND_ORIGIN", defaultFrontendOrigin),
		AuthRequired:               boolOrDefault("AUTH_REQUIRED", true),
		ModelSyncBearerToken:       strings.TrimSpace(os.Getenv("MODEL_SYNC_BEARER_TOKEN")),
		CookieSecure:               boolOrDefault("COOKIE_SECURE", false),
		SessionCookieName:          envOrDefault("SESSION_COOKIE_NAME", defaultSessionCookieName),
		GoogleClientID:             strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		InsecureSkipGoogleVerify:   boolOrDefault("AUTH_INSECURE_SKIP_GOOGLE_VERIFY", false),
		TursoDatabaseURL:           strings.TrimSpace(os.Getenv("TURSO_DATABASE_URL")),
		TursoAuthToken:             strings.TrimSpace(os.Getenv("TURSO_AUTH_TOKEN")),
		OpenRouterAPIKey:           strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterBaseURL:          envOrDefault("OPENROUTER_API_BASE_URL", defaultOpenRouterBaseURL),
		OpenRouterDefaultModel:     envOrDefault("OPENROUTER_FREE_TIER_DEFAULT_MODEL", defaultDefaultModel),
		DefaultChatReasoningEffort: strings.ToLower(envOrDefault("DEFAULT_CHAT_REASONING_EFFORT", defaultChatReasoningEffort)),
		DefaultDeepReasoningEffort: strings.ToLower(envOrDefault("DEFAULT_DEEP_RESEARCH_REASONING_EFFORT", defaultDeepReasoningEffort)),
		BraveAPIKey:                strings.TrimSpace(os.Getenv("BRAVE_API_KEY")),
		BraveBaseURL:               envOrDefault("BRAVE_API_BASE_URL", defaultBraveBaseURL),
		LocalUploadDir:             envOrDefault("LOCAL_UPLOAD_DIR", defaultUploadDir),
		GCSUploadBucket:            strings.TrimSpace(os.Getenv("GCS_UPLOAD_BUCKET")),
		GCSUploadPrefix:            envOrDefault("GCS_UPLOAD_PREFIX", defaultGCSUploadPrefix),
		DeepResearchTimeoutSeconds: intOrDefault("DEEP_RESEARCH_TIMEOUT_SECONDS", defaultResearchTimeoutSecs),
		AgenticResearchChatEnabled: boolOrDefault("AGENTIC_RESEARCH_CHAT_ENABLED", true),
		AgenticResearchDeepEnabled: boolOrDefault("AGENTIC_RESEARCH_DEEP_ENABLED", true),
		ChatResearchMaxLoops:       intOrDefault("CHAT_RESEARCH_MAX_LOOPS", defaultChatMaxLoops),
		ChatResearchMaxSourcesRead: intOrDefault("CHAT_RESEARCH_MAX_SOURCES_READ", defaultChatMaxSourcesRead),
		ChatResearchMaxSearchQ:     intOrDefault("CHAT_RESEARCH_MAX_SEARCH_QUERIES", defaultChatMaxSearchQ),
		ChatResearchTimeoutSeconds: intOrDefault("CHAT_RESEARCH_TIMEOUT_SECONDS", defaultChatResearchTimeout),
		DeepResearchMaxLoops:       intOrDefault("DEEP_RESEARCH_MAX_LOOPS", defaultDeepMaxLoops),
		DeepResearchMaxSourcesRead: intOrDefault("DEEP_RESEARCH_MAX_SOURCES_READ", defaultDeepMaxSourcesRead),
		DeepResearchMaxSearchQ:     intOrDefault("DEEP_RESEARCH_MAX_SEARCH_QUERIES", defaultDeepMaxSearchQ),
		ResearchSourceTimeoutSecs:  intOrDefault("RESEARCH_SOURCE_FETCH_TIMEOUT_SECONDS", defaultSourceFetchTimeout),
		ResearchSourceMaxBytes:     intOrDefault("RESEARCH_SOURCE_MAX_BYTES", defaultSourceMaxBytes),
		ResearchMaxCitationsChat:   intOrDefault("RESEARCH_MAX_CITATIONS_CHAT", defaultChatMaxCitations),
		ResearchMaxCitationsDeep:   intOrDefault("RESEARCH_MAX_CITATIONS_DEEP", defaultDeepMaxCitations),
	}

	if cfg.Environment == "production" {
		cfg.CookieSecure = true
	}

	sessionTTLHours := intOrDefault("SESSION_TTL_HOURS", defaultSessionTTLHours)
	cfg.SessionTTL = time.Duration(sessionTTLHours) * time.Hour
	if cfg.SessionTTL <= 0 {
		return Config{}, errors.New("SESSION_TTL_HOURS must be > 0")
	}

	emails := envOrDefault("ALLOWED_GOOGLE_EMAILS", "acastesol@gmail.com,obzen.black@gmail.com")
	cfg.AllowedGoogleEmails = parseEmailSet(emails)

	origins := parseList(envOrDefault("CORS_ALLOWED_ORIGINS", cfg.FrontendOrigin+",http://localhost:5173,http://localhost:4173"))
	if len(origins) == 0 {
		return Config{}, errors.New("CORS_ALLOWED_ORIGINS must include at least one origin")
	}
	cfg.AllowedOrigins = origins

	if cfg.TursoDatabaseURL == "" {
		return Config{}, errors.New("TURSO_DATABASE_URL is required")
	}
	if strings.HasPrefix(cfg.TursoDatabaseURL, "libsql://") && cfg.TursoAuthToken == "" {
		return Config{}, errors.New("TURSO_AUTH_TOKEN is required for libsql:// URLs")
	}
	if cfg.AuthRequired && !cfg.InsecureSkipGoogleVerify && cfg.GoogleClientID == "" {
		return Config{}, errors.New("GOOGLE_CLIENT_ID is required unless AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true")
	}
	if err := validateReasoningEffort(cfg.DefaultChatReasoningEffort); err != nil {
		return Config{}, fmt.Errorf("DEFAULT_CHAT_REASONING_EFFORT %w", err)
	}
	if err := validateReasoningEffort(cfg.DefaultDeepReasoningEffort); err != nil {
		return Config{}, fmt.Errorf("DEFAULT_DEEP_RESEARCH_REASONING_EFFORT %w", err)
	}

	cfg.DeepResearchTimeoutSeconds = ensurePositiveInt(cfg.DeepResearchTimeoutSeconds, defaultResearchTimeoutSecs)
	cfg.ChatResearchMaxLoops = ensurePositiveInt(cfg.ChatResearchMaxLoops, defaultChatMaxLoops)
	cfg.ChatResearchMaxSourcesRead = ensurePositiveInt(cfg.ChatResearchMaxSourcesRead, defaultChatMaxSourcesRead)
	cfg.ChatResearchMaxSearchQ = ensurePositiveInt(cfg.ChatResearchMaxSearchQ, defaultChatMaxSearchQ)
	cfg.ChatResearchTimeoutSeconds = ensurePositiveInt(cfg.ChatResearchTimeoutSeconds, defaultChatResearchTimeout)
	cfg.DeepResearchMaxLoops = ensurePositiveInt(cfg.DeepResearchMaxLoops, defaultDeepMaxLoops)
	cfg.DeepResearchMaxSourcesRead = ensurePositiveInt(cfg.DeepResearchMaxSourcesRead, defaultDeepMaxSourcesRead)
	cfg.DeepResearchMaxSearchQ = ensurePositiveInt(cfg.DeepResearchMaxSearchQ, defaultDeepMaxSearchQ)
	cfg.ResearchSourceTimeoutSecs = ensurePositiveInt(cfg.ResearchSourceTimeoutSecs, defaultSourceFetchTimeout)
	cfg.ResearchSourceMaxBytes = ensurePositiveInt(cfg.ResearchSourceMaxBytes, defaultSourceMaxBytes)
	cfg.ResearchMaxCitationsChat = ensurePositiveInt(cfg.ResearchMaxCitationsChat, defaultChatMaxCitations)
	cfg.ResearchMaxCitationsDeep = ensurePositiveInt(cfg.ResearchMaxCitationsDeep, defaultDeepMaxCitations)

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func boolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func intOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseList(raw string) []string {
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseEmailSet(raw string) map[string]struct{} {
	emails := parseList(raw)
	out := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		out[strings.ToLower(email)] = struct{}{}
	}
	return out
}

func validateReasoningEffort(effort string) error {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("must be one of: low, medium, high")
	}
}

func ensurePositiveInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
