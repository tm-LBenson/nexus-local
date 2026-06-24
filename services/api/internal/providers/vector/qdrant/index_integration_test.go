package qdrant

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestIndexIntegrationUpsertSearchAndDelete(t *testing.T) {
	baseURL := os.Getenv("TEST_QDRANT_URL")
	if baseURL == "" {
		t.Skip("set TEST_QDRANT_URL to run Qdrant integration tests")
	}

	ctx := context.Background()
	collection := "documents_test_" + time.Now().UTC().Format("20060102150405")
	index, err := New(ctx, Config{
		BaseURL:    baseURL,
		Collection: collection,
		Dimensions: 2,
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	t.Cleanup(func() {
		_ = index.expectOK(context.Background(), "DELETE", "/collections/"+collection, nil)
	})

	err = index.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_1"),
			ChunkID:    "chunk_0000",
			Values:     []float32{1, 0},
			Text:       "alpha beta",
			Metadata:   map[string]string{"document_name": "Handbook.md"},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := index.Search(ctx, providers.VectorSearch{
		TenantID: domain.TenantID("tenant_1"),
		Query:    []float32{1, 0},
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].DocumentID != domain.DocumentID("doc_1") {
		t.Fatalf("document id = %q", hits[0].DocumentID)
	}

	if err := index.DeleteDocument(ctx, domain.TenantID("tenant_1"), domain.DocumentID("doc_1")); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	hits, err = index.Search(ctx, providers.VectorSearch{
		TenantID: domain.TenantID("tenant_1"),
		Query:    []float32{1, 0},
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits after delete = %d, want 0", len(hits))
	}
}
