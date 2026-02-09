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

	if cfg.SessionTTL.Hours() != 168 {
		t.Fatalf("expected default 168h session ttl, got %v", cfg.SessionTTL)
	}

	if _, ok := cfg.AllowedGoogleEmails["acastesol@gmail.com"]; !ok {
		t.Fatalf("default allowlist missing acastesol@gmail.com")
	}

	if cfg.OpenRouterDefaultModel != "openrouter/free" {
		t.Fatalf("unexpected default model: %s", cfg.OpenRouterDefaultModel)
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

func unsetIfSet(t *testing.T, key string) {
	t.Helper()
	if _, ok := os.LookupEnv(key); ok {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset env %s: %v", key, err)
		}
	}
}
