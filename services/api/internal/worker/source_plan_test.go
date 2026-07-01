package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourcePlanWorkerSummarizesSourceWithoutImporting(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	root := t.TempDir()
	writePlanFixture(t, root, "keep.md", "alpha")
	writePlanFixture(t, root, "notes.txt", "beta")
	writePlanFixture(t, root, "skip.tmp", "gamma")
	writePlanFixture(t, root, "large.md", "0123456789")
	if err := os.Mkdir(filepath.Join(root, ".cache"), 0o700); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	writePlanFixture(t, root, ".cache/hidden.md", "delta")

	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	if err := source.Update(
		source.Name,
		source.Type,
		source.RootPath,
		domain.ConnectorConfig{},
		[]string{"**/*.md", "**/*.txt"},
		[]string{"notes.txt"},
		0,
		fixedTime(),
	); err != nil {
		t.Fatalf("update source patterns: %v", err)
	}
	job := newSourcePlanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePlanWorker(repos, fixedClock{}).WithPolicy(SourceScanPolicy{
		MaxFileBytes:    6,
		SkipHiddenNames: true,
		SkipDirectoryName: map[string]bool{
			".cache": true,
		},
	})
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source plan: %v", err)
	}
	if result.SourceID != source.ID ||
		result.Summary.WouldImport != 1 ||
		result.Summary.Skipped != 4 ||
		result.Summary.Failed != 0 ||
		result.Summary.EstimatedBytes != 5 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Summary.Reasons["excluded"] != 1 ||
		result.Summary.Reasons["not_included"] != 1 ||
		result.Summary.Reasons["too_large"] != 1 ||
		result.Summary.Reasons["policy"] != 1 {
		t.Fatalf("reasons = %#v", result.Summary.Reasons)
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateSucceeded || updatedJob.ResultJSON == "" {
		t.Fatalf("job = %#v, want succeeded with result JSON", updatedJob)
	}
	var summary SourcePlanSummary
	if err := json.Unmarshal([]byte(updatedJob.ResultJSON), &summary); err != nil {
		t.Fatalf("decode result JSON: %v", err)
	}
	if summary.WouldImport != result.Summary.WouldImport || len(summary.Samples) == 0 {
		t.Fatalf("stored summary = %#v", summary)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 0 {
		t.Fatalf("documents len = %d, want 0", len(documents))
	}
}

func TestSourcePlanWorkerCapturesBoundedReviewSamples(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	root := t.TempDir()
	totalFiles := sourcePlanSampleLimit + 5
	for index := 0; index < totalFiles; index++ {
		writePlanFixture(t, root, fmt.Sprintf("doc-%03d.md", index), "alpha")
	}
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	job := newSourcePlanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	result, err := NewSourcePlanWorker(repos, fixedClock{}).ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source plan: %v", err)
	}
	if result.Summary.WouldImport != totalFiles {
		t.Fatalf("would import = %d, want %d", result.Summary.WouldImport, totalFiles)
	}
	if len(result.Summary.Samples) != sourcePlanSampleLimit {
		t.Fatalf("samples len = %d, want %d", len(result.Summary.Samples), sourcePlanSampleLimit)
	}
}

func TestSourcePlanWorkerFailsMissingRoot(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, filepath.Join(t.TempDir(), "missing"), domain.DataSourceStatusActive)
	job := newSourcePlanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePlanWorker(repos, fixedClock{})
	_, err := worker.ProcessNext(ctx)
	if err == nil {
		t.Fatal("err = nil, want missing root error")
	}
	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateFailed || updatedJob.ResultJSON != "" {
		t.Fatalf("job = %#v, want failed without result JSON", updatedJob)
	}
}

func TestSourcePlanWorkerCancelsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, t.TempDir(), domain.DataSourceStatusArchived)
	job := newSourcePlanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourcePlanWorker(repos, fixedClock{})
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source plan: %v", err)
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

func TestSourcePlanWorkerReturnsNoQueuedJobs(t *testing.T) {
	worker := NewSourcePlanWorker(memory.New(), fixedClock{})

	_, err := worker.ProcessNext(context.Background())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func newSourcePlanJob(t *testing.T, source domain.DataSource) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_source_plan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePlan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}

func writePlanFixture(t *testing.T, root string, relativePath string, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir fixture %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", relativePath, err)
	}
}
