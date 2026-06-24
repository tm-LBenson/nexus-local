package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
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
