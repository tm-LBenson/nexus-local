package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
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
	var run SourceScanResult
	if err := json.Unmarshal([]byte(updatedJob.ResultJSON), &run); err != nil {
		t.Fatalf("decode scan result: %v", err)
	}
	if run.JobID != job.ID ||
		run.SourceID != source.ID ||
		run.ImportedCount != 2 ||
		run.SkippedCount != 1 ||
		run.FailedCount != 0 ||
		run.StartedAt.IsZero() ||
		run.FinishedAt.IsZero() {
		t.Fatalf("scan run = %#v", run)
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

func TestSourceScanWorkerReplacesChangedFileDocumentOnRescan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	vectors := vectormemory.New()
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

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).
		WithObjectStore(objects).
		WithVectorIndex(vectors)
	firstResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if firstResult.ImportedCount != 1 {
		t.Fatalf("first result = %#v, want one import", firstResult)
	}
	firstEntries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list first scan entries: %v", err)
	}
	firstDocumentID := scanEntriesByPath(firstEntries)["overview.md"].DocumentID
	if firstDocumentID == "" {
		t.Fatalf("first entries = %#v, want imported document id", firstEntries)
	}
	if err := vectors.Upsert(ctx, []providers.Vector{{
		TenantID:   source.TenantID,
		DocumentID: firstDocumentID,
		ChunkID:    "chunk_1",
		Values:     []float32{1},
		Text:       "alpha docs",
	}}); err != nil {
		t.Fatalf("upsert old vector: %v", err)
	}

	mustWriteFile(t, filepath.Join(root, "overview.md"), "beta docs")
	secondJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_2"))
	if err := repos.SaveJob(ctx, secondJob); err != nil {
		t.Fatalf("save second job: %v", err)
	}
	secondResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if secondResult.ImportedCount != 1 || secondResult.SkippedCount != 0 || secondResult.FailedCount != 0 {
		t.Fatalf("second result = %#v, want changed file replacement", secondResult)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if activeDocumentCount(documents) != 1 {
		t.Fatalf("documents = %#v, want one active document after replacement", documents)
	}
	oldDocument, err := repos.GetDocument(ctx, source.TenantID, firstDocumentID)
	if err != nil {
		t.Fatalf("get old document: %v", err)
	}
	if oldDocument.Status != domain.DocumentStatusDeleted {
		t.Fatalf("old document status = %q, want deleted", oldDocument.Status)
	}
	if keys := objects.Keys(); len(keys) != 1 {
		t.Fatalf("object keys = %v, want only replacement object", keys)
	}
	if vectors.Count() != 0 {
		t.Fatalf("vector count = %d, want old document vectors removed", vectors.Count())
	}

	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	var changed domain.DataSourceScanEntry
	for _, entry := range entries {
		if entry.JobID == secondJob.ID && entry.Path == "overview.md" {
			changed = entry
			break
		}
	}
	if changed.Outcome != domain.DataSourceScanOutcomeImported ||
		changed.Reason != "changed" ||
		changed.DocumentID == "" ||
		changed.DocumentID == firstDocumentID ||
		!strings.Contains(changed.Message, string(firstDocumentID)) {
		t.Fatalf("changed entry = %#v", changed)
	}
}

