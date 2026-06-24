package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
)

func TestSearchReturnsTenantScopedVectorHits(t *testing.T) {
	ctx := context.Background()
	embedder := embeddinghash.New("test", 16)
	index := vectormemory.New()
	service := NewSearchService(embedder, index)

	semanticVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed semantic text: %v", err)
	}
	otherVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"invoice receipt payroll"}})
	if err != nil {
		t.Fatalf("embed other text: %v", err)
	}
	if err := index.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_1"),
			ChunkID:    "chunk_1",
			Values:     semanticVector.Vectors[0],
			Text:       "alpha beta launch plan",
			Metadata:   map[string]string{"section": "planning"},
		},
		{
			TenantID:   domain.TenantID("tenant_2"),
			DocumentID: domain.DocumentID("doc_2"),
			ChunkID:    "chunk_2",
			Values:     semanticVector.Vectors[0],
			Text:       "alpha beta launch plan from another tenant",
			Metadata:   map[string]string{"section": "planning"},
		},
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_3"),
			ChunkID:    "chunk_3",
			Values:     otherVector.Vectors[0],
			Text:       "invoice receipt payroll",
			Metadata:   map[string]string{"section": "finance"},
		},
	}); err != nil {
		t.Fatalf("upsert vectors: %v", err)
	}

	result, err := service.Search(ctx, SearchInput{
		TenantID: domain.TenantID("tenant_1"),
		Query:    "alpha beta",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(result.Hits))
	}
	if result.Hits[0].DocumentID != domain.DocumentID("doc_1") {
		t.Fatalf("document id = %q, want doc_1", result.Hits[0].DocumentID)
	}
}

func TestSearchRejectsInvalidInput(t *testing.T) {
	service := NewSearchService(embeddinghash.New("test", 16), vectormemory.New())

	_, err := service.Search(context.Background(), SearchInput{
		TenantID: domain.TenantID("tenant_1"),
	})
	if !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}

func TestSearchRequiresPipeline(t *testing.T) {
	service := NewSearchService(nil, nil)

	_, err := service.Search(context.Background(), SearchInput{
		TenantID: domain.TenantID("tenant_1"),
		Query:    "alpha",
	})
	if !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("err = %v, want ErrSearchUnavailable", err)
	}
}
