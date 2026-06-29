package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourceScheduler struct {
	repos store.RepositorySet
	ids   SourceScanIDs
	clock Clock
}

type SourceScheduleResult struct {
	QueuedCount        int
	SkippedActiveCount int
}

func NewSourceScheduler(repos store.RepositorySet, ids SourceScanIDs, clock Clock) SourceScheduler {
	return SourceScheduler{repos: repos, ids: ids, clock: clock}
}

func (s SourceScheduler) QueueDueScans(ctx context.Context, limit int) (SourceScheduleResult, error) {
	if limit <= 0 {
		limit = 25
	}
	now := s.clock.Now()
	sources, err := s.repos.ListDueDataSources(ctx, now, limit)
	if err != nil {
		return SourceScheduleResult{}, err
	}
	result := SourceScheduleResult{}
	for _, source := range sources {
		latest, err := s.repos.GetDataSource(ctx, source.TenantID, source.ID)
		if err != nil {
			return result, err
		}
		if !dataSourceDueForScan(latest, now) {
			continue
		}
		jobs, err := s.repos.ListJobs(ctx, latest.TenantID, 100)
		if err != nil {
			return result, err
		}
		if hasActiveSourceScanJob(jobs, latest.ID) {
			result.SkippedActiveCount++
			continue
		}
		job, err := domain.NewJob(domain.JobCreate{
			ID:           s.ids.NewJobID(),
			TenantID:     latest.TenantID,
			Type:         domain.JobTypeSourceScan,
			ResourceType: "data_source",
			ResourceID:   string(latest.ID),
			Now:          now,
		})
		if err != nil {
			return result, err
		}
		if err := s.repos.SaveJob(ctx, job); err != nil {
			return result, err
		}
		latest.ScheduleNextScan(now)
		if err := s.repos.SaveDataSource(ctx, latest); err != nil {
			return result, fmt.Errorf("advance source schedule %s: %w", latest.ID, err)
		}
		result.QueuedCount++
	}
	return result, nil
}

func dataSourceDueForScan(source domain.DataSource, now time.Time) bool {
	return source.ScanIntervalMinutes > 0 &&
		source.NextScanAt != nil &&
		!source.NextScanAt.After(now) &&
		source.Status != domain.DataSourceStatusArchived &&
		source.Status != domain.DataSourceStatusScanning
}

func hasActiveSourceScanJob(jobs []domain.Job, sourceID domain.DataSourceID) bool {
	for _, job := range jobs {
		if job.Type != domain.JobTypeSourceScan ||
			job.ResourceType != "data_source" ||
			job.ResourceID != string(sourceID) {
			continue
		}
		if job.State == domain.JobStateQueued ||
			job.State == domain.JobStateRunning ||
			job.State == domain.JobStateRetrying {
			return true
		}
	}
	return false
}
