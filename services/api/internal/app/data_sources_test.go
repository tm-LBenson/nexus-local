package app

import (
	"context"
	"errors"
	"fmt"
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

func TestCreateDataSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})

	result, err := service.Create(ctx, CreateDataSourceInput{
		TenantID:            domain.TenantID("tenant_1"),
		OwnerID:             domain.UserID("user_1"),
		Type:                domain.DataSourceTypeSyncedFolder,
		Name:                "OneDrive Docs",
		RootPath:            "C:\\Users\\team\\OneDrive\\Docs",
		IncludePatterns:     []string{"**/*.md"},
		ExcludePatterns:     []string{"archive/**"},
		ScanIntervalMinutes: 60,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if result.Source.ID != domain.DataSourceID("src_fixed") {
		t.Fatalf("source id = %q, want src_fixed", result.Source.ID)
	}
	if result.Source.Status != domain.DataSourceStatusActive {
		t.Fatalf("status = %q, want active", result.Source.Status)
	}

	saved, err := repos.GetDataSource(ctx, domain.TenantID("tenant_1"), domain.DataSourceID("src_fixed"))
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if saved.Name != "OneDrive Docs" ||
		len(saved.IncludePatterns) != 1 ||
		saved.IncludePatterns[0] != "**/*.md" ||
		len(saved.ExcludePatterns) != 1 ||
		saved.ExcludePatterns[0] != "archive/**" ||
		saved.ScanIntervalMinutes != 60 ||
		saved.NextScanAt == nil {
		t.Fatalf("saved source = %#v", saved)
	}
}

func TestListDataSourcesFiltersArchived(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})

	active := newDataSource(t, "src_active", domain.DataSourceStatusActive)
	archived := newDataSource(t, "src_archived", domain.DataSourceStatusArchived)
	if err := repos.SaveDataSource(ctx, active); err != nil {
		t.Fatalf("save active: %v", err)
	}
	if err := repos.SaveDataSource(ctx, archived); err != nil {
		t.Fatalf("save archived: %v", err)
	}

	result, err := service.List(ctx, ListDataSourcesInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != active.ID {
		t.Fatalf("sources = %#v, want only active", result.Sources)
	}

	all, err := service.List(ctx, ListDataSourcesInput{
		TenantID:        domain.TenantID("tenant_1"),
		IncludeArchived: true,
	})
	if err != nil {
		t.Fatalf("list all sources: %v", err)
	}
	if len(all.Sources) != 2 {
		t.Fatalf("all sources len = %d, want 2", len(all.Sources))
	}
}

func TestUpdateDataSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	result, err := service.Update(ctx, UpdateDataSourceInput{
		TenantID:            source.TenantID,
		DataSourceID:        source.ID,
		Type:                domain.DataSourceTypeNetworkShare,
		Name:                "NAS Runbooks",
		RootPath:            "\\\\nas\\runbooks",
		IncludePatterns:     []string{"runbooks/**"},
		ExcludePatterns:     []string{"drafts/**"},
		ScanIntervalMinutes: 1440,
	})
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if result.Source.Name != "NAS Runbooks" || result.Source.Type != domain.DataSourceTypeNetworkShare {
		t.Fatalf("source = %#v", result.Source)
	}
	if len(result.Source.IncludePatterns) != 1 ||
		result.Source.IncludePatterns[0] != "runbooks/**" ||
		len(result.Source.ExcludePatterns) != 1 ||
		result.Source.ExcludePatterns[0] != "drafts/**" {
		t.Fatalf("patterns = %#v/%#v", result.Source.IncludePatterns, result.Source.ExcludePatterns)
	}
	if result.Source.ScanIntervalMinutes != 1440 || result.Source.NextScanAt == nil {
		t.Fatalf("schedule = %d/%v", result.Source.ScanIntervalMinutes, result.Source.NextScanAt)
	}
}

