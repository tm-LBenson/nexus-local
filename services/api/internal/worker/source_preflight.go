package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourcePreflightWorker struct {
	repos store.RepositorySet
	clock Clock
}

type SourcePreflightResult struct {
	JobID    domain.JobID
	SourceID domain.DataSourceID
	Path     string
}

func NewSourcePreflightWorker(repos store.RepositorySet, clock Clock) SourcePreflightWorker {
	return SourcePreflightWorker{
		repos: repos,
		clock: clock,
	}
}

func (w SourcePreflightWorker) ProcessNext(ctx context.Context) (SourcePreflightResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now(), domain.JobTypeSourcePreflight)
	if err != nil {
		return SourcePreflightResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if errors.Is(err, ErrSourceArchived) {
			if transitionErr := job.Transition(domain.JobStateCanceled, w.clock.Now()); transitionErr != nil {
				return SourcePreflightResult{}, transitionErr
			}
			if saveErr := w.repos.SaveJob(ctx, job); saveErr != nil {
				return SourcePreflightResult{}, saveErr
			}
			return result, nil
		}
		if transitionErr := job.Fail(err, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return SourcePreflightResult{}, err
	}

	if err := job.Transition(domain.JobStateSucceeded, w.clock.Now()); err != nil {
		return SourcePreflightResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return SourcePreflightResult{}, err
	}
	return result, nil
}

func (w SourcePreflightWorker) processClaimedJob(ctx context.Context, job domain.Job) (SourcePreflightResult, error) {
	if job.Type != domain.JobTypeSourcePreflight || job.ResourceType != "data_source" || job.ResourceID == "" {
		return SourcePreflightResult{}, fmt.Errorf("unsupported source preflight job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	sourceID := domain.DataSourceID(job.ResourceID)
	source, err := w.repos.GetDataSource(ctx, job.TenantID, sourceID)
	if err != nil {
		return SourcePreflightResult{}, err
	}
	result := SourcePreflightResult{
		JobID:    job.ID,
		SourceID: source.ID,
		Path:     source.RootPath,
	}

	if source.Status == domain.DataSourceStatusArchived {
		return result, ErrSourceArchived
	}

	if err := preflightSourceRoot(source.RootPath); err != nil {
		return result, err
	}
	return result, nil
}

func preflightSourceRoot(root string) error {
	cleanRoot := filepath.Clean(root)
	info, err := os.Stat(cleanRoot)
	if err != nil {
		return fmt.Errorf("source preflight cannot access %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source preflight root %q is not a directory", root)
	}

	dir, err := os.Open(cleanRoot)
	if err != nil {
		return fmt.Errorf("source preflight cannot open %q: %w", root, err)
	}
	defer dir.Close()

	if _, err := dir.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("source preflight cannot read %q: %w", root, err)
	}
	return nil
}
