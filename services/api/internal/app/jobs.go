package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

const (
	defaultJobListLimit = 25
	maxJobListLimit     = 100
)

type JobService struct {
	repos store.RepositorySet
}

type ListJobsInput struct {
	TenantID domain.TenantID
	Limit    int
}

type ListJobsResult struct {
	Jobs []domain.Job
}

func NewJobService(repos store.RepositorySet) JobService {
	return JobService{repos: repos}
}

func (s JobService) ListJobs(ctx context.Context, input ListJobsInput) (ListJobsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListJobsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListJobsResult{}, fmt.Errorf("jobs: %w", domain.ErrInvalidEntity)
	}
	jobs, err := s.repos.ListJobs(ctx, input.TenantID, normalizeJobLimit(input.Limit))
	if err != nil {
		return ListJobsResult{}, err
	}
	return ListJobsResult{Jobs: jobs}, nil
}

func normalizeJobLimit(limit int) int {
	if limit <= 0 {
		return defaultJobListLimit
	}
	if limit > maxJobListLimit {
		return maxJobListLimit
	}
	return limit
}
