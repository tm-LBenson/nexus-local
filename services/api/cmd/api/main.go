package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/httpapi"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/postgres"
)

func main() {
	cfg := config.Load()

	modelRouter, err := providers.NewModelRouter([]providers.TargetConfig{
		{
			Name:     "general",
			Provider: "openai-compatible",
			BaseURL:  cfg.ModelGatewayBaseURL,
			Model:    cfg.GeneralModelID,
		},
	}, cfg.DefaultModelTarget)
	if err != nil {
		log.Fatalf("model router: %v", err)
	}

	repos, closeRepos := openRepositories(cfg)
	defer closeRepos()
	documentService := app.NewDocumentService(repos, app.NewRandomIDs(), app.SystemClock{})

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(cfg, httpapi.Dependencies{
			ModelRouter: modelRouter,
			Documents:   documentService,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func openRepositories(cfg config.Config) (store.RepositorySet, func()) {
	switch strings.ToLower(strings.TrimSpace(cfg.PersistenceBackend)) {
	case "", "memory":
		return memory.New(), func() {}
	case "postgres":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		repos, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("postgres store: %v", err)
		}
		if cfg.RunMigrations {
			if err := repos.Migrate(ctx); err != nil {
				repos.Close()
				log.Fatalf("postgres migrations: %v", err)
			}
		}
		return repos, repos.Close
	default:
		log.Fatalf("unsupported persistence backend %q", cfg.PersistenceBackend)
		return nil, func() {}
	}
}