func TestGetDataSourceIncludesRelatedJobs(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	sourceJob, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_source"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new source job: %v", err)
	}
	otherJob, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_other"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   "doc_1",
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new other job: %v", err)
	}
	if err := repos.SaveJob(ctx, sourceJob); err != nil {
		t.Fatalf("save source job: %v", err)
	}
	if err := repos.SaveJob(ctx, otherJob); err != nil {
		t.Fatalf("save other job: %v", err)
	}
	entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
		TenantID: source.TenantID,
		JobID:    sourceJob.ID,
		SourceID: source.ID,
		Path:     "runbooks/setup.md",
		Outcome:  domain.DataSourceScanOutcomeImported,
		Now:      fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new scan entry: %v", err)
	}
	if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
		t.Fatalf("save scan entry: %v", err)
	}

	result, err := service.Get(ctx, DataSourceDetailInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if result.Source.ID != source.ID {
		t.Fatalf("source id = %q, want %q", result.Source.ID, source.ID)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].ID != sourceJob.ID {
		t.Fatalf("jobs = %#v, want only source job", result.Jobs)
	}
	if len(result.ScanEntries) != 1 || result.ScanEntries[0].Path != "runbooks/setup.md" {
		t.Fatalf("scan entries = %#v, want imported setup entry", result.ScanEntries)
	}
	if result.ScanSummary.Total != 1 ||
		result.ScanSummary.Imported != 1 ||
		result.ScanSummary.LatestJobID != sourceJob.ID {
		t.Fatalf("scan summary = %#v, want one imported entry for source job", result.ScanSummary)
	}
}

func TestGetDataSourceSummarizesLatestScanEntries(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	oldJobID := domain.JobID("job_old")
	latestJobID := domain.JobID("job_latest")
	entries := []domain.DataSourceScanEntry{
		newSourceScanEntryAt(t, source, oldJobID, "old.md", domain.DataSourceScanOutcomeImported, "", domain.DocumentID("doc_old"), fixedClock{}.Now().Add(-time.Hour)),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/setup.md", domain.DataSourceScanOutcomeImported, "", domain.DocumentID("doc_setup"), fixedClock{}.Now()),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/unchanged.md", domain.DataSourceScanOutcomeSkipped, "unchanged", "", fixedClock{}.Now()),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/broken.pdf", domain.DataSourceScanOutcomeFailed, "parse_failed", "", fixedClock{}.Now()),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/removed.md", domain.DataSourceScanOutcomeDeleted, "missing", domain.DocumentID("doc_removed"), fixedClock{}.Now()),
	}
	for _, entry := range entries {
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	}

	result, err := service.Get(ctx, DataSourceDetailInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if result.ScanSummary.LatestJobID != latestJobID ||
		result.ScanSummary.Total != 4 ||
		result.ScanSummary.Imported != 1 ||
		result.ScanSummary.Skipped != 1 ||
		result.ScanSummary.Failed != 1 ||
		result.ScanSummary.Deleted != 1 {
		t.Fatalf("scan summary = %#v, want latest scan counts", result.ScanSummary)
	}
	if result.ScanSummary.Reasons["unchanged"] != 1 ||
		result.ScanSummary.Reasons["parse_failed"] != 1 ||
		result.ScanSummary.Reasons["missing"] != 1 {
		t.Fatalf("scan summary reasons = %#v", result.ScanSummary.Reasons)
	}
}

