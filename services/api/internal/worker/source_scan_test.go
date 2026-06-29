package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourceScanWorkerImportsSupportedFiles(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "overview.md"), "alpha docs")
	mustWriteFile(t, filepath.Join(root, "nested", "guide.txt"), "beta docs")
	mustWriteFile(t, filepath.Join(root, "image.png"), "not supported")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	job := newSourceScanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).WithObjectStore(objects)
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source scan: %v", err)
	}
	if result.ImportedCount != 2 || result.SkippedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("result = %#v, want 2 imported, 1 skipped, 0 failed", result)
	}

	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.Status != domain.DataSourceStatusActive {
		t.Fatalf("source status = %q, want active", updatedSource.Status)
	}
	if updatedSource.LastScanAt == nil {
		t.Fatal("last scan time is nil")
	}
	if updatedSource.LastScanImported != 2 ||
		updatedSource.LastScanSkipped != 1 ||
		updatedSource.LastScanFailed != 0 {
		t.Fatalf("source scan counts = %d/%d/%d, want 2/1/0", updatedSource.LastScanImported, updatedSource.LastScanSkipped, updatedSource.LastScanFailed)
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get scan job: %v", err)
	}
	if updatedJob.State != domain.JobStateSucceeded || updatedJob.Attempts != 1 {
		t.Fatalf("scan job = %#v, want succeeded with one attempt", updatedJob)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(documents))
	}
	jobs, err := repos.ListJobs(ctx, source.TenantID, 10)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	ingestionJobs := 0
	for _, listedJob := range jobs {
		if listedJob.Type == domain.JobTypeDocumentIngestion {
			ingestionJobs++
			if listedJob.State != domain.JobStateQueued || listedJob.ResourceType != "document" {
				t.Fatalf("ingestion job = %#v", listedJob)
			}
		}
	}
	if ingestionJobs != 2 {
		t.Fatalf("ingestion jobs = %d, want 2", ingestionJobs)
	}
	if keys := objects.Keys(); len(keys) != 2 {
		t.Fatalf("object keys = %v, want 2 objects", keys)
	}
	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("scan entries len = %d, want 3", len(entries))
	}
	outcomes := scanEntriesByPath(entries)
	if outcomes["overview.md"].Outcome != domain.DataSourceScanOutcomeImported ||
		!strings.HasPrefix(outcomes["overview.md"].ContentHash, "sha256:") ||
		outcomes["nested/guide.txt"].Outcome != domain.DataSourceScanOutcomeImported ||
		outcomes["image.png"].Outcome != domain.DataSourceScanOutcomeSkipped ||
		outcomes["image.png"].Reason != "unsupported_type" {
		t.Fatalf("scan entries = %#v", outcomes)
	}
}

func TestSourceScanWorkerSkipsUnchangedFilesOnRescan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "overview.md"), "alpha docs")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	firstJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_1"))
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, firstJob); err != nil {
		t.Fatalf("save first job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).WithObjectStore(objects)
	firstResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if firstResult.ImportedCount != 1 || firstResult.SkippedCount != 0 {
		t.Fatalf("first result = %#v, want one import", firstResult)
	}

	secondJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_2"))
	if err := repos.SaveJob(ctx, secondJob); err != nil {
		t.Fatalf("save second job: %v", err)
	}
	secondResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if secondResult.ImportedCount != 0 || secondResult.SkippedCount != 1 || secondResult.FailedCount != 0 {
		t.Fatalf("second result = %#v, want unchanged skip", secondResult)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 1 {
		t.Fatalf("documents len = %d, want unchanged rescan to keep one document", len(documents))
	}
	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.LastScanImported != 0 || updatedSource.LastScanSkipped != 1 || updatedSource.LastScanFailed != 0 {
		t.Fatalf("source counts = %d/%d/%d, want 0/1/0", updatedSource.LastScanImported, updatedSource.LastScanSkipped, updatedSource.LastScanFailed)
	}

	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	var unchanged domain.DataSourceScanEntry
	for _, entry := range entries {
		if entry.JobID == secondJob.ID && entry.Path == "overview.md" {
			unchanged = entry
			break
		}
	}
	if unchanged.Outcome != domain.DataSourceScanOutcomeSkipped ||
		unchanged.Reason != "unchanged" ||
		unchanged.DocumentID == "" ||
		!strings.HasPrefix(unchanged.ContentHash, "sha256:") {
		t.Fatalf("unchanged entry = %#v", unchanged)
	}
}

func TestSourceScanWorkerAppliesDefaultSafetyPolicy(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "ok.md"), "alpha")
	mustWriteFile(t, filepath.Join(root, ".hidden.md"), "hidden docs")
	mustWriteFile(t, filepath.Join(root, "node_modules", "package.md"), "dependency docs")
	mustWriteFile(t, filepath.Join(root, "large.md"), "large docs")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	job := newSourceScanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).
		WithObjectStore(objects).
		WithPolicy(SourceScanPolicy{
			MaxFileBytes:    6,
			SkipHiddenNames: true,
			SkipDirectoryName: map[string]bool{
				"node_modules": true,
			},
		})
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source scan: %v", err)
	}
	if result.ImportedCount != 1 ||
		result.SkippedCount != 3 ||
		result.SkippedPolicyCount != 2 ||
		result.SkippedTooLargeCount != 1 ||
		result.FailedCount != 0 {
		t.Fatalf("result = %#v, want 1 imported, 3 skipped with policy/large counts", result)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 1 || documents[0].Name != "ok.md" {
		t.Fatalf("documents = %#v, want only ok.md", documents)
	}
	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.LastScanImported != 1 || updatedSource.LastScanSkipped != 3 {
		t.Fatalf("source counts = %d/%d, want 1/3", updatedSource.LastScanImported, updatedSource.LastScanSkipped)
	}
	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	outcomes := scanEntriesByPath(entries)
	if outcomes["ok.md"].Outcome != domain.DataSourceScanOutcomeImported ||
		outcomes[".hidden.md"].Reason != "policy" ||
		outcomes["node_modules/"].Reason != "policy" ||
		outcomes["large.md"].Reason != "too_large" {
		t.Fatalf("scan entries = %#v", outcomes)
	}
}

