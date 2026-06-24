package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

func TestStoreIntegrationDocumentsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)

	docA := newTestDocument(t, domain.TenantID("tenant_a"), domain.DocumentID("doc_1"))
	docB := newTestDocument(t, domain.TenantID("tenant_b"), domain.DocumentID("doc_1"))

	if err := repo.SaveDocument(ctx, docA); err != nil {
		t.Fatalf("save docA: %v", err)
	}
	if err := repo.SaveDocument(ctx, docB); err != nil {
		t.Fatalf("save docB: %v", err)
	}

	docs, err := repo.ListDocuments(ctx, domain.TenantID("tenant_a"))
	if err != nil {
		t.Fatalf("list docs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	if docs[0].TenantID != domain.TenantID("tenant_a") {
		t.Fatalf("tenant = %q, want tenant_a", docs[0].TenantID)
	}
}

func TestStoreIntegrationClaimNextQueuedJob(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)
	job, err := domain.NewJob(domain.JobCreate{
		ID:       domain.JobID("job_1"),
		TenantID: domain.TenantID("tenant_a"),
		Type:     domain.JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	claimed, err := repo.ClaimNextQueuedJob(ctx, fixedTime().Add(time.Minute))
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed.State != domain.JobStateRunning {
		t.Fatalf("state = %q, want running", claimed.State)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", claimed.Attempts)
	}

	_, err = repo.ClaimNextQueuedJob(ctx, fixedTime())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func newIntegrationStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Postgres integration tests")
	}

	repo, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(repo.Close)

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `
		TRUNCATE tenants, users, memberships, documents, jobs
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return repo
}

func newTestDocument(t *testing.T, tenantID domain.TenantID, documentID domain.DocumentID) domain.Document {
	t.Helper()

	doc, err := domain.NewDocument(domain.DocumentCreate{
		ID:         documentID,
		TenantID:   tenantID,
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/" + string(tenantID) + "/documents/" + string(documentID),
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	return doc
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