func TestGetDataSourceSummarizesLatestScanBeyondDefaultPage(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_large_summary", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	latestJobID := domain.JobID("job_latest")
	for index := 0; index < 125; index++ {
		outcome := domain.DataSourceScanOutcomeImported
		reason := ""
		if index%5 == 0 {
			outcome = domain.DataSourceScanOutcomeSkipped
			reason = "unsupported_type"
		}
		entry := newSourceScanEntryAt(
			t,
			source,
			latestJobID,
			fmt.Sprintf("batch/doc-%03d.md", index),
			outcome,
			reason,
			domain.DocumentID(fmt.Sprintf("doc_%03d", index)),
			fixedClock{}.Now(),
		)
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	}

	result, err := service.Get(ctx, DataSourceDetailInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if result.ScanSummary.Total != 125 ||
		result.ScanSummary.Imported != 100 ||
		result.ScanSummary.Skipped != 25 ||
		result.ScanSummary.Reasons["unsupported_type"] != 25 {
		t.Fatalf("scan summary = %#v, want full latest scan counts", result.ScanSummary)
	}
	if len(result.ScanEntries) != defaultScanEntryListLimit {
		t.Fatalf("scan entries len = %d, want paged default %d", len(result.ScanEntries), defaultScanEntryListLimit)
	}
}

func TestGetDataSourcePaginatesScanEntries(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	latestJobID := domain.JobID("job_latest")
	entries := []domain.DataSourceScanEntry{
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/setup.md", domain.DataSourceScanOutcomeImported, "", domain.DocumentID("doc_setup"), fixedClock{}.Now()),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/a-broken.pdf", domain.DataSourceScanOutcomeFailed, "parse_failed", "", fixedClock{}.Now()),
		newSourceScanEntryAt(t, source, latestJobID, "runbooks/b-broken.pdf", domain.DataSourceScanOutcomeFailed, "parse_failed", "", fixedClock{}.Now()),
	}
	for _, entry := range entries {
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	}

	result, err := service.Get(ctx, DataSourceDetailInput{
		TenantID:         source.TenantID,
		DataSourceID:     source.ID,
		ScanEntryOutcome: domain.DataSourceScanOutcomeFailed,
		ScanEntryLimit:   1,
		ScanEntryOffset:  1,
	})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if result.ScanPage.Total != 2 ||
		result.ScanPage.Limit != 1 ||
		result.ScanPage.Offset != 1 ||
		result.ScanPage.Outcome != domain.DataSourceScanOutcomeFailed {
		t.Fatalf("scan page = %#v, want failed page metadata", result.ScanPage)
	}
	if len(result.ScanEntries) != 1 || result.ScanEntries[0].Path != "runbooks/b-broken.pdf" {
		t.Fatalf("scan entries = %#v, want second failed entry", result.ScanEntries)
	}
	if result.ScanSummary.Total != 3 || result.ScanSummary.Failed != 2 || result.ScanSummary.Imported != 1 {
		t.Fatalf("scan summary = %#v, want unfiltered latest scan summary", result.ScanSummary)
	}
}

func TestArchiveDataSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	result, err := service.Archive(ctx, ArchiveDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("archive source: %v", err)
	}
	if result.Source.Status != domain.DataSourceStatusArchived {
		t.Fatalf("status = %q, want archived", result.Source.Status)
	}

	list, err := service.List(ctx, ListDataSourcesInput{TenantID: source.TenantID})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(list.Sources) != 0 {
		t.Fatalf("sources len = %d, want 0", len(list.Sources))
	}
}

func TestArchiveDataSourceCanDeleteImportedDocuments(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	vectors := vectormemory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{}).
		WithObjectStore(objects).
		WithVectorIndex(vectors)
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	firstDocument := newSourceDocument(t, "doc_source_1", "source/one.md")
	secondDocument := newSourceDocument(t, "doc_source_2", "source/two.md")
	otherDocument := newSourceDocument(t, "doc_other", "other.md")
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	for _, document := range []domain.Document{firstDocument, secondDocument, otherDocument} {
		if err := repos.SaveDocument(ctx, document); err != nil {
			t.Fatalf("save document %s: %v", document.ID, err)
		}
		if _, err := objects.PutObject(ctx, providers.ObjectPut{
			TenantID:    document.TenantID,
			Key:         document.StorageKey,
			Body:        strings.NewReader(string(document.ID)),
			ContentType: "text/markdown",
			SizeBytes:   int64(len(document.ID)),
		}); err != nil {
			t.Fatalf("put object %s: %v", document.ID, err)
		}
	}
	if err := vectors.Upsert(ctx, []providers.Vector{
		{
			TenantID:   source.TenantID,
			DocumentID: firstDocument.ID,
			ChunkID:    "chunk_1",
			Values:     []float32{1},
			Text:       "one",
		},
		{
			TenantID:   source.TenantID,
			DocumentID: secondDocument.ID,
			ChunkID:    "chunk_2",
			Values:     []float32{1},
			Text:       "two",
		},
	}); err != nil {
		t.Fatalf("upsert vectors: %v", err)
	}
	for _, entry := range []domain.DataSourceScanEntry{
		newSourceScanEntry(t, source, domain.JobID("job_scan_1"), "one.md", domain.DataSourceScanOutcomeImported, "", firstDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_1"), "two.md", domain.DataSourceScanOutcomeSkipped, "unchanged", secondDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_2"), "gone.md", domain.DataSourceScanOutcomeDeleted, "missing", domain.DocumentID("doc_gone")),
	} {
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry %s: %v", entry.Path, err)
		}
	}

	result, err := service.Archive(ctx, ArchiveDataSourceInput{
		TenantID:        source.TenantID,
		DataSourceID:    source.ID,
		DeleteDocuments: true,
	})
	if err != nil {
		t.Fatalf("archive source with documents: %v", err)
	}
	if result.Source.Status != domain.DataSourceStatusArchived || result.DeletedDocumentCount != 2 {
		t.Fatalf("result = %#v, want archived with two deleted documents", result)
	}
	for _, documentID := range []domain.DocumentID{firstDocument.ID, secondDocument.ID} {
		document, err := repos.GetDocument(ctx, source.TenantID, documentID)
		if err != nil {
			t.Fatalf("get document %s: %v", documentID, err)
		}
		if document.Status != domain.DocumentStatusDeleted {
			t.Fatalf("document %s status = %q, want deleted", documentID, document.Status)
		}
	}
	other, err := repos.GetDocument(ctx, source.TenantID, otherDocument.ID)
	if err != nil {
		t.Fatalf("get other document: %v", err)
	}
	if other.Status == domain.DocumentStatusDeleted {
		t.Fatalf("other document status = %q, want active", other.Status)
	}
	if keys := objects.Keys(); len(keys) != 1 || keys[0] != otherDocument.StorageKey {
		t.Fatalf("object keys = %v, want only other document object", keys)
	}
	if vectors.Count() != 0 {
		t.Fatalf("vector count = %d, want source vectors deleted", vectors.Count())
	}
}