func TestSourceScanWorkerDeletesMissingSourceFileDocumentOnRescan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	vectors := vectormemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.md"), "keep docs")
	mustWriteFile(t, filepath.Join(root, "remove.md"), "remove docs")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	firstJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_1"))
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, firstJob); err != nil {
		t.Fatalf("save first job: %v", err)
	}

	worker := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).
		WithObjectStore(objects).
		WithVectorIndex(vectors)
	firstResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if firstResult.ImportedCount != 2 || firstResult.SkippedCount != 0 || firstResult.FailedCount != 0 {
		t.Fatalf("first result = %#v, want two imports", firstResult)
	}
	firstEntries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list first scan entries: %v", err)
	}
	firstByPath := scanEntriesByPath(firstEntries)
	removedDocumentID := firstByPath["remove.md"].DocumentID
	if removedDocumentID == "" {
		t.Fatalf("first entries = %#v, want removed document id", firstEntries)
	}
	if err := vectors.Upsert(ctx, []providers.Vector{{
		TenantID:   source.TenantID,
		DocumentID: removedDocumentID,
		ChunkID:    "chunk_1",
		Values:     []float32{1},
		Text:       "remove docs",
	}}); err != nil {
		t.Fatalf("upsert removed vector: %v", err)
	}

	if err := os.Remove(filepath.Join(root, "remove.md")); err != nil {
		t.Fatalf("remove source file: %v", err)
	}
	secondJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_2"))
	if err := repos.SaveJob(ctx, secondJob); err != nil {
		t.Fatalf("save second job: %v", err)
	}
	secondResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if secondResult.ImportedCount != 0 || secondResult.SkippedCount != 2 || secondResult.FailedCount != 0 {
		t.Fatalf("second result = %#v, want unchanged keep plus deleted missing file", secondResult)
	}
	if secondResult.DeletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1", secondResult.DeletedCount)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if activeDocumentCount(documents) != 1 {
		t.Fatalf("documents = %#v, want one active document after missing file cleanup", documents)
	}
	removedDocument, err := repos.GetDocument(ctx, source.TenantID, removedDocumentID)
	if err != nil {
		t.Fatalf("get removed document: %v", err)
	}
	if removedDocument.Status != domain.DocumentStatusDeleted {
		t.Fatalf("removed document status = %q, want deleted", removedDocument.Status)
	}
	if keys := objects.Keys(); len(keys) != 1 {
		t.Fatalf("object keys = %v, want only retained source object", keys)
	}
	if vectors.Count() != 0 {
		t.Fatalf("vector count = %d, want removed document vectors deleted", vectors.Count())
	}

	secondEntries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list second scan entries: %v", err)
	}
	var deleted domain.DataSourceScanEntry
	for _, entry := range secondEntries {
		if entry.JobID == secondJob.ID && entry.Path == "remove.md" {
			deleted = entry
			break
		}
	}
	if deleted.Outcome != domain.DataSourceScanOutcomeDeleted ||
		deleted.Reason != "missing" ||
		deleted.DocumentID != removedDocumentID ||
		!strings.Contains(deleted.Message, string(removedDocumentID)) {
		t.Fatalf("deleted entry = %#v", deleted)
	}

	thirdJob := newSourceScanJobWithID(t, source, domain.JobID("job_source_scan_3"))
	if err := repos.SaveJob(ctx, thirdJob); err != nil {
		t.Fatalf("save third job: %v", err)
	}
	thirdResult, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if thirdResult.ImportedCount != 0 || thirdResult.SkippedCount != 1 || thirdResult.FailedCount != 0 {
		t.Fatalf("third result = %#v, want only retained unchanged file", thirdResult)
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

func TestSourceScanWorkerAppliesSourcePatterns(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.md"), "alpha")
	mustWriteFile(t, filepath.Join(root, "notes.txt"), "text docs")
	mustWriteFile(t, filepath.Join(root, "draft.md"), "draft docs")
	mustWriteFile(t, filepath.Join(root, "archive", "old.md"), "old docs")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	source.IncludePatterns = []string{"*.md"}
	source.ExcludePatterns = []string{"draft.md", "archive/**"}
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
	if result.ImportedCount != 1 ||
		result.SkippedCount != 3 ||
		result.SkippedPolicyCount != 3 ||
		result.FailedCount != 0 {
		t.Fatalf("result = %#v, want 1 imported and 3 pattern skips", result)
	}

	documents, err := repos.ListDocuments(ctx, source.TenantID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 1 || documents[0].Name != "keep.md" {
		t.Fatalf("documents = %#v, want only keep.md", documents)
	}
	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list scan entries: %v", err)
	}
	outcomes := scanEntriesByPath(entries)
	if outcomes["keep.md"].Outcome != domain.DataSourceScanOutcomeImported ||
		outcomes["notes.txt"].Reason != "not_included" ||
		outcomes["draft.md"].Reason != "excluded" ||
		outcomes["archive/"].Reason != "excluded" {
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
	var run SourceScanResult
	if err := json.Unmarshal([]byte(updatedJob.ResultJSON), &run); err != nil {
		t.Fatalf("decode failed scan result: %v", err)
	}
	if run.JobID != job.ID || run.SourceID != source.ID || run.FailedCount != 1 {
		t.Fatalf("failed scan run = %#v", run)
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
	var run SourceScanResult
	if err := json.Unmarshal([]byte(updatedJob.ResultJSON), &run); err != nil {
		t.Fatalf("decode canceled scan result: %v", err)
	}
	if run.JobID != job.ID || run.SourceID != source.ID {
		t.Fatalf("canceled scan run = %#v", run)
	}
}

func TestSourceScanWorkerStopsWhenScanJobIsCanceled(t *testing.T) {
	ctx := context.Background()
	base := memory.New()
	repos := &cancelingJobStore{RepositorySet: base, cancelAfter: 3}
	objects := objectmemory.New()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.md"), "alpha")
	mustWriteFile(t, filepath.Join(root, "b.md"), "beta")
	source := newTestDataSource(t, root, domain.DataSourceStatusActive)
	job := newSourceScanJob(t, source)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	result, err := NewSourceScanWorker(repos, &scanIDs{}, fixedClock{}).
		WithObjectStore(objects).
		ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process source scan: %v", err)
	}
	if result.ImportedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("result = %#v, want one imported before cancellation", result)
	}
	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateCanceled || updatedJob.ResultJSON == "" {
		t.Fatalf("job = %#v, want canceled with result", updatedJob)
	}
	updatedSource, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.Status != domain.DataSourceStatusActive {
		t.Fatalf("source status = %q, want active", updatedSource.Status)
	}
	entries, err := repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 10)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one entry before cancellation", entries)
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

func activeDocumentCount(documents []domain.Document) int {
	count := 0
	for _, document := range documents {
		if document.Status != domain.DocumentStatusDeleted {
			count++
		}
	}
	return count
}

type cancelingJobStore struct {
	store.RepositorySet
	cancelAfter int
	gets        int
}

func (s *cancelingJobStore) GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error) {
	job, err := s.RepositorySet.GetJob(ctx, tenantID, id)
	if err != nil {
		return domain.Job{}, err
	}
	if job.Type == domain.JobTypeSourceScan && job.State == domain.JobStateRunning {
		s.gets++
		if s.gets >= s.cancelAfter {
			if err := job.Transition(domain.JobStateCanceled, fixedTime().Add(time.Second)); err != nil {
				return domain.Job{}, err
			}
			if err := s.RepositorySet.SaveJob(ctx, job); err != nil {
				return domain.Job{}, err
			}
		}
	}
	return job, nil
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
