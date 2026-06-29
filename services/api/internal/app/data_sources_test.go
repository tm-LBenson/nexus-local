package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestCreateDataSource(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDataSourceService(repos, fixedIDs{}, fixedClock{})

	result, err := service.Create(ctx, CreateDataSourceInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Type:     domain.DataSourceTypeSyncedFolder,
		Name:     "OneDrive Docs",
		RootPath: "C:\\Users\\team\\OneDrive\\Docs",
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
	if saved.Name != "OneDrive Docs" {
		t.Fatalf("name = %q, want OneDrive Docs", saved.Name)
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
		TenantID:     source.TenantID,
		DataSourceID: source.ID,
		Type:         domain.DataSourceTypeNetworkShare,
		Name:         "NAS Runbooks",
		RootPath:     "\\\\nas\\runbooks",
	})
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if result.Source.Name != "NAS Runbooks" || result.Source.Type != domain.DataSourceTypeNetworkShare {
		t.Fatalf("source = %#v", result.Source)
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
