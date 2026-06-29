package worker

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/ingest"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourceScanIDs interface {
	NewDocumentID() domain.DocumentID
	NewJobID() domain.JobID
}

type SourceScanWorker struct {
	repos     store.RepositorySet
	clock     Clock
	documents app.DocumentService
}

type SourceScanResult struct {
	JobID         domain.JobID
	SourceID      domain.DataSourceID
	ImportedCount int
	SkippedCount  int
	FailedCount   int
}

func NewSourceScanWorker(repos store.RepositorySet, ids SourceScanIDs, clock Clock) SourceScanWorker {
	return SourceScanWorker{
		repos:     repos,
		clock:     clock,
		documents: app.NewDocumentService(repos, ids, clock),
	}
}

func (w SourceScanWorker) WithObjectStore(objects providers.ObjectStore) SourceScanWorker {
	w.documents = w.documents.WithObjectStore(objects)
	return w
}

func (w SourceScanWorker) ProcessNext(ctx context.Context) (SourceScanResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now(), domain.JobTypeSourceScan)
	if err != nil {
		return SourceScanResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if errors.Is(err, ErrSourceArchived) {
			if transitionErr := job.Transition(domain.JobStateCanceled, w.clock.Now()); transitionErr != nil {
				return SourceScanResult{}, transitionErr
			}
			if saveErr := w.repos.SaveJob(ctx, job); saveErr != nil {
				return SourceScanResult{}, saveErr
			}
			return result, nil
		}
		if transitionErr := job.Fail(err, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return SourceScanResult{}, err
	}

	if err := job.Transition(domain.JobStateSucceeded, w.clock.Now()); err != nil {
		return SourceScanResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return SourceScanResult{}, err
	}
	return result, nil
}

var ErrSourceArchived = errors.New("source was archived before scan")

func (w SourceScanWorker) processClaimedJob(ctx context.Context, job domain.Job) (SourceScanResult, error) {
	if job.Type != domain.JobTypeSourceScan || job.ResourceType != "data_source" || job.ResourceID == "" {
		return SourceScanResult{}, fmt.Errorf("unsupported source scan job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	sourceID := domain.DataSourceID(job.ResourceID)
	source, err := w.repos.GetDataSource(ctx, job.TenantID, sourceID)
	if err != nil {
		return SourceScanResult{}, err
	}
	result := SourceScanResult{
		JobID:    job.ID,
		SourceID: source.ID,
	}

	if source.Status == domain.DataSourceStatusArchived {
		return result, ErrSourceArchived
	}
	if source.Status != domain.DataSourceStatusScanning {
		if err := source.Transition(domain.DataSourceStatusScanning, w.clock.Now()); err != nil {
			return SourceScanResult{}, err
		}
		if err := w.repos.SaveDataSource(ctx, source); err != nil {
			return SourceScanResult{}, err
		}
	}

	result, scanErr := w.scanSource(ctx, source, result)
	if scanErr != nil {
		_ = w.markSourceFailed(ctx, source)
		return result, scanErr
	}

	latestSource, err := w.repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		return SourceScanResult{}, err
	}
	if latestSource.Status != domain.DataSourceStatusArchived {
		if latestSource.Status != domain.DataSourceStatusScanning {
			if err := latestSource.Transition(domain.DataSourceStatusScanning, w.clock.Now()); err != nil {
				return SourceScanResult{}, err
			}
		}
		if err := latestSource.Transition(domain.DataSourceStatusActive, w.clock.Now()); err != nil {
			return SourceScanResult{}, err
		}
		if err := w.repos.SaveDataSource(ctx, latestSource); err != nil {
			return SourceScanResult{}, err
		}
	}
	return result, nil
}

func (w SourceScanWorker) scanSource(ctx context.Context, source domain.DataSource, result SourceScanResult) (SourceScanResult, error) {
	root := filepath.Clean(source.RootPath)
	info, err := os.Stat(root)
	if err != nil {
		return result, fmt.Errorf("source scan cannot access %q: %w", source.RootPath, err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("source scan root %q is not a directory", source.RootPath)
	}

	var firstFailure error
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			result.FailedCount++
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", path, walkErr)
			}
			return nil
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			result.SkippedCount++
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		relativeName, err := filepath.Rel(root, path)
		if err != nil {
			relativeName = filepath.Base(path)
		}
		relativeName = filepath.ToSlash(relativeName)
		contentType := contentTypeForPath(path)
		if err := ingest.ValidateDocumentType(relativeName, contentType); err != nil {
			result.SkippedCount++
			return nil
		}

		fileInfo, err := entry.Info()
		if err != nil {
			result.FailedCount++
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, err)
			}
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			result.FailedCount++
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, err)
			}
			return nil
		}
		_, uploadErr := w.documents.UploadDocument(ctx, app.UploadDocumentInput{
			TenantID:    source.TenantID,
			OwnerID:     source.OwnerID,
			Name:        relativeName,
			ContentType: contentType,
			SizeBytes:   fileInfo.Size(),
			Body:        file,
		})
		closeErr := file.Close()
		if uploadErr != nil {
			result.FailedCount++
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, uploadErr)
			}
			return nil
		}
		if closeErr != nil {
			result.FailedCount++
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, closeErr)
			}
			return nil
		}
		result.ImportedCount++
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.FailedCount > 0 {
		return result, fmt.Errorf("source scan imported %d files, skipped %d files, failed %d files; first failure: %w", result.ImportedCount, result.SkippedCount, result.FailedCount, firstFailure)
	}
	return result, nil
}

func (w SourceScanWorker) markSourceFailed(ctx context.Context, source domain.DataSource) error {
	latest, err := w.repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		return err
	}
	if latest.Status == domain.DataSourceStatusArchived || latest.Status == domain.DataSourceStatusFailed {
		return nil
	}
	if err := latest.Transition(domain.DataSourceStatusFailed, w.clock.Now()); err != nil {
		return err
	}
	return w.repos.SaveDataSource(ctx, latest)
}

func contentTypeForPath(path string) string {
	contentType := strings.TrimSpace(mime.TypeByExtension(filepath.Ext(path)))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