func TestArchiveDataSourceWithDocumentsRejectsActiveScan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_scan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new scan job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save scan job: %v", err)
	}

	_, err = service.Archive(ctx, ArchiveDataSourceInput{
		TenantID:        source.TenantID,
		DataSourceID:    source.ID,
		DeleteDocuments: true,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
	saved, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if saved.Status != domain.DataSourceStatusActive {
		t.Fatalf("source status = %q, want active", saved.Status)
	}
}

func TestRequestDataSourceScanQueuesJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	result, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request scan: %v", err)
	}
	if result.Source.ID != source.ID {
		t.Fatalf("source id = %q, want %q", result.Source.ID, source.ID)
	}
	if result.Source.Status != domain.DataSourceStatusActive {
		t.Fatalf("source status = %q, want active", result.Source.Status)
	}
	if result.Job.ID != domain.JobID("job_fixed") ||
		result.Job.Type != domain.JobTypeSourceScan ||
		result.Job.ResourceType != "data_source" ||
		result.Job.ResourceID != string(source.ID) ||
		result.Job.State != domain.JobStateQueued {
		t.Fatalf("job = %#v", result.Job)
	}

	savedJob, err := repos.GetJob(ctx, source.TenantID, result.Job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if savedJob.Type != domain.JobTypeSourceScan {
		t.Fatalf("saved job type = %q, want source_scan", savedJob.Type)
	}
}

func TestRequestDataSourceScanRejectsDuplicateActiveScan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_scan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourceScanRejectsActivePreflight(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_preflight"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePreflight,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourceScanRejectsFailedPreflight(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job := failedSourcePreflightJob(t, source, fixedClock{}.Now(), "source preflight cannot access mount")
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) ||
		!strings.Contains(err.Error(), "source preflight cannot access mount") {
		t.Fatalf("err = %v, want failed preflight invalid state", err)
	}
}

func TestRequestDataSourceScanIgnoresStaleFailedPreflight(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	staleFailureAt := fixedClock{}.Now()
	source.UpdatedAt = staleFailureAt.Add(time.Minute)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job := failedSourcePreflightJob(t, source, staleFailureAt, "old path failed")
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	result, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request scan: %v", err)
	}
	if result.Job.Type != domain.JobTypeSourceScan {
		t.Fatalf("job type = %q, want source_scan", result.Job.Type)
	}
}

