package main

import (
	"log"
	"net/http"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/httpapi"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
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

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(cfg, modelRouter),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
