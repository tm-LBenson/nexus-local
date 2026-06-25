package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/ingest"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestRegisterDocumentCreatesDocumentAndIngestionJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})

	result, err := service.RegisterDocument(ctx, RegisterDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/source.md",
		SizeBytes:  42,
	})
	if err != nil {
		t.Fatalf("register document: %v", err)
	}

	if result.Document.ID != domain.DocumentID("doc_fixed") {
		t.Fatalf("document id = %q, want doc_fixed", result.Document.ID)
	}
	if result.Job.Type != domain.JobTypeDocumentIngestion {
		t.Fatalf("job type = %q, want document ingestion", result.Job.Type)
	}
	if result.Job.State != domain.JobStateQueued {
		t.Fatalf("job state = %q, want queued", result.Job.State)
	}
	if result.Job.ResourceType != "document" || result.Job.ResourceID != "doc_fixed" {
		t.Fatalf("job resource = %s/%s, want document/doc_fixed", result.Job.ResourceType, result.Job.ResourceID)
	}

	savedDoc, err := repos.GetDocument(ctx, domain.TenantID("tenant_1"), domain.DocumentID("doc_fixed"))
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if savedDoc.StorageKey != "tenants/tenant_1/documents/source.md" {
		t.Fatalf("storage key = %q", savedDoc.StorageKey)
	}

	savedJob, err := repos.GetJob(ctx, domain.TenantID("tenant_1"), domain.JobID("job_fixed"))
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if savedJob.State != domain.JobStateQueued {
		t.Fatalf("saved job state = %q, want queued", savedJob.State)
	}
	if savedJob.ResourceID != "doc_fixed" {
		t.Fatalf("saved job resource id = %q, want doc_fixed", savedJob.ResourceID)
	}
}

func TestRegisterDocumentRejectsInvalidInput(t *testing.T) {
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.RegisterDocument(context.Background(), RegisterDocumentInput{
		TenantID:  domain.TenantID("tenant_1"),
		OwnerID:   domain.UserID("user_1"),
		Name:      "Handbook.md",
		SizeBytes: 42,
	})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}

func TestRegisterDocumentRejectsUnsupportedType(t *testing.T) {
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.RegisterDocument(context.Background(), RegisterDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "photo.png",
		StorageKey: "tenants/tenant_1/documents/photo.png",
		SizeBytes:  42,
	})
	if !errors.Is(err, ingest.ErrUnsupportedDocumentType) {
		t.Fatalf("err = %v, want unsupported document type", err)
	}
}

func TestUploadDocumentStoresObjectAndRegistersDocument(t *testing.T) {
	ctx := context.Background()
	objects := objectmemory.New()
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{}).WithObjectStore(objects)

	result, err := service.UploadDocument(ctx, UploadDocumentInput{
		TenantID:    domain.TenantID("tenant_1"),
		OwnerID:     domain.UserID("user_1"),
		Name:        "../Handbook.md",
		ContentType: "text/markdown",
		SizeBytes:   11,
		Body:        strings.NewReader("hello world"),
	})
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}

	wantKey := "tenants/tenant_1/documents/doc_fixed/Handbook.md"
	if result.Document.StorageKey != wantKey {
		t.Fatalf("storage key = %q, want %q", result.Document.StorageKey, wantKey)
	}
	if result.Document.SizeBytes != 11 {
		t.Fatalf("size = %d, want 11", result.Document.SizeBytes)
	}
	if keys := objects.Keys(); len(keys) != 1 || keys[0] != wantKey {
		t.Fatalf("keys = %v, want [%s]", keys, wantKey)
	}
}

func TestUploadDocumentRejectsUnsupportedTypeBeforeStoringObject(t *testing.T) {
	ctx := context.Background()
	objects := objectmemory.New()
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{}).WithObjectStore(objects)

	_, err := service.UploadDocument(ctx, UploadDocumentInput{
		TenantID:    domain.TenantID("tenant_1"),
		OwnerID:     domain.UserID("user_1"),
		Name:        "photo.png",
		ContentType: "image/png",
		SizeBytes:   11,
		Body:        strings.NewReader("not really a png"),
	})
	if !errors.Is(err, ingest.ErrUnsupportedDocumentType) {
		t.Fatalf("err = %v, want unsupported document type", err)
	}
	if keys := objects.Keys(); len(keys) != 0 {
		t.Fatalf("keys = %v, want none", keys)
	}
}

