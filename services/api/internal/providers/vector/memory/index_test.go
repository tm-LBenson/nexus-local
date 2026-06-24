package memory

import (
	"context"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestIndexSearchIsTenantScoped(t *testing.T) {
	index := New()
	err := index.Upsert(context.Background(), []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_1"),
			ChunkID:    "chunk_0000",
			Values:     []float32{1, 0},
			Text:       "alpha",
		},
		{
			TenantID:   domain.TenantID("tenant_2"),
			DocumentID: domain.DocumentID("doc_2"),
			ChunkID:    "chunk_0000",
			Values:     []float32{1, 0},
			Text:       "beta",
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := index.Search(context.Background(), providers.VectorSearch{
		TenantID: domain.TenantID("tenant_1"),
		Query:    []float32{1, 0},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].DocumentID != domain.DocumentID("doc_1") {
		t.Fatalf("doc = %q", hits[0].DocumentID)
	}
}

func TestIndexDeleteDocument(t *testing.T) {
	index := New()
	_ = index.Upsert(context.Background(), []providers.Vector{
		{TenantID: "tenant_1", DocumentID: "doc_1", ChunkID: "a", Values: []float32{1}},
		{TenantID: "tenant_1", DocumentID: "doc_1", ChunkID: "b", Values: []float32{1}},
	})

	if err := index.DeleteDocument(context.Background(), "tenant_1", "doc_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if index.Count() != 0 {
		t.Fatalf("count = %d, want 0", index.Count())
	}
}