func TestSourceScanWorkerFailsMissingRoot(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, filepath.Join(t.TempDir(), "missing"), domain.DataSourceStatusActive)
	job := newSourceScanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).WithObjectStore(objectmemory.New())
	_, err := worker.ProcessNext(ctx)
	if err == nil {
		t.Fatal("err = nil, want missing root error")
	}

	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.Status != domain.DataSourceStatusFailed {
		t.Fatalf("source status = %q, want failed", updatedSource.Status)
	}
	if updatedSource.LastScanImported != 0 ||
		updatedSource.LastScanSkipped != 0 ||
		updatedSource.LastScanFailed != 1 {
		t.Fatalf("source counts = %d/%d/%d, want 0/0/1", updatedSource.LastScanImported, updatedSource.LastScanSkipped, updatedSource.LastScanFailed)
	}
	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateFailed || updatedJob.ErrorMessage == "" {
		t.Fatalf("job = %#v, want failed with message", updatedJob)
	}
	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	if len(entries) != 1 ||
		entries[0].Outcome != domain.DataSourceScanOutcomeFailed ||
		entries[0].Reason != "cannot_access" {
		t.Fatalf("scan entries = %#v", entries)
	}
}

func TestSourceScanWorkerCancelsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, t.TempDir(), domain.DataSourceStatusArchived)
	job := newSourceScanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).WithObjectStore(objectmemory.New())
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source scan: %v", err)
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

func TestSourceScanWorkerReturnsNoQueuedJobs(t *testing.T) {
	worker := NewSourceScanWorker(memory.New(), &scanIDs{}, fixedClock{}).WithObjectStore(objectmemory.New())

	_, err := worker.ProcessNext(context.Background())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func newTestDataSource(t *testing.T, root string, status domain.DataSourceStatus) domain.DataSource {
	t.Helper()
	source, err := domain.NewDataSource(domain.DataSourceCreate{
		ID:       domain.DataSourceID("src_1"),
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Type:     domain.DataSourceTypeFolder,
		Name:     "Source",
		RootPath: root,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	if status != domain.DataSourceStatusActive {
		if err := source.Transition(status, fixedTime()); err != nil {
			t.Fatalf("transition source: %v", err)
		}
	}
	return source
}

func newSourceScanJob(t *testing.T, source domain.DataSource) domain.Job {
	t.Helper()
	return newSourceScanJobWithID(t, source, domain.JobID("job_source_scan"))
}

func newSourceScanJobWithID(t *testing.T, source domain.DataSource, jobID domain.JobID) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           jobID,
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func scanEntriesByPath(entries []domain.DataSourceScanEntry) map[string]domain.DataSourceScanEntry {
	byPath := make(map[string]domain.DataSourceScanEntry, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry
	}
	return byPath
}

type scanIDs struct {
	document int
	job      int
}

func (g *scanIDs) NewDocumentID() domain.DocumentID {
	g.document++
	return domain.DocumentID(fmt.Sprintf("doc_scan_%d", g.document))
}

func (g *scanIDs) NewJobID() domain.JobID {
	g.job++
	return domain.JobID(fmt.Sprintf("job_ingest_%d", g.job))
}
