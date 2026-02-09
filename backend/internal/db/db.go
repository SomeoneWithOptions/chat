package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"chat/backend/internal/config"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

func Open(cfg config.Config) (*sql.DB, error) {
	dsn, err := buildDSN(cfg.TursoDatabaseURL, cfg.TursoAuthToken)
	if err != nil {
		return nil, err
	}

	database, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open libsql db: %w", err)
	}

	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return database, nil
}

func buildDSN(rawURL, authToken string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", fmt.Errorf("empty database url")
	}

	if strings.HasPrefix(rawURL, "file:") {
		return rawURL, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}

	if strings.HasPrefix(rawURL, "libsql://") {
		query := parsed.Query()
		if query.Get("authToken") == "" && strings.TrimSpace(authToken) != "" {
			query.Set("authToken", strings.TrimSpace(authToken))
			parsed.RawQuery = query.Encode()
		}
	}

	return parsed.String(), nil
}
