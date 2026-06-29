package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type DataSourceIDs interface {
	NewDocumentID() domain.DocumentID
	NewDataSourceID() domain.DataSourceID
	NewJobID() domain.JobID
}

type DataSourceService struct {
	repos     store.RepositorySet
	ids       DataSourceIDs
	clock     Clock
	documents DocumentService
}

type CreateDataSourceInput struct {
	TenantID            domain.TenantID
	OwnerID             domain.UserID
	Type                domain.DataSourceType
	Name                string
	RootPath            string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
}

type UpdateDataSourceInput struct {
	TenantID            domain.TenantID
	DataSourceID        domain.DataSourceID
	Type                domain.DataSourceType
	Name                string
	RootPath            string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
}

type DataSourceDetailInput struct {
	TenantID         domain.TenantID
	DataSourceID     domain.DataSourceID
	ScanEntryLimit   int
	ScanEntryOffset  int
	ScanEntryOutcome domain.DataSourceScanOutcome
}

type ListDataSourcesInput struct {
	TenantID        domain.TenantID
	IncludeArchived bool
}

type ArchiveDataSourceInput struct {
	TenantID        domain.TenantID
	DataSourceID    domain.DataSourceID
	DeleteDocuments bool
}

type ScanDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type PreflightDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type PlanDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type ReindexDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type RetryFailedDataSourceDocumentsInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type ExportDataSourceScanEntriesInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
	Outcome      domain.DataSourceScanOutcome
}

type DataSourceResult struct {
	Source               domain.DataSource
	DeletedDocumentCount int
}

type DataSourceDetailResult struct {
	Source          domain.DataSource
	Jobs            []domain.Job
	ScanEntries     []domain.DataSourceScanEntry
	ScanSummary     DataSourceScanSummary
	ScanPage        DataSourceScanEntryPage
	FailedDocuments int
}

type DataSourceScanSummary struct {
	Total       int
	Imported    int
	Skipped     int
	Failed      int
	Deleted     int
	LatestJobID domain.JobID
	LatestAt    time.Time
	Reasons     map[string]int
}

type DataSourceScanEntryPage struct {
	Total   int
	Limit   int
	Offset  int
	Outcome domain.DataSourceScanOutcome
}

type ScanDataSourceResult struct {
	Source domain.DataSource
	Job    domain.Job
}

type PreflightDataSourceResult struct {
	Source domain.DataSource
	Job    domain.Job
}

type PlanDataSourceResult struct {
	Source domain.DataSource
	Job    domain.Job
}

type ReindexDataSourceResult struct {
	Source               domain.DataSource
	Jobs                 []domain.Job
	SkippedDocumentCount int
}

type RetryFailedDataSourceDocumentsResult struct {
	Source               domain.DataSource
	Jobs                 []domain.Job
	SkippedDocumentCount int
}

type ListDataSourcesResult struct {
	Sources []domain.DataSource
}

type ExportDataSourceScanEntriesResult struct {
	Source    domain.DataSource
	Entries   []domain.DataSourceScanEntry
	Total     int
	Outcome   domain.DataSourceScanOutcome
	Truncated bool
}

func NewDataSourceService(repos store.RepositorySet, ids DataSourceIDs, clock Clock) DataSourceService {
	return DataSourceService{
		repos:     repos,
		ids:       ids,
		clock:     clock,
		documents: NewDocumentService(repos, ids, clock),
	}
}

const (
	defaultScanEntryListLimit = 100
	maxScanEntryListLimit     = 500
	maxScanEntryExportLimit   = 10000
)

func (s DataSourceService) WithObjectStore(objects providers.ObjectStore) DataSourceService {
	s.documents = s.documents.WithObjectStore(objects)
	return s
}

func (s DataSourceService) WithVectorIndex(vectors providers.VectorIndex) DataSourceService {
	s.documents = s.documents.WithVectorIndex(vectors)
	return s
}