func TestRequestDataSourceScanRejectsActivePlan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_plan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePlan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourceScanRejectsFailedPlan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job := failedSourcePlanJob(t, source, fixedClock{}.Now(), "source plan cannot access mount")
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) ||
		!strings.Contains(err.Error(), "source plan cannot access mount") {
		t.Fatalf("err = %v, want failed plan invalid state", err)
	}
}

func TestRequestDataSourceScanIgnoresStaleFailedPlan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	staleFailureAt := fixedClock{}.Now()
	source.UpdatedAt = staleFailureAt.Add(time.Minute)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job := failedSourcePlanJob(t, source, staleFailureAt, "old plan failed")
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	result, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request scan: %v", err)
	}
	if result.Job.Type != domain.JobTypeSourceScan {
		t.Fatalf("job type = %q, want source_scan", result.Job.Type)
	}
}

func TestRequestDataSourceScanRejectsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusArchived)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	_, err := service.RequestScan(ctx, ScanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourcePreflightQueuesJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	result, err := service.RequestPreflight(ctx, PreflightDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request preflight: %v", err)
	}
	if result.Source.ID != source.ID {
		t.Fatalf("source id = %q, want %q", result.Source.ID, source.ID)
	}
	if result.Job.ID != domain.JobID("job_fixed") ||
		result.Job.Type != domain.JobTypeSourcePreflight ||
		result.Job.ResourceType != "data_source" ||
		result.Job.ResourceID != string(source.ID) ||
		result.Job.State != domain.JobStateQueued {
		t.Fatalf("job = %#v", result.Job)
	}

	savedJob, err := repos.GetJob(ctx, source.TenantID, result.Job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if savedJob.Type != domain.JobTypeSourcePreflight {
		t.Fatalf("saved job type = %q, want source_preflight", savedJob.Type)
	}
}

func TestRequestDataSourcePreflightRejectsDuplicateActiveCheck(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_preflight"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePreflight,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestPreflight(ctx, PreflightDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourcePreflightRejectsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusArchived)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	_, err := service.RequestPreflight(ctx, PreflightDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourcePlanQueuesJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	result, err := service.RequestPlan(ctx, PlanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request plan: %v", err)
	}
	if result.Source.ID != source.ID {
		t.Fatalf("source id = %q, want %q", result.Source.ID, source.ID)
	}
	if result.Job.ID != domain.JobID("job_fixed") ||
		result.Job.Type != domain.JobTypeSourcePlan ||
		result.Job.ResourceType != "data_source" ||
		result.Job.ResourceID != string(source.ID) ||
		result.Job.State != domain.JobStateQueued {
		t.Fatalf("job = %#v", result.Job)
	}
}

func TestRequestDataSourcePlanRejectsDuplicateActivePlan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_plan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePlan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestPlan(ctx, PlanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourcePlanRejectsActiveScan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_scan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RequestPlan(ctx, PlanDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourceReindexQueuesDocumentJobs(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, &sourceReindexIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	firstDocument := newSourceDocument(t, "doc_source_1", "source/one.md")
	secondDocument := newSourceDocument(t, "doc_source_2", "source/two.md")
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	for _, document := range []domain.Document{firstDocument, secondDocument} {
		if err := repos.SaveDocument(ctx, document); err != nil {
			t.Fatalf("save document %s: %v", document.ID, err)
		}
	}
	for _, entry := range []domain.DataSourceScanEntry{
		newSourceScanEntry(t, source, domain.JobID("job_scan_1"), "one.md", domain.DataSourceScanOutcomeImported, "", firstDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_1"), "two.md", domain.DataSourceScanOutcomeSkipped, "unchanged", secondDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_2"), "gone.md", domain.DataSourceScanOutcomeDeleted, "missing", domain.DocumentID("doc_gone")),
	} {
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry %s: %v", entry.Path, err)
		}
	}
	activeJob, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_active_ingestion"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(secondDocument.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new active job: %v", err)
	}
	if err := repos.SaveJob(ctx, activeJob); err != nil {
		t.Fatalf("save active job: %v", err)
	}

	result, err := service.RequestReindex(ctx, ReindexDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("request reindex: %v", err)
	}
	if result.Source.ID != source.ID || len(result.Jobs) != 1 || result.SkippedDocumentCount != 1 {
		t.Fatalf("result = %#v, want one queued and one skipped", result)
	}
	if result.Jobs[0].Type != domain.JobTypeDocumentIngestion ||
		result.Jobs[0].ResourceType != "document" ||
		result.Jobs[0].ResourceID != string(firstDocument.ID) ||
		result.Jobs[0].State != domain.JobStateQueued {
		t.Fatalf("queued job = %#v", result.Jobs[0])
	}
	savedJob, err := repos.GetJob(ctx, source.TenantID, result.Jobs[0].ID)
	if err != nil {
		t.Fatalf("get saved reindex job: %v", err)
	}
	if savedJob.ResourceID != string(firstDocument.ID) {
		t.Fatalf("saved job = %#v", savedJob)
	}
}

