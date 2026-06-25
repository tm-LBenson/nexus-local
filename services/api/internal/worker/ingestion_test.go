package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestDocumentIngestionWorkerProcessesQueuedJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	vectors := vectormemory.New()
	doc := newTestDocument(t)
	job := newDocumentJob(t, doc)

	if _, err := objects.PutObject(ctx, providers.ObjectPut{
		TenantID:    doc.TenantID,
		Key:         doc.StorageKey,
		Body:        strings.NewReader("alpha beta gamma delta epsilon zeta eta theta"),
		ContentType: "text/markdown",
		SizeBytes:   47,
	}); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if err := repos.SaveDocument(ctx, doc); err != nil {
		t.Fatalf("save document: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewDocumentIngestionWorker(repos, fixedClock{}).
		WithPipeline(objects, embeddinghash.New("test", 16), vectors)
	result, err := worker.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process next: %v", err)
	}
	if result.JobID != job.ID {
		t.Fatalf("job id = %q, want %q", result.JobID, job.ID)
	}
	if result.ChunkCount == 0 {
		t.Fatal("chunk count = 0, want vectors")
	}

	updatedDoc, err := repos.GetDocument(ctx, doc.TenantID, doc.ID)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if updatedDoc.Status != domain.DocumentStatusReady {
		t.Fatalf("document status = %q, want ready", updatedDoc.Status)
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateSucceeded {
		t.Fatalf("job state = %q, want succeeded", updatedJob.State)
	}

	query, err := embeddinghash.New("test", 16).Embed(ctx, providers.EmbeddingRequest{Texts: []string{"alpha beta"}})
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	hits, err := vectors.Search(ctx, providers.VectorSearch{
		TenantID: doc.TenantID,
		Query:    query.Vectors[0],
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("search vectors: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].Metadata["document_name"] != "Handbook.md" {
		t.Fatalf("document name metadata = %q, want Handbook.md", hits[0].Metadata["document_name"])
	}
	if hits[0].Metadata["chunk_index"] == "" {
		t.Fatalf("chunk index metadata is empty: %#v", hits[0].Metadata)
	}
}

func TestDocumentIngestionWorkerReturnsNoQueuedJobs(t *testing.T) {
	worker := NewDocumentIngestionWorker(memory.New(), fixedClock{})

	_, err := worker.ProcessNext(context.Background())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDocumentIngestionWorkerFailsUnsupportedJob(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_1"),
		TenantID:     domain.TenantID("tenant_1"),
		Type:         domain.JobTypeAITurn,
		ResourceType: "conversation",
		ResourceID:   "conv_1",
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repos.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	worker := NewDocumentIngestionWorker(repos, fixedClock{})
	_, err = worker.ProcessNext(ctx)
	if err == nil {
		t.Fatal("err = nil, want unsupported job error")
	}

	updatedJob, err := repos.GetJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updatedJob.State != domain.JobStateFailed {
		t.Fatalf("job state = %q, want failed", updatedJob.State)
	}
}

func newTestDocument(t *testing.T) domain.Document {
	t.Helper()
	doc, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID("doc_1"),
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/doc_1/Handbook.md",
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	return doc
}

func newDocumentJob(t *testing.T, doc domain.Document) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_1"),
		TenantID:     doc.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(doc.ID),
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}

type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return fixedTime()
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
