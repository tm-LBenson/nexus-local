package main

import (
	"log"
	"net/http"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/httpapi"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
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

	repos := memory.New()
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
