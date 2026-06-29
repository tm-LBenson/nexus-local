package worker

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourceSchedulerQueuesDueScans(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, t.TempDir(), domain.DataSourceStatusActive)
	source.ScanIntervalMinutes = 60
	next := fixedTime().Add(-time.Minute)
	source.NextScanAt = &next
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	scheduler := NewSourceScheduler(repos, &scanIDs{}, fixedClock{})
	result, err := scheduler.QueueDueScans(ctx, 10)
	if err != nil {
		t.Fatalf("queue due scans: %v", err)
	}
	if result.QueuedCount != 1 || result.SkippedActiveCount != 0 {
		t.Fatalf("result = %#v, want one queued", result)
	}

	jobs, err := repos.ListJobs(ctx, source.TenantID, 10)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 ||
		jobs[0].Type != domain.JobTypeSourceScan ||
		jobs[0].ResourceType != "data_source" ||
		jobs[0].ResourceID != string(source.ID) {
		t.Fatalf("jobs = %#v, want one source scan", jobs)
	}
	updated, err := repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	wantNext := fixedTime().Add(time.Hour)
	if updated.NextScanAt == nil || !updated.NextScanAt.Equal(wantNext) {
		t.Fatalf("next scan = %v, want %s", updated.NextScanAt, wantNext)
	}
}

func TestSourceSchedulerSkipsSourcesWithActiveScan(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	source := newTestDataSource(t, t.TempDir(), domain.DataSourceStatusActive)
	source.ScanIntervalMinutes = 60
	next := fixedTime().Add(-time.Minute)
	source.NextScanAt = &next
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	job := newSourceScanJobWithID(t, source, domain.JobID("job_existing_scan"))
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	scheduler := NewSourceScheduler(repos, &scanIDs{}, fixedClock{})
	result, err := scheduler.QueueDueScans(ctx, 10)
	if err != nil {
		t.Fatalf("queue due scans: %v", err)
	}
	if result.QueuedCount != 0 || result.SkippedActiveCount != 1 {
		t.Fatalf("result = %#v, want one skipped active", result)
	}
	jobs, err := repos.ListJobs(ctx, source.TenantID, 10)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs len = %d, want existing job only", len(jobs))
	}
}