func (s DataSourceService) Create(ctx context.Context, input CreateDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	source, err := domain.NewDataSource(domain.DataSourceCreate{
		ID:                  s.ids.NewDataSourceID(),
		TenantID:            input.TenantID,
		OwnerID:             input.OwnerID,
		Type:                input.Type,
		Name:                input.Name,
		RootPath:            input.RootPath,
		IncludePatterns:     input.IncludePatterns,
		ExcludePatterns:     input.ExcludePatterns,
		ScanIntervalMinutes: input.ScanIntervalMinutes,
		Now:                 s.clock.Now(),
	})
	if err != nil {
		return DataSourceResult{}, err
	}
	if err := s.repos.SaveDataSource(ctx, source); err != nil {
		return DataSourceResult{}, err
	}
	return DataSourceResult{Source: source}, nil
}

func (s DataSourceService) Update(ctx context.Context, input UpdateDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceResult{}, err
	}
	if err := source.Update(input.Name, input.Type, input.RootPath, input.IncludePatterns, input.ExcludePatterns, input.ScanIntervalMinutes, s.clock.Now()); err != nil {
		return DataSourceResult{}, err
	}
	if err := s.repos.SaveDataSource(ctx, source); err != nil {
		return DataSourceResult{}, err
	}
	return DataSourceResult{Source: source}, nil
}

func (s DataSourceService) Get(ctx context.Context, input DataSourceDetailInput) (DataSourceDetailResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceDetailResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return DataSourceDetailResult{}, fmt.Errorf("data source: %w", domain.ErrInvalidEntity)
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceDetailResult{}, err
	}
	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return DataSourceDetailResult{}, err
	}
	relatedJobs := jobs[:0]
	for _, job := range jobs {
		if job.ResourceType == "data_source" && job.ResourceID == string(source.ID) {
			relatedJobs = append(relatedJobs, job)
		}
	}
	scanEntryLimit := normalizeScanEntryLimit(input.ScanEntryLimit, defaultScanEntryListLimit, maxScanEntryListLimit)
	scanEntryOffset := input.ScanEntryOffset
	if scanEntryOffset < 0 {
		scanEntryOffset = 0
	}
	if input.ScanEntryOutcome != "" && !input.ScanEntryOutcome.Valid() {
		return DataSourceDetailResult{}, fmt.Errorf("data source scan outcome: %w", domain.ErrInvalidEntity)
	}
	page, err := s.repos.ListDataSourceScanEntryPage(ctx, input.TenantID, source.ID, store.DataSourceScanEntryFilter{
		Outcome: input.ScanEntryOutcome,
		Limit:   scanEntryLimit,
		Offset:  scanEntryOffset,
	})
	if err != nil {
		return DataSourceDetailResult{}, err
	}
	summaryEntries, err := s.repos.ListDataSourceScanEntries(ctx, input.TenantID, source.ID, 0)
	if err != nil {
		return DataSourceDetailResult{}, err
	}
	failedDocuments, err := s.failedDocumentCountForSource(ctx, source)
	if err != nil {
		return DataSourceDetailResult{}, err
	}
	return DataSourceDetailResult{
		Source:      source,
		Jobs:        relatedJobs,
		ScanEntries: page.Entries,
		ScanSummary: summarizeDataSourceScanEntries(summaryEntries),
		ScanPage: DataSourceScanEntryPage{
			Total:   page.Total,
			Limit:   page.Limit,
			Offset:  page.Offset,
			Outcome: input.ScanEntryOutcome,
		},
		FailedDocuments: failedDocuments,
	}, nil
}

func normalizeScanEntryLimit(value int, defaultLimit int, maxLimit int) int {
	if value <= 0 {
		return defaultLimit
	}
	if value > maxLimit {
		return maxLimit
	}
	return value
}

