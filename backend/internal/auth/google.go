package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"chat/backend/internal/config"
	"google.golang.org/api/idtoken"
)

var ErrUnverifiedEmail = errors.New("google account email is not verified")

type GoogleIdentity struct {
	GoogleSubject string
	Email         string
	Name          string
	AvatarURL     string
}

type Verifier struct {
	cfg config.Config
}

func NewVerifier(cfg config.Config) Verifier {
	return Verifier{cfg: cfg}
}

func (v Verifier) Verify(ctx context.Context, idToken string) (GoogleIdentity, error) {
	if strings.TrimSpace(idToken) == "" {
		return GoogleIdentity{}, errors.New("id token is required")
	}

	if v.cfg.InsecureSkipGoogleVerify {
		return GoogleIdentity{}, errors.New("AUTH_INSECURE_SKIP_GOOGLE_VERIFY enabled: testing endpoint requires explicit test identity header")
	}

	payload, err := idtoken.Validate(ctx, idToken, v.cfg.GoogleClientID)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("validate id token: %w", err)
	}

	email, _ := payload.Claims["email"].(string)
	if strings.TrimSpace(email) == "" {
		return GoogleIdentity{}, errors.New("google token missing email claim")
	}

	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if !emailVerified {
		return GoogleIdentity{}, ErrUnverifiedEmail
	}

	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	return GoogleIdentity{
		GoogleSubject: payload.Subject,
		Email:         strings.ToLower(email),
		Name:          strings.TrimSpace(name),
		AvatarURL:     strings.TrimSpace(picture),
	}, nil
}
