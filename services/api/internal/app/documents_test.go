package app

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
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