func TestUploadDocumentRequiresObjectStore(t *testing.T) {
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.UploadDocument(context.Background(), UploadDocumentInput{
		TenantID:  domain.TenantID("tenant_1"),
		OwnerID:   domain.UserID("user_1"),
		Name:      "Handbook.md",
		SizeBytes: 11,
		Body:      strings.NewReader("hello world"),
	})
	if err == nil {
		t.Fatal("err = nil, want object store error")
	}
}

func TestDownloadDocumentReturnsStoredObject(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{}).WithObjectStore(objects)

	uploaded, err := service.UploadDocument(ctx, UploadDocumentInput{
		TenantID:    domain.TenantID("tenant_1"),
		OwnerID:     domain.UserID("user_1"),
		Name:        "Handbook.md",
		ContentType: "text/markdown",
		SizeBytes:   11,
		Body:        strings.NewReader("hello world"),
	})
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}

	downloaded, err := service.DownloadDocument(ctx, DownloadDocumentInput{
		TenantID:   uploaded.Document.TenantID,
		DocumentID: uploaded.Document.ID,
	})
	if err != nil {
		t.Fatalf("download document: %v", err)
	}
	defer downloaded.Body.Close()

	content, err := io.ReadAll(downloaded.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("content = %q, want hello world", content)
	}
	if downloaded.Document.ID != uploaded.Document.ID {
		t.Fatalf("document id = %q, want %q", downloaded.Document.ID, uploaded.Document.ID)
	}
	if downloaded.Object.ContentType != "text/markdown" {
		t.Fatalf("content type = %q, want text/markdown", downloaded.Object.ContentType)
	}
}

func TestDownloadDocumentRequiresObjectStore(t *testing.T) {
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.DownloadDocument(context.Background(), DownloadDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		DocumentID: domain.DocumentID("doc_1"),
	})
	if !errors.Is(err, ErrObjectStoreUnavailable) {
		t.Fatalf("err = %v, want object store unavailable", err)
	}
}

func TestListDocumentsReturnsTenantDocuments(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})

	if _, err := service.RegisterDocument(ctx, RegisterDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/source.md",
		SizeBytes:  42,
	}); err != nil {
		t.Fatalf("register document: %v", err)
	}

	result, err := service.ListDocuments(ctx, ListDocumentsInput{
		TenantID: domain.TenantID("tenant_1"),
	})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(result.Documents))
	}
	if result.Documents[0].Name != "Handbook.md" {
		t.Fatalf("document name = %q, want Handbook.md", result.Documents[0].Name)
	}
}

func TestListDocumentsRejectsInvalidInput(t *testing.T) {
	service := NewDocumentService(memory.New(), fixedIDs{}, fixedClock{})

	_, err := service.ListDocuments(context.Background(), ListDocumentsInput{})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}

func TestGetDocumentDetailReturnsRelatedJobs(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})

	registered, err := service.RegisterDocument(ctx, RegisterDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/source.md",
		SizeBytes:  42,
	})
	if err != nil {
		t.Fatalf("register document: %v", err)
	}

	unrelated, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_other"),
		TenantID:     domain.TenantID("tenant_1"),
		Type:         domain.JobTypeEmbeddingBackfill,
		ResourceType: "document",
		ResourceID:   "doc_other",
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("create unrelated job: %v", err)
	}
	if err := repos.SaveJob(ctx, unrelated); err != nil {
		t.Fatalf("save unrelated job: %v", err)
	}

	result, err := service.GetDocumentDetail(ctx, DocumentDetailInput{
		TenantID:   domain.TenantID("tenant_1"),
		DocumentID: registered.Document.ID,
	})
	if err != nil {
		t.Fatalf("get document detail: %v", err)
	}
	if result.Document.ID != registered.Document.ID {
		t.Fatalf("document id = %q, want %q", result.Document.ID, registered.Document.ID)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(result.Jobs))
	}
	if result.Jobs[0].ResourceID != string(registered.Document.ID) {
		t.Fatalf("job resource id = %q, want %q", result.Jobs[0].ResourceID, registered.Document.ID)
	}
}

