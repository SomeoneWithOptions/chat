package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"

	"chat/backend/internal/auth"
	"chat/backend/internal/brave"
	"chat/backend/internal/config"
	"chat/backend/internal/openrouter"
	"chat/backend/internal/session"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(cfg config.Config, db *sql.DB) http.Handler {
	store := session.NewStore(db)
	verifier := auth.NewVerifier(cfg)
	openRouterClient := openrouter.NewClient(cfg, nil)

	var files fileObjectStore
	if strings.TrimSpace(cfg.GCSUploadBucket) != "" {
		gcsStore, err := newGCSObjectStore(context.Background(), cfg.GCSUploadBucket)
		if err != nil {
			log.Printf("attachments disabled: failed to initialize gcs storage: %v", err)
		} else {
			files = gcsStore
		}
	}

	h := NewHandlerWithFileStore(cfg, db, store, verifier, openRouterClient, files)
	h.grounding = brave.NewClient(cfg, nil)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Test-Email", "X-Test-Google-Sub"},
		ExposedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Cloud Run reserves exact `/healthz`, so expose a platform-safe health path.
	r.Get("/health", h.Healthz)
	r.Get("/healthz", h.Healthz)
	r.Get("/healthz/", h.Healthz)

	r.Route("/v1", func(v1 chi.Router) {
		v1.Route("/auth", func(authR chi.Router) {
			authR.Post("/google", h.AuthGoogle)
			authR.With(h.RequireSession).Get("/me", h.AuthMe)
			authR.With(h.RequireSession).Post("/logout", h.AuthLogout)
		})

		v1.Group(func(p chi.Router) {
			p.Use(h.RequireSession)
			p.Get("/models", h.ListModels)
			p.Post("/models/sync", h.SyncModels)
			p.Put("/models/preferences", h.UpdateModelPreferences)
			p.Put("/models/favorites", h.UpdateModelFavorite)
			p.Post("/files", h.UploadFile)
			p.Post("/conversations", h.CreateConversation)
			p.Get("/conversations", h.ListConversations)
			p.Delete("/conversations", h.DeleteAllConversations)
			p.Delete("/conversations/{id}", h.DeleteConversation)
			p.Get("/conversations/{id}/messages", h.ListConversationMessages)
			p.Post("/chat/messages", h.ChatMessages)
		})
	})

	return r
}