func summarizeDataSourceScanEntries(entries []domain.DataSourceScanEntry) DataSourceScanSummary {
	summary := DataSourceScanSummary{Reasons: map[string]int{}}
	if len(entries) == 0 {
		return summary
	}
	summary.LatestJobID = entries[0].JobID
	summary.LatestAt = entries[0].CreatedAt
	for _, entry := range entries {
		if entry.JobID != summary.LatestJobID {
			continue
		}
		summary.Total++
		if entry.CreatedAt.After(summary.LatestAt) {
			summary.LatestAt = entry.CreatedAt
		}
		switch entry.Outcome {
		case domain.DataSourceScanOutcomeImported:
			summary.Imported++
		case domain.DataSourceScanOutcomeSkipped:
			summary.Skipped++
		case domain.DataSourceScanOutcomeFailed:
			summary.Failed++
		case domain.DataSourceScanOutcomeDeleted:
			summary.Deleted++
		}
		if entry.Reason != "" {
			summary.Reasons[entry.Reason]++
		}
	}
	return summary
}

func (s DataSourceService) List(ctx context.Context, input ListDataSourcesInput) (ListDataSourcesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDataSourcesResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListDataSourcesResult{}, fmt.Errorf("data sources: %w", domain.ErrInvalidEntity)
	}
	sources, err := s.repos.ListDataSources(ctx, input.TenantID)
	if err != nil {
		return ListDataSourcesResult{}, err
	}
	if !input.IncludeArchived {
		sources = filterActiveDataSources(sources)
	}
	return ListDataSourcesResult{Sources: sources}, nil
}

func (s DataSourceService) ExportScanEntries(ctx context.Context, input ExportDataSourceScanEntriesInput) (ExportDataSourceScanEntriesResult, error) {
	if err := ctx.Err(); err != nil {
		return ExportDataSourceScanEntriesResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return ExportDataSourceScanEntriesResult{}, fmt.Errorf("export data source scan entries: %w", domain.ErrInvalidEntity)
	}
	if input.Outcome != "" && !input.Outcome.Valid() {
		return ExportDataSourceScanEntriesResult{}, fmt.Errorf("data source scan outcome: %w", domain.ErrInvalidEntity)
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return ExportDataSourceScanEntriesResult{}, err
	}
	page, err := s.repos.ListDataSourceScanEntryPage(ctx, input.TenantID, source.ID, store.DataSourceScanEntryFilter{
		Outcome: input.Outcome,
		Limit:   maxScanEntryExportLimit,
	})
	if err != nil {
		return ExportDataSourceScanEntriesResult{}, err
	}
	return ExportDataSourceScanEntriesResult{
		Source:    source,
		Entries:   page.Entries,
		Total:     page.Total,
		Outcome:   input.Outcome,
		Truncated: page.Total > len(page.Entries),
	}, nil
}

func (s DataSourceService) Archive(ctx context.Context, input ArchiveDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return DataSourceResult{}, fmt.Errorf("archive data source: %w", domain.ErrInvalidEntity)
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceResult{}, err
	}
	deletedDocuments := 0
	if input.DeleteDocuments {
		jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
		if err != nil {
			return DataSourceResult{}, err
		}
		for _, job := range jobs {
			if isActiveDataSourceScanJob(job, source.ID) {
				return DataSourceResult{}, fmt.Errorf("source %s has an active scan job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
			}
		}
		documentIDs, err := s.activeDocumentIDsForSource(ctx, source)
		if err != nil {
			return DataSourceResult{}, err
		}
		for _, documentID := range documentIDs {
			document, err := s.repos.GetDocument(ctx, source.TenantID, documentID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				return DataSourceResult{}, err
			}
			if document.Status == domain.DocumentStatusDeleted {
				continue
			}
			if _, err := s.documents.DeleteDocument(ctx, DeleteDocumentInput{
				TenantID:   source.TenantID,
				DocumentID: documentID,
			}); err != nil {
				return DataSourceResult{}, err
			}
			deletedDocuments++
		}
	}
	if source.Status != domain.DataSourceStatusArchived {
		if err := source.Transition(domain.DataSourceStatusArchived, s.clock.Now()); err != nil {
			return DataSourceResult{}, err
		}
		if err := s.repos.SaveDataSource(ctx, source); err != nil {
			return DataSourceResult{}, err
		}
	}
	return DataSourceResult{Source: source, DeletedDocumentCount: deletedDocuments}, nil
}

