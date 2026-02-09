package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("session not found")

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	GoogleSub string `json:"googleSub"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) UpsertUser(ctx context.Context, googleSub, email, name, avatar string) (User, error) {
	id := uuid.NewString()
	query := `
INSERT INTO users (id, google_sub, email, display_name, avatar_url)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(google_sub) DO UPDATE SET
  email = excluded.email,
  display_name = excluded.display_name,
  avatar_url = excluded.avatar_url,
  updated_at = CURRENT_TIMESTAMP
RETURNING id, google_sub, email, COALESCE(display_name, ''), COALESCE(avatar_url, ''), created_at, updated_at;
`

	var out User
	if err := s.db.QueryRowContext(ctx, query, id, googleSub, strings.ToLower(email), strings.TrimSpace(name), strings.TrimSpace(avatar)).Scan(
		&out.ID,
		&out.GoogleSub,
		&out.Email,
		&out.Name,
		&out.AvatarURL,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}

	return out, nil
}

func (s Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, time.Time, error) {
	rawToken, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}

	expiresAt := time.Now().Add(ttl).UTC()
	query := `INSERT INTO sessions (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?);`

	if _, err := s.db.ExecContext(ctx, query, uuid.NewString(), userID, hashToken(rawToken), expiresAt.Format(time.RFC3339)); err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}

	return rawToken, expiresAt, nil
}

func (s Store) ResolveSession(ctx context.Context, rawToken string) (User, error) {
	query := `
SELECT u.id, u.google_sub, u.email, COALESCE(u.display_name, ''), COALESCE(u.avatar_url, ''), u.created_at, u.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > CURRENT_TIMESTAMP
LIMIT 1;
`

	var out User
	err := s.db.QueryRowContext(ctx, query, hashToken(rawToken)).Scan(
		&out.ID,
		&out.GoogleSub,
		&out.Email,
		&out.Name,
		&out.AvatarURL,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("resolve session: %w", err)
	}
	return out, nil
}

func (s Store) DeleteSession(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?;`, hashToken(rawToken))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
