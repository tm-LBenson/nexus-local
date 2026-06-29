package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestProcessDocumentIngestionBatchProcessesQueuedJobsOnce(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	objects := objectmemory.New()
	vectors := vectormemory.New()

	for i := 1; i <= 3; i++ {
		doc := newBatchDocument(t, i)
		job := newBatchDocumentJob(t, doc, i)
		if _, err := objects.PutObject(ctx, providers.ObjectPut{
			TenantID:    doc.TenantID,
			Key:         doc.StorageKey,
			Body:        strings.NewReader(fmt.Sprintf("batch document %d alpha beta gamma", i)),
			ContentType: "text/markdown",
			SizeBytes:   doc.SizeBytes,
		}); err != nil {
			t.Fatalf("put object %s: %v", doc.ID, err)
		}
		if err := repos.SaveDocument(ctx, doc); err != nil {
			t.Fatalf("save document %s: %v", doc.ID, err)
		}
		if err := repos.SaveJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", job.ID, err)
		}
	}

	worker := NewDocumentIngestionWorker(repos, fixedClock{}).
		WithPipeline(objects, embeddinghash.New("test", 16), vectors)
	result, err := ProcessDocumentIngestionBatch(ctx, worker, 5)
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}
	if result.Processed != 3 || result.Empty != 2 || result.Failed != 0 {
		t.Fatalf("batch = %#v, want 3 processed, 2 empty, 0 failed", result)
	}
	seen := map[domain.DocumentID]bool{}
	for _, processed := range result.Results {
		if seen[processed.DocumentID] {
			t.Fatalf("document %s processed more than once", processed.DocumentID)
		}
		seen[processed.DocumentID] = true
		document, err := repos.GetDocument(ctx, domain.TenantID("tenant_1"), processed.DocumentID)
		if err != nil {
			t.Fatalf("get document %s: %v", processed.DocumentID, err)
		}
		if document.Status != domain.DocumentStatusReady {
			t.Fatalf("document %s status = %s, want ready", document.ID, document.Status)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("processed documents = %#v, want 3 unique documents", seen)
	}
}

func TestProcessDocumentIngestionBatchNormalizesConcurrency(t *testing.T) {
	ctx := context.Background()
	worker := NewDocumentIngestionWorker(memory.New(), fixedClock{})

	result, err := ProcessDocumentIngestionBatch(ctx, worker, 0)
	if err != nil {
		t.Fatalf("process empty batch: %v", err)
	}
	if result.Processed != 0 || result.Empty != 1 || result.Failed != 0 {
		t.Fatalf("batch = %#v, want one empty slot", result)
	}
}

func newBatchDocument(t *testing.T, index int) domain.Document {
	t.Helper()
	doc, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID(fmt.Sprintf("doc_batch_%d", index)),
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       fmt.Sprintf("Batch-%d.md", index),
		StorageKey: fmt.Sprintf("tenants/tenant_1/documents/doc_batch_%d/Batch-%d.md", index, index),
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new batch document: %v", err)
	}
	return doc
}

func newBatchDocumentJob(t *testing.T, doc domain.Document, index int) domain.Job {
	t.Helper()
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID(fmt.Sprintf("job_batch_%d", index)),
		TenantID:     doc.TenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   string(doc.ID),
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new batch job: %v", err)
	}
	return job
}