func TestRetryDocumentIngestionCreatesQueuedJobForFailedDocument(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})
	document := newFailedDocument(t)
	if err := repos.SaveDocument(ctx, document); err != nil {
		t.Fatalf("save document: %v", err)
	}

	result, err := service.RetryDocumentIngestion(ctx, RetryDocumentInput{
		TenantID:   document.TenantID,
		DocumentID: document.ID,
	})
	if err != nil {
		t.Fatalf("retry document: %v", err)
	}
	if result.Document.ID != document.ID {
		t.Fatalf("document id = %q, want %q", result.Document.ID, document.ID)
	}
	if result.Job.State != domain.JobStateQueued {
		t.Fatalf("job state = %q, want queued", result.Job.State)
	}
	if result.Job.Type != domain.JobTypeDocumentIngestion || result.Job.ResourceID != string(document.ID) {
		t.Fatalf("job resource = %s/%s, want document ingestion for %s", result.Job.Type, result.Job.ResourceID, document.ID)
	}
}

func TestRetryDocumentIngestionRejectsActiveJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})
	document := newFailedDocument(t)
	if err := repos.SaveDocument(ctx, document); err != nil {
		t.Fatalf("save document: %v", err)
	}
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_active"),
		TenantID:     document.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(document.ID),
		Now:          fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = service.RetryDocumentIngestion(ctx, RetryDocumentInput{
		TenantID:   document.TenantID,
		DocumentID: document.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestRetryDocumentIngestionRejectsUploadedDocument(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{})
	registered, err := service.RegisterDocument(ctx, RegisterDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/source.md",
		SizeBytes:  42,
	})
	if err != nil {
		t.Fatalf("register document: %v", err)
	}

	_, err = service.RetryDocumentIngestion(ctx, RetryDocumentInput{
		TenantID:   registered.Document.TenantID,
		DocumentID: registered.Document.ID,
	})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want invalid state transition", err)
	}
}

func TestDeleteDocumentMarksDeletedAndRemovesObject(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	service := NewDocumentService(repos, fixedIDs{}, fixedClock{}).WithObjectStore(objects)

	uploaded, err := service.UploadDocument(ctx, UploadDocumentInput{
		TenantID:    domain.TenantID("tenant_1"),
		OwnerID:     domain.UserID("user_1"),
		Name:        "Handbook.md",
		ContentType: "text/markdown",
		SizeBytes:   11,
		Body:        strings.NewReader("hello world"),
	})
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}

	result, err := service.DeleteDocument(ctx, DeleteDocumentInput{
		TenantID:   domain.TenantID("tenant_1"),
		DocumentID: uploaded.Document.ID,
	})
	if err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if result.Document.Status != domain.DocumentStatusDeleted {
		t.Fatalf("status = %q, want deleted", result.Document.Status)
	}
	if keys := objects.Keys(); len(keys) != 0 {
		t.Fatalf("object keys = %v, want none", keys)
	}

	list, err := service.ListDocuments(ctx, ListDocumentsInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(list.Documents) != 0 {
		t.Fatalf("documents len = %d, want 0", len(list.Documents))
	}
}

func newFailedDocument(t *testing.T) domain.Document {
	t.Helper()
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID("doc_failed"),
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/doc_failed/Handbook.md",
		SizeBytes:  42,
		Now:        fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	if err := document.Transition(domain.DocumentStatusFailed, fixedClock{}.Now()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	return document
}

type fixedIDs struct{}

func (fixedIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_fixed")
}

func (fixedIDs) NewJobID() domain.JobID {
	return domain.JobID("job_fixed")
}

type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
