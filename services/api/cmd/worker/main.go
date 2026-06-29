package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
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

	ids := app.NewRandomIDs()
	sourceScheduler := worker.NewSourceScheduler(repos, ids, systemClock{})
	sourcePreflightWorker := worker.NewSourcePreflightWorker(repos, systemClock{})
	sourceScanWorker := worker.NewSourceScanWorker(repos, ids, systemClock{}).
		WithObjectStore(objectStore).
		WithVectorIndex(vectorIndex)
	ingestionWorker := worker.NewDocumentIngestionWorker(repos, systemClock{}).
		WithPipeline(objectStore, embedder, vectorIndex)
	pollInterval := cfg.WorkerPollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	documentConcurrency := cfg.WorkerDocumentConcurrency
	if documentConcurrency < 1 {
		documentConcurrency = 1
	}

	log.Printf("worker started with poll interval %s and document concurrency %d", pollInterval, documentConcurrency)
	for {
		select {
		case <-ctx.Done():
			log.Print("worker stopped")
			return
		default:
		}

		scheduleResult, err := sourceScheduler.QueueDueScans(ctx, 25)
		if err != nil {
			log.Printf("source scheduler error: %v", err)
		} else if scheduleResult.QueuedCount > 0 || scheduleResult.SkippedActiveCount > 0 {
			log.Printf("source scheduler queued=%d skipped_active=%d", scheduleResult.QueuedCount, scheduleResult.SkippedActiveCount)
		}

		preflightResult, err := sourcePreflightWorker.ProcessNext(ctx)
		if err == nil {
			log.Printf(
				"processed source preflight job=%s source=%s path=%s",
				preflightResult.JobID,
				preflightResult.SourceID,
				preflightResult.Path,
			)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("source preflight worker error: %v", err)
			continue
		}

		scanResult, err := sourceScanWorker.ProcessNext(ctx)
		if err == nil {
			log.Printf(
				"processed source scan job=%s source=%s imported=%d skipped=%d failed=%d",
				scanResult.JobID,
				scanResult.SourceID,
				scanResult.ImportedCount,
				scanResult.SkippedCount,
				scanResult.FailedCount,
			)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("source scan worker error: %v", err)
			continue
		}

		batchResult, err := worker.ProcessDocumentIngestionBatch(ctx, ingestionWorker, documentConcurrency)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			log.Printf("worker error: %v", err)
		}
		if batchResult.Processed > 0 {
			log.Printf(
				"processed document ingestion batch processed=%d empty=%d failed=%d",
				batchResult.Processed,
				batchResult.Empty,
				batchResult.Failed,
			)
			continue
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