func TestRequestDataSourceReindexRejectsActiveScan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, &sourceReindexIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusActive)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_scan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new scan job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save scan job: %v", err)
	}

	_, err = service.RequestReindex(ctx, ReindexDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestDataSourceReindexRejectsArchivedSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, &sourceReindexIDs{}, fixedClock{})
	source := newDataSource(t, "src_1", domain.DataSourceStatusArchived)
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	_, err := service.RequestReindex(ctx, ReindexDataSourceInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRequestRetryFailedDataSourceDocumentsQueuesOnlyFailedDocuments(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, &sourceReindexIDs{}, fixedClock{})
	source := newDataSource(t, "src_retry", domain.DataSourceStatusActive)
	failedDocument := newSourceDocument(t, "doc_failed", "source/failed.md")
	readyDocument := newSourceDocument(t, "doc_ready", "source/ready.md")
	activeFailedDocument := newSourceDocument(t, "doc_active_failed", "source/active-failed.md")
	for _, document := range []*domain.Document{&failedDocument, &activeFailedDocument} {
		if err := document.Transition(domain.DocumentStatusFailed, fixedClock{}.Now()); err != nil {
			t.Fatalf("mark failed document %s: %v", document.ID, err)
		}
	}
	if err := readyDocument.Transition(domain.DocumentStatusProcessing, fixedClock{}.Now()); err != nil {
		t.Fatalf("mark ready processing: %v", err)
	}
	if err := readyDocument.Transition(domain.DocumentStatusReady, fixedClock{}.Now()); err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	for _, document := range []domain.Document{failedDocument, readyDocument, activeFailedDocument} {
		if err := repos.SaveDocument(ctx, document); err != nil {
			t.Fatalf("save document %s: %v", document.ID, err)
		}
	}
	for _, entry := range []domain.DataSourceScanEntry{
		newSourceScanEntry(t, source, domain.JobID("job_scan_retry"), "failed.md", domain.DataSourceScanOutcomeImported, "", failedDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_retry"), "ready.md", domain.DataSourceScanOutcomeImported, "", readyDocument.ID),
		newSourceScanEntry(t, source, domain.JobID("job_scan_retry"), "active-failed.md", domain.DataSourceScanOutcomeImported, "", activeFailedDocument.ID),
	} {
		if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
			t.Fatalf("save scan entry %s: %v", entry.Path, err)
		}
	}
	activeJob, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_active_failed_retry"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(activeFailedDocument.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new active job: %v", err)
	}
	if err := repos.SaveJob(ctx, activeJob); err != nil {
		t.Fatalf("save active job: %v", err)
	}

	result, err := service.RequestRetryFailedDocuments(ctx, RetryFailedDataSourceDocumentsInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("retry failed source documents: %v", err)
	}
	if result.Source.ID != source.ID || len(result.Jobs) != 1 || result.SkippedDocumentCount != 2 {
		t.Fatalf("result = %#v, want one queued and two skipped", result)
	}
	if result.Jobs[0].Type != domain.JobTypeDocumentIngestion ||
		result.Jobs[0].ResourceType != "document" ||
		result.Jobs[0].ResourceID != string(failedDocument.ID) ||
		result.Jobs[0].State != domain.JobStateQueued {
		t.Fatalf("queued job = %#v", result.Jobs[0])
	}
}