func (s DataSourceService) activeDocumentIDsForSource(ctx context.Context, source domain.DataSource) ([]domain.DocumentID, error) {
	entries, err := s.repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 0)
	if err != nil {
		return nil, err
	}
	latestSeen := map[string]bool{}
	uniqueDocuments := map[domain.DocumentID]bool{}
	documentIDs := make([]domain.DocumentID, 0)
	for _, entry := range entries {
		if latestSeen[entry.Path] {
			continue
		}
		latestSeen[entry.Path] = true
		if !isActiveSourceDocumentEntry(entry) || uniqueDocuments[entry.DocumentID] {
			continue
		}
		uniqueDocuments[entry.DocumentID] = true
		documentIDs = append(documentIDs, entry.DocumentID)
	}
	return documentIDs, nil
}

func isActiveSourceDocumentEntry(entry domain.DataSourceScanEntry) bool {
	if entry.DocumentID == "" {
		return false
	}
	if entry.Outcome == domain.DataSourceScanOutcomeImported {
		return true
	}
	return entry.Outcome == domain.DataSourceScanOutcomeSkipped && entry.Reason == "unchanged"
}

func (s DataSourceService) RequestScan(ctx context.Context, input ScanDataSourceInput) (ScanDataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanDataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return ScanDataSourceResult{}, fmt.Errorf("scan data source: %w", domain.ErrInvalidEntity)
	}

	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return ScanDataSourceResult{}, err
	}
	if source.Status == domain.DataSourceStatusArchived {
		return ScanDataSourceResult{}, fmt.Errorf("archived data source %s cannot be scanned: %w", source.ID, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return ScanDataSourceResult{}, err
	}
	for _, job := range jobs {
		if isActiveDataSourceScanJob(job, source.ID) {
			return ScanDataSourceResult{}, fmt.Errorf("scan is already queued or running for data source %s with job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
	}
	if job, ok := latestRelevantDataSourceJob(jobs, source, domain.JobTypeSourcePreflight); ok {
		if isActiveJobState(job.State) {
			return ScanDataSourceResult{}, fmt.Errorf("path check is still queued or running for data source %s with job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
		if job.State == domain.JobStateFailed {
			return ScanDataSourceResult{}, fmt.Errorf("path check failed for data source %s: %s: %w", source.ID, job.ErrorMessage, domain.ErrInvalidStateTransition)
		}
	}

	job, err := domain.NewJob(domain.JobCreate{
		ID:           s.ids.NewJobID(),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourceScan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          s.clock.Now(),
	})
	if err != nil {
		return ScanDataSourceResult{}, err
	}
	if err := s.repos.SaveJob(ctx, job); err != nil {
		return ScanDataSourceResult{}, err
	}

	return ScanDataSourceResult{
		Source: source,
		Job:    job,
	}, nil
}

func (s DataSourceService) RequestPreflight(ctx context.Context, input PreflightDataSourceInput) (PreflightDataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return PreflightDataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return PreflightDataSourceResult{}, fmt.Errorf("preflight data source: %w", domain.ErrInvalidEntity)
	}

	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return PreflightDataSourceResult{}, err
	}
	if source.Status == domain.DataSourceStatusArchived {
		return PreflightDataSourceResult{}, fmt.Errorf("archived data source %s cannot be checked: %w", source.ID, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return PreflightDataSourceResult{}, err
	}
	for _, job := range jobs {
		if isActiveDataSourceJob(job, source.ID, domain.JobTypeSourcePreflight) {
			return PreflightDataSourceResult{}, fmt.Errorf("path check is already queued or running for data source %s with job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
	}

	job, err := domain.NewJob(domain.JobCreate{
		ID:           s.ids.NewJobID(),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePreflight,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          s.clock.Now(),
	})
	if err != nil {
		return PreflightDataSourceResult{}, err
	}
	if err := s.repos.SaveJob(ctx, job); err != nil {
		return PreflightDataSourceResult{}, err
	}

	return PreflightDataSourceResult{
		Source: source,
		Job:    job,
	}, nil
}

func (s DataSourceService) RequestPlan(ctx context.Context, input PlanDataSourceInput) (PlanDataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return PlanDataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return PlanDataSourceResult{}, fmt.Errorf("plan data source: %w", domain.ErrInvalidEntity)
	}

	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return PlanDataSourceResult{}, err
	}
	if source.Status == domain.DataSourceStatusArchived {
		return PlanDataSourceResult{}, fmt.Errorf("archived data source %s cannot be planned: %w", source.ID, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return PlanDataSourceResult{}, err
	}
	for _, job := range jobs {
		switch {
		case isActiveDataSourceJob(job, source.ID, domain.JobTypeSourcePlan):
			return PlanDataSourceResult{}, fmt.Errorf("plan is already queued or running for data source %s with job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		case isActiveDataSourceJob(job, source.ID, domain.JobTypeSourceScan):
			return PlanDataSourceResult{}, fmt.Errorf("source %s has an active scan job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		case isActiveDataSourceJob(job, source.ID, domain.JobTypeSourcePreflight):
			return PlanDataSourceResult{}, fmt.Errorf("source %s has an active path check job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
	}

	job, err := domain.NewJob(domain.JobCreate{
		ID:           s.ids.NewJobID(),
		TenantID:     source.TenantID,
		Type:         domain.JobTypeSourcePlan,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Now:          s.clock.Now(),
	})
	if err != nil {
		return PlanDataSourceResult{}, err
	}
	if err := s.repos.SaveJob(ctx, job); err != nil {
		return PlanDataSourceResult{}, err
	}

	return PlanDataSourceResult{
		Source: source,
		Job:    job,
	}, nil
}

func (s DataSourceService) RequestReindex(ctx context.Context, input ReindexDataSourceInput) (ReindexDataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return ReindexDataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return ReindexDataSourceResult{}, fmt.Errorf("reindex data source: %w", domain.ErrInvalidEntity)
	}

	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return ReindexDataSourceResult{}, err
	}
	if source.Status == domain.DataSourceStatusArchived {
		return ReindexDataSourceResult{}, fmt.Errorf("archived data source %s cannot be reindexed: %w", source.ID, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return ReindexDataSourceResult{}, err
	}
	for _, job := range jobs {
		if isActiveDataSourceScanJob(job, source.ID) {
			return ReindexDataSourceResult{}, fmt.Errorf("source %s has an active scan job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
	}

	documentIDs, err := s.activeDocumentIDsForSource(ctx, source)
	if err != nil {
		return ReindexDataSourceResult{}, err
	}
	queued := make([]domain.Job, 0, len(documentIDs))
	skipped := 0
	for _, documentID := range documentIDs {
		document, err := s.repos.GetDocument(ctx, source.TenantID, documentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				skipped++
				continue
			}
			return ReindexDataSourceResult{}, err
		}
		if document.Status == domain.DocumentStatusDeleted {
			skipped++
			continue
		}
		if hasActiveDocumentIngestionJob(jobs, document.ID) {
			skipped++
			continue
		}

		job, err := domain.NewJob(domain.JobCreate{
			ID:           s.ids.NewJobID(),
			TenantID:     document.TenantID,
			Type:         domain.JobTypeDocumentIngestion,
			ResourceType: "document",
			ResourceID:   string(document.ID),
			Now:          s.clock.Now(),
		})
		if err != nil {
			return ReindexDataSourceResult{}, err
		}
		if err := s.repos.SaveJob(ctx, job); err != nil {
			return ReindexDataSourceResult{}, err
		}
		queued = append(queued, job)
	}

	return ReindexDataSourceResult{
		Source:               source,
		Jobs:                 queued,
		SkippedDocumentCount: skipped,
	}, nil
}

func (s DataSourceService) RequestRetryFailedDocuments(ctx context.Context, input RetryFailedDataSourceDocumentsInput) (RetryFailedDataSourceDocumentsResult, error) {
	if err := ctx.Err(); err != nil {
		return RetryFailedDataSourceDocumentsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return RetryFailedDataSourceDocumentsResult{}, fmt.Errorf("retry failed data source documents: %w", domain.ErrInvalidEntity)
	}

	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return RetryFailedDataSourceDocumentsResult{}, err
	}
	if source.Status == domain.DataSourceStatusArchived {
		return RetryFailedDataSourceDocumentsResult{}, fmt.Errorf("archived data source %s cannot retry failed documents: %w", source.ID, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return RetryFailedDataSourceDocumentsResult{}, err
	}
	for _, job := range jobs {
		if isActiveDataSourceScanJob(job, source.ID) {
			return RetryFailedDataSourceDocumentsResult{}, fmt.Errorf("source %s has an active scan job %s: %w", source.ID, job.ID, domain.ErrInvalidStateTransition)
		}
	}

	documentIDs, err := s.activeDocumentIDsForSource(ctx, source)
	if err != nil {
		return RetryFailedDataSourceDocumentsResult{}, err
	}
	queued := make([]domain.Job, 0)
	skipped := 0
	for _, documentID := range documentIDs {
		document, err := s.repos.GetDocument(ctx, source.TenantID, documentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				skipped++
				continue
			}
			return RetryFailedDataSourceDocumentsResult{}, err
		}
		if document.Status != domain.DocumentStatusFailed {
			skipped++
			continue
		}
		if hasActiveDocumentIngestionJob(jobs, document.ID) {
			skipped++
			continue
		}

		job, err := domain.NewJob(domain.JobCreate{
			ID:           s.ids.NewJobID(),
			TenantID:     document.TenantID,
			Type:         domain.JobTypeDocumentIngestion,
			ResourceType: "document",
			ResourceID:   string(document.ID),
			Now:          s.clock.Now(),
		})
		if err != nil {
			return RetryFailedDataSourceDocumentsResult{}, err
		}
		if err := s.repos.SaveJob(ctx, job); err != nil {
			return RetryFailedDataSourceDocumentsResult{}, err
		}
		queued = append(queued, job)
	}

	return RetryFailedDataSourceDocumentsResult{
		Source:               source,
		Jobs:                 queued,
		SkippedDocumentCount: skipped,
	}, nil
}

func (s DataSourceService) failedDocumentCountForSource(ctx context.Context, source domain.DataSource) (int, error) {
	documentIDs, err := s.activeDocumentIDsForSource(ctx, source)
	if err != nil {
		return 0, err
	}
	failed := 0
	for _, documentID := range documentIDs {
		document, err := s.repos.GetDocument(ctx, source.TenantID, documentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return 0, err
		}
		if document.Status == domain.DocumentStatusFailed {
			failed++
		}
	}
	return failed, nil
}

func filterActiveDataSources(sources []domain.DataSource) []domain.DataSource {
	active := sources[:0]
	for _, source := range sources {
		if source.Status != domain.DataSourceStatusArchived {
			active = append(active, source)
		}
	}
	return active
}

func isActiveDataSourceScanJob(job domain.Job, sourceID domain.DataSourceID) bool {
	return isActiveDataSourceJob(job, sourceID, domain.JobTypeSourceScan)
}

func latestRelevantDataSourceJob(jobs []domain.Job, source domain.DataSource, jobType domain.JobType) (domain.Job, bool) {
	var latest domain.Job
	found := false
	for _, job := range jobs {
		if job.Type != jobType || job.ResourceType != "data_source" || job.ResourceID != string(source.ID) {
			continue
		}
		if job.UpdatedAt.Before(source.UpdatedAt) {
			continue
		}
		if !found || job.UpdatedAt.After(latest.UpdatedAt) {
			latest = job
			found = true
		}
	}
	return latest, found
}

func isActiveDataSourceJob(job domain.Job, sourceID domain.DataSourceID, jobType domain.JobType) bool {
	if job.Type != jobType || job.ResourceType != "data_source" || job.ResourceID != string(sourceID) {
		return false
	}
	return isActiveJobState(job.State)
}

func isActiveJobState(state domain.JobState) bool {
	return state == domain.JobStateQueued || state == domain.JobStateRunning || state == domain.JobStateRetrying
}

func hasActiveDocumentIngestionJob(jobs []domain.Job, documentID domain.DocumentID) bool {
	for _, job := range jobs {
		if isActiveDocumentIngestionJob(job, documentID) {
			return true
		}
	}
	return false
}
