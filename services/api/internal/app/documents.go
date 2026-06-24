package app

import (
	"context"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type DocumentIDs interface {
	NewDocumentID() domain.DocumentID
	NewJobID() domain.JobID
}

type Clock interface {
	Now() time.Time
}

type DocumentService struct {
	repos store.RepositorySet
	ids   DocumentIDs
	clock Clock
}

type RegisterDocumentInput struct {
	TenantID   domain.TenantID
	OwnerID    domain.UserID
	Name       string
	StorageKey string
	SizeBytes  int64
}

type RegisterDocumentResult struct {
	Document domain.Document
	Job      domain.Job
}

func NewDocumentService(repos store.RepositorySet, ids DocumentIDs, clock Clock) DocumentService {
	return DocumentService{
		repos: repos,
		ids:   ids,
		clock: clock,
	}
}

func (s DocumentService) RegisterDocument(ctx context.Context, input RegisterDocumentInput) (RegisterDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return RegisterDocumentResult{}, err
	}

	now := s.clock.Now()
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         s.ids.NewDocumentID(),
		TenantID:   input.TenantID,
		OwnerID:    input.OwnerID,
		Name:       input.Name,
		StorageKey: input.StorageKey,
		SizeBytes:  input.SizeBytes,
		Now:        now,
	})
	if err != nil {
		return RegisterDocumentResult{}, err
	}

	job, err := domain.NewJob(domain.JobCreate{
		ID:       s.ids.NewJobID(),
		TenantID: input.TenantID,
		Type:     domain.JobTypeDocumentIngestion,
		Now:      now,
	})
	if err != nil {
		return RegisterDocumentResult{}, err
	}

	if err := s.repos.SaveDocument(ctx, document); err != nil {
		return RegisterDocumentResult{}, err
	}
	if err := s.repos.SaveJob(ctx, job); err != nil {
		return RegisterDocumentResult{}, err
	}

	return RegisterDocumentResult{
		Document: document,
		Job:      job,
	}, nil
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
