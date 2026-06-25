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
	vectors providers.VectorIndex
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

type ListDocumentsInput struct {
	TenantID domain.TenantID
}

type ListDocumentsResult struct {
	Documents []domain.Document
}

type DocumentDetailInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
}

type DocumentDetailResult struct {
	Document domain.Document
	Jobs     []domain.Job
}

type DownloadDocumentInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
}

type DownloadDocumentResult struct {
	Document domain.Document
	Object   providers.ObjectInfo
	Body     io.ReadCloser
}

type DeleteDocumentInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
}

type DeleteDocumentResult struct {
	Document domain.Document
}

type RetryDocumentInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
}

type RetryDocumentResult struct {
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

func (s DocumentService) WithObjectStore(objects providers.ObjectStore) DocumentService {
	s.objects = objects
	return s
}

func (s DocumentService) WithVectorIndex(vectors providers.VectorIndex) DocumentService {
	s.vectors = vectors
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

func (s DocumentService) ListDocuments(ctx context.Context, input ListDocumentsInput) (ListDocumentsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDocumentsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListDocumentsResult{}, fmt.Errorf("documents: %w", domain.ErrInvalidEntity)
	}
	documents, err := s.repos.ListDocuments(ctx, input.TenantID)
	if err != nil {
		return ListDocumentsResult{}, err
	}
	documents = filterActiveDocuments(documents)
	return ListDocumentsResult{Documents: documents}, nil
}

func (s DocumentService) GetDocumentDetail(ctx context.Context, input DocumentDetailInput) (DocumentDetailResult, error) {
	if err := ctx.Err(); err != nil {
		return DocumentDetailResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DocumentID)) == "" {
		return DocumentDetailResult{}, fmt.Errorf("document detail: %w", domain.ErrInvalidEntity)
	}

	document, err := s.repos.GetDocument(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return DocumentDetailResult{}, err
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return DocumentDetailResult{}, err
	}

	relatedJobs := jobs[:0]
	for _, job := range jobs {
		if job.ResourceType == "document" && job.ResourceID == string(document.ID) {
			relatedJobs = append(relatedJobs, job)
		}
	}

	return DocumentDetailResult{
		Document: document,
		Jobs:     relatedJobs,
	}, nil
}

func (s DocumentService) DownloadDocument(ctx context.Context, input DownloadDocumentInput) (DownloadDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return DownloadDocumentResult{}, err
	}
	if s.objects == nil {
		return DownloadDocumentResult{}, ErrObjectStoreUnavailable
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DocumentID)) == "" {
		return DownloadDocumentResult{}, fmt.Errorf("download document: %w", domain.ErrInvalidEntity)
	}

	document, err := s.repos.GetDocument(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return DownloadDocumentResult{}, err
	}
	if document.Status == domain.DocumentStatusDeleted {
		return DownloadDocumentResult{}, store.ErrNotFound
	}

	body, info, err := s.objects.GetObject(ctx, document.TenantID, document.StorageKey)
	if err != nil {
		return DownloadDocumentResult{}, err
	}

	return DownloadDocumentResult{
		Document: document,
		Object:   info,
		Body:     body,
	}, nil
}

func (s DocumentService) RetryDocumentIngestion(ctx context.Context, input RetryDocumentInput) (RetryDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return RetryDocumentResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DocumentID)) == "" {
		return RetryDocumentResult{}, fmt.Errorf("retry document: %w", domain.ErrInvalidEntity)
	}

	document, err := s.repos.GetDocument(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return RetryDocumentResult{}, err
	}
	if document.Status != domain.DocumentStatusFailed {
		return RetryDocumentResult{}, fmt.Errorf("retry document %s from %s: %w", document.ID, document.Status, domain.ErrInvalidStateTransition)
	}

	jobs, err := s.repos.ListJobs(ctx, input.TenantID, maxJobListLimit)
	if err != nil {
		return RetryDocumentResult{}, err
	}
	for _, job := range jobs {
		if isActiveDocumentIngestionJob(job, document.ID) {
			return RetryDocumentResult{}, fmt.Errorf("retry document %s with active job %s: %w", document.ID, job.ID, domain.ErrInvalidStateTransition)
		}
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
		return RetryDocumentResult{}, err
	}
	if err := s.repos.SaveJob(ctx, job); err != nil {
		return RetryDocumentResult{}, err
	}

	return RetryDocumentResult{
		Document: document,
		Job:      job,
	}, nil
}

func (s DocumentService) DeleteDocument(ctx context.Context, input DeleteDocumentInput) (DeleteDocumentResult, error) {
	if err := ctx.Err(); err != nil {
		return DeleteDocumentResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DocumentID)) == "" {
		return DeleteDocumentResult{}, fmt.Errorf("delete document: %w", domain.ErrInvalidEntity)
	}

	document, err := s.repos.GetDocument(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return DeleteDocumentResult{}, err
	}
	if document.Status == domain.DocumentStatusDeleted {
		return DeleteDocumentResult{Document: document}, nil
	}

	if s.vectors != nil {
		if err := s.vectors.DeleteDocument(ctx, document.TenantID, document.ID); err != nil {
			return DeleteDocumentResult{}, err
		}
	}
	if s.objects != nil {
		if err := s.objects.DeleteObject(ctx, document.TenantID, document.StorageKey); err != nil {
			return DeleteDocumentResult{}, err
		}
	}
	if err := document.Transition(domain.DocumentStatusDeleted, s.clock.Now()); err != nil {
		return DeleteDocumentResult{}, err
	}
	if err := s.repos.SaveDocument(ctx, document); err != nil {
		return DeleteDocumentResult{}, err
	}
	return DeleteDocumentResult{Document: document}, nil
}

func isActiveDocumentIngestionJob(job domain.Job, documentID domain.DocumentID) bool {
	if job.Type != domain.JobTypeDocumentIngestion || job.ResourceType != "document" || job.ResourceID != string(documentID) {
		return false
	}
	return job.State == domain.JobStateQueued || job.State == domain.JobStateRunning || job.State == domain.JobStateRetrying
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
		ID:           jobID,
		TenantID:     input.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(document.ID),
		Now:          now,
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

func filterActiveDocuments(documents []domain.Document) []domain.Document {
	active := documents[:0]
	for _, document := range documents {
		if document.Status != domain.DocumentStatusDeleted {
			active = append(active, document)
		}
	}
	return active
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
