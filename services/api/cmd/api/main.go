package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/httpapi"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/runtime"
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

	repos, closeRepos, err := runtime.OpenRepositories(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer closeRepos()
	objectStore, err := runtime.OpenObjectStore(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	embedder, err := runtime.OpenEmbedder(cfg)
	if err != nil {
		log.Fatal(err)
	}
	vectorIndex, err := runtime.OpenVectorIndex(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ids := app.NewRandomIDs()
	clock := app.SystemClock{}
	modelGateway := runtime.NewRoutedModelGateway(modelRouter, cfg)
	documentService := app.NewDocumentService(repos, ids, clock).WithObjectStore(objectStore)
	searchService := app.NewSearchService(embedder, vectorIndex)
	conversationService := app.NewConversationService(repos, ids, clock, searchService, modelGateway)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(cfg, httpapi.Dependencies{
			ModelRouter:   modelRouter,
			Documents:     documentService,
			Search:        searchService,
			Conversations: conversationService,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