func TestGetDataSourceCountsFailedDocuments(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})
	source := newDataSource(t, "src_failed_docs", domain.DataSourceStatusActive)
	failedDocument := newSourceDocument(t, "doc_failed_count", "source/failed.md")
	if err := failedDocument.Transition(domain.DocumentStatusFailed, fixedClock{}.Now()); err != nil {
		t.Fatalf("mark failed document: %v", err)
	}
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	if err := repos.SaveDocument(ctx, failedDocument); err != nil {
		t.Fatalf("save failed document: %v", err)
	}
	entry := newSourceScanEntry(t, source, domain.JobID("job_scan_failed_docs"), "failed.md", domain.DataSourceScanOutcomeImported, "", failedDocument.ID)
	if err := repos.SaveDataSourceScanEntry(ctx, entry); err != nil {
		t.Fatalf("save scan entry: %v", err)
	}

	result, err := service.Get(ctx, DataSourceDetailInput{
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
	})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if result.FailedDocuments != 1 {
		t.Fatalf("failed documents = %d, want 1", result.FailedDocuments)
	}
}

func TestGetDataSourceRejectsMissingSource(t *testing.T) {
	service := NewDataSourceService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.Get(context.Background(), DataSourceDetailInput{
		TenantID:     domain.TenantID("tenant_1"),
		DataSourceID: domain.DataSourceID("src_missing"),
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func newDataSource(t *testing.T, id string, status domain.DataSourceStatus) domain.DataSource {
	t.Helper()
	source, err := domain.NewDataSource(domain.DataSourceCreate{
		ID:       domain.DataSourceID(id),
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Type:     domain.DataSourceTypeFolder,
		Name:     id,
		RootPath: "C:\\Docs\\" + id,
		Now:      fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	if status != domain.DataSourceStatusActive {
		if err := source.Transition(status, fixedClock{}.Now()); err != nil {
			t.Fatalf("transition source: %v", err)
		}
	}
	return source
}

func newSourceDocument(t *testing.T, id string, name string) domain.Document {
	t.Helper()
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID(id),
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       name,
		StorageKey: "tenants/tenant_1/documents/" + id + "/" + name,
		SizeBytes:  42,
		Now:        fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	return document
}

func newSourceScanEntry(t *testing.T, source domain.DataSource, jobID domain.JobID, path string, outcome domain.DataSourceScanOutcome, reason string, documentID domain.DocumentID) domain.DataSourceScanEntry {
	t.Helper()
	return newSourceScanEntryAt(t, source, jobID, path, outcome, reason, documentID, fixedClock{}.Now())
}

func newSourceScanEntryAt(t *testing.T, source domain.DataSource, jobID domain.JobID, path string, outcome domain.DataSourceScanOutcome, reason string, documentID domain.DocumentID, now time.Time) domain.DataSourceScanEntry {
	t.Helper()
	entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
		TenantID:    source.TenantID,
		JobID:       jobID,
		SourceID:    source.ID,
		Path:        path,
		Outcome:     outcome,
		Reason:      reason,
		DocumentID:  documentID,
		SizeBytes:   42,
		ContentHash: "sha256:" + string(documentID),
		Now:         now,
	})
	if err != nil {
		t.Fatalf("new scan entry: %v", err)
	}
	return entry
}

func failedSourcePreflightJob(t *testing.T, source domain.DataSource, now time.Time, message string) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_preflight"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePreflight,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          now,
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := job.Transition(domain.JobStateRunning, now); err != nil {
		t.Fatalf("run job: %v", err)
	}
	if err := job.Fail(errors.New(message), now); err != nil {
		t.Fatalf("fail job: %v", err)
	}
	return job
}

func failedSourcePlanJob(t *testing.T, source domain.DataSource, now time.Time, message string) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_plan"),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePlan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          now,
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := job.Transition(domain.JobStateRunning, now); err != nil {
		t.Fatalf("run job: %v", err)
	}
	if err := job.Fail(errors.New(message), now); err != nil {
		t.Fatalf("fail job: %v", err)
	}
	return job
}

type sourceReindexIDs struct {
	job int
}

func (g *sourceReindexIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_reindex")
}

func (g *sourceReindexIDs) NewDataSourceID() domain.DataSourceID {
	return domain.DataSourceID("src_reindex")
}

func (g *sourceReindexIDs) NewJobID() domain.JobID {
	g.job++
	return domain.JobID(fmt.Sprintf("job_reindex_%d", g.job))
}
