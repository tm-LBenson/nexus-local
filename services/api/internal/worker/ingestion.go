package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type Clock interface {
	Now() time.Time
}

type DocumentIngestionWorker struct {
	repos store.RepositorySet
	clock Clock
}

type ProcessResult struct {
	JobID      domain.JobID
	DocumentID domain.DocumentID
}

func NewDocumentIngestionWorker(repos store.RepositorySet, clock Clock) DocumentIngestionWorker {
	return DocumentIngestionWorker{
		repos: repos,
		clock: clock,
	}
}

func (w DocumentIngestionWorker) ProcessNext(ctx context.Context) (ProcessResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now())
	if err != nil {
		return ProcessResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if transitionErr := job.Transition(domain.JobStateFailed, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return ProcessResult{}, err
	}

	if err := job.Transition(domain.JobStateSucceeded, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return ProcessResult{}, err
	}

	return result, nil
}

func (w DocumentIngestionWorker) processClaimedJob(ctx context.Context, job domain.Job) (ProcessResult, error) {
	if job.Type != domain.JobTypeDocumentIngestion || job.ResourceType != "document" || job.ResourceID == "" {
		return ProcessResult{}, fmt.Errorf("unsupported ingestion job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	documentID := domain.DocumentID(job.ResourceID)
	document, err := w.repos.GetDocument(ctx, job.TenantID, documentID)
	if err != nil {
		return ProcessResult{}, err
	}

	if err := document.Transition(domain.DocumentStatusProcessing, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveDocument(ctx, document); err != nil {
		return ProcessResult{}, err
	}

	// Placeholder for extraction, chunking, embedding, and vector upsert.
	if err := document.Transition(domain.DocumentStatusReady, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveDocument(ctx, document); err != nil {
		return ProcessResult{}, err
	}

	return ProcessResult{
		JobID:      job.ID,
		DocumentID: document.ID,
	}, nil
}
