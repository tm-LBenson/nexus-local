package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/runtime"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repos, closeRepos, err := runtime.OpenRepositories(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer closeRepos()

	objectStore, err := runtime.OpenObjectStore(ctx, cfg)
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

	ingestionWorker := worker.NewDocumentIngestionWorker(repos, systemClock{}).
		WithPipeline(objectStore, embedder, vectorIndex)
	pollInterval := cfg.WorkerPollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	log.Printf("worker started with poll interval %s", pollInterval)
	for {
		select {
		case <-ctx.Done():
			log.Print("worker stopped")
			return
		default:
		}

		result, err := ingestionWorker.ProcessNext(ctx)
		if err == nil {
			log.Printf("processed document ingestion job=%s document=%s", result.JobID, result.DocumentID)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("worker error: %v", err)
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Print("worker stopped")
			return
		case <-timer.C:
		}
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}
