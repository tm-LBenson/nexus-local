package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourcePreflightWorkerSucceedsForReadableDirectory(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	job := newSourcePreflightJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePreflightWorker(repos, fixedClock{})
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process preflight: %v", err)
	}
	if result.JobID != job.ID || result.SourceID != source.ID || result.Path != root {
		t.Fatalf("result = %#v", result)
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateSucceeded || updatedJob.ErrorMessage != "" {
		t.Fatalf("job = %#v, want succeeded", updatedJob)
	}
	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.Status != domain.DataSourceStatusActive {
		t.Fatalf("source status = %q, want active", updatedSource.Status)
	}
}

func TestSourcePreflightWorkerFailsMissingRoot(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, filepath.Join(t.TempDir(), "missing"), domain.DataSourceStatusActive)
	job := newSourcePreflightJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePreflightWorker(repos, fixedClock{})
	_, err := worker.ProcessNext(ctx)
	if err == nil {
		t.Fatal("err = nil, want missing root error")
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateFailed ||
		!strings.Contains(updatedJob.ErrorMessage, "source preflight cannot access") {
		t.Fatalf("job = %#v, want failed cannot access", updatedJob)
	}
}

func TestSourcePreflightWorkerFailsFileRoot(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	root := t.TempDir()
	filePath := filepath.Join(root, "notes.md")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	source := newTestDataSource(t, filePath, domain.DataSourceStatusActive)
	job := newSourcePreflightJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePreflightWorker(repos, fixedClock{})
	_, err := worker.ProcessNext(ctx)
	if err == nil {
		t.Fatal("err = nil, want not directory error")
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateFailed ||
		!strings.Contains(updatedJob.ErrorMessage, "not a directory") {
		t.Fatalf("job = %#v, want failed not directory", updatedJob)
	}
}

func TestSourcePreflightWorkerCancelsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, t.TempDir(), domain.DataSourceStatusArchived)
	job := newSourcePreflightJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePreflightWorker(repos, fixedClock{})
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process preflight: %v", err)
	}
	if result.SourceID != source.ID {
		t.Fatalf("source id = %q, want %q", result.SourceID, source.ID)
	}
	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateCanceled {
		t.Fatalf("job state = %q, want canceled", updatedJob.State)
	}
}

func TestSourcePreflightWorkerReturnsNoQueuedJobs(t *testing.T) {
	worker := NewSourcePreflightWorker(memory.New(), fixedClock{})

	_, err := worker.ProcessNext(context.Background())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func newSourcePreflightJob(t *testing.T, source domain.DataSource) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_source_preflight"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePreflight,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}
