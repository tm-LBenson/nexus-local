package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

var ErrObjectStoreUnavailable = errors.New("object store is not configured")

type DocumentIDs interface {
	NewDocumentID() domain.DocumentID
	NewJobID() domain.JobID
}

type Clock interface {
	Now() time.Time
}

type DocumentService struct {
	repos   store.RepositorySet
	ids     DocumentIDs
	clock   Clock
	objects providers.ObjectStore
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

type UploadDocumentInput struct {
	TenantID    domain.TenantID
	OwnerID     domain.UserID
	Name        string
	ContentType string
	SizeBytes   int64
	Body        io.Reader
}

func NewDocumentService(repos store.RepositorySet, ids DocumentIDs, clock Clock) DocumentService {
	return DocumentService{
		repos: repos,
		ids:   ids,
		clock: clock,
	}
}

func (s DocumentService) WithObjectStore(objects providers.ObjectStore) DocumentService {
	s.objects = objects
	return s
}

func (s DocumentService) RegisterDocument(ctx context.Context, input RegisterDocumentInput) (RegisterDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return RegisterDocumentResult{}, err
	}

	now := s.clock.Now()
	return s.registerDocument(ctx, input, s.ids.NewDocumentID(), s.ids.NewJobID(), now)
}

func (s DocumentService) UploadDocument(ctx context.Context, input UploadDocumentInput) (RegisterDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return RegisterDocumentResult{}, err
	}
	if s.objects == nil {
		return RegisterDocumentResult{}, ErrObjectStoreUnavailable
	}

	documentID := s.ids.NewDocumentID()
	jobID := s.ids.NewJobID()
	storageKey := documentStorageKey(input.TenantID, documentID, input.Name)
	info, err := s.objects.PutObject(ctx, providers.ObjectPut{
		TenantID:    input.TenantID,
		Key:         storageKey,
		Body:        input.Body,
		ContentType: input.ContentType,
		SizeBytes:   input.SizeBytes,
	})
	if err != nil {
		return RegisterDocumentResult{}, err
	}

	return s.registerDocument(ctx, RegisterDocumentInput{
		TenantID:   input.TenantID,
		OwnerID:    input.OwnerID,
		Name:       input.Name,
		StorageKey: info.Key,
		SizeBytes:  info.SizeBytes,
	}, documentID, jobID, s.clock.Now())
}

func (s DocumentService) registerDocument(ctx context.Context, input RegisterDocumentInput, documentID domain.DocumentID, jobID domain.JobID, now time.Time) (RegisterDocumentResult, error) {
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         documentID,
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
		ID:       jobID,
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

func documentStorageKey(tenantID domain.TenantID, documentID domain.DocumentID, name string) string {
	return fmt.Sprintf("tenants/%s/documents/%s/%s", tenantID, documentID, safeFileName(name))
}

func safeFileName(name string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	base := path.Base(normalized)
	if base == "." || base == "/" || base == "" {
		return "upload.bin"
	}
	return base
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
