package httpapi

import (
	"database/sql"
	"net/http"

	"chat/backend/internal/auth"
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
	h := NewHandler(cfg, db, store, verifier, openRouterClient)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Test-Email", "X-Test-Google-Sub"},
		ExposedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", h.Healthz)

	r.Route("/v1", func(v1 chi.Router) {
		v1.Route("/auth", func(authR chi.Router) {
			authR.Post("/google", h.AuthGoogle)
			authR.With(h.RequireSession).Get("/me", h.AuthMe)
			authR.With(h.RequireSession).Post("/logout", h.AuthLogout)
		})

		v1.Group(func(p chi.Router) {
			p.Use(h.RequireSession)
			p.Get("/models", h.ListModels)
			p.Post("/chat/messages", h.ChatMessages)
		})
	})

	return r
}
