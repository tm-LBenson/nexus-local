package app

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestListJobsReturnsRecentTenantJobs(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewJobService(repos)

	first := newTestJob(t, domain.TenantID("tenant_1"), domain.JobID("job_1"), fixedClock{}.Now())
	second := newTestJob(t, domain.TenantID("tenant_1"), domain.JobID("job_2"), fixedClock{}.Now().Add(time.Minute))
	otherTenant := newTestJob(t, domain.TenantID("tenant_2"), domain.JobID("job_3"), fixedClock{}.Now().Add(2*time.Minute))
	for _, job := range []domain.Job{first, second, otherTenant} {
		if err := repos.SaveJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", job.ID, err)
		}
	}

	result, err := service.ListJobs(ctx, ListJobsInput{
		TenantID: domain.TenantID("tenant_1"),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(result.Jobs))
	}
	if result.Jobs[0].ID != domain.JobID("job_2") || result.Jobs[1].ID != domain.JobID("job_1") {
		t.Fatalf("job order = %s, %s; want job_2, job_1", result.Jobs[0].ID, result.Jobs[1].ID)
	}
}

func TestListJobsRejectsInvalidInput(t *testing.T) {
	service := NewJobService(memory.New())

	_, err := service.ListJobs(context.Background(), ListJobsInput{})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}

func TestNormalizeJobLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default", limit: 0, want: defaultJobListLimit},
		{name: "negative", limit: -5, want: defaultJobListLimit},
		{name: "custom", limit: 7, want: 7},
		{name: "max", limit: maxJobListLimit + 1, want: maxJobListLimit},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeJobLimit(tc.limit); got != tc.want {
				t.Fatalf("limit = %d, want %d", got, tc.want)
			}
		})
	}
}

func newTestJob(t *testing.T, tenantID domain.TenantID, jobID domain.JobID, now time.Time) domain.Job {
	t.Helper()

	job, err := domain.NewJob(domain.JobCreate{
		ID:           jobID,
		TenantID:     tenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   "doc_1",
		Now:          now,
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}
