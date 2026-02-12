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
