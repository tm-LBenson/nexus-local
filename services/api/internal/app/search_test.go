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

type fixedSearchEmbedder struct{}

func (fixedSearchEmbedder) Embed(context.Context, providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	return providers.EmbeddingResponse{Model: "fixed", Vectors: [][]float32{{1}}}, nil
}

type fixedSearchIndex struct {
	hits      []providers.VectorHit
	lastLimit int
}

func (i *fixedSearchIndex) Upsert(context.Context, []providers.Vector) error {
	return nil
}

func (i *fixedSearchIndex) Search(_ context.Context, input providers.VectorSearch) ([]providers.VectorHit, error) {
	i.lastLimit = input.Limit
	if input.Limit > 0 && len(i.hits) > input.Limit {
		return append([]providers.VectorHit(nil), i.hits[:input.Limit]...), nil
	}
	return append([]providers.VectorHit(nil), i.hits...), nil
}

func (i *fixedSearchIndex) DeleteDocument(context.Context, domain.TenantID, domain.DocumentID) error {
	return nil
}

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

func TestSearchFiltersByDocumentID(t *testing.T) {
	ctx := context.Background()
	embedder := embeddinghash.New("test", 16)
	index := vectormemory.New()
	service := NewSearchService(embedder, index)

	alphaVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed alpha text: %v", err)
	}
	omegaVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"omega archive notes"}})
	if err != nil {
		t.Fatalf("embed omega text: %v", err)
	}
	if err := index.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_alpha"),
			ChunkID:    "chunk_1",
			Values:     alphaVector.Vectors[0],
			Text:       "alpha beta launch plan",
		},
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_omega"),
			ChunkID:    "chunk_2",
			Values:     omegaVector.Vectors[0],
			Text:       "omega archive notes",
		},
	}); err != nil {
		t.Fatalf("upsert vectors: %v", err)
	}

	result, err := service.Search(ctx, SearchInput{
		TenantID:   domain.TenantID("tenant_1"),
		DocumentID: domain.DocumentID("doc_omega"),
		Query:      "alpha beta",
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(result.Hits))
	}
	if result.Hits[0].DocumentID != domain.DocumentID("doc_omega") {
		t.Fatalf("document id = %q, want doc_omega", result.Hits[0].DocumentID)
	}
}

func TestHybridSearchReranksLexicalMatchesFromCandidateSet(t *testing.T) {
	ctx := context.Background()
	index := &fixedSearchIndex{hits: []providers.VectorHit{
		{
			DocumentID: domain.DocumentID("doc_generic"),
			ChunkID:    "chunk_generic",
			Text:       "General account troubleshooting notes mention support contacts and escalation queues.",
			Score:      0.99,
			Metadata:   map[string]string{"document_name": "General Support.md"},
		},
		{
			DocumentID: domain.DocumentID("doc_oidc"),
			ChunkID:    "chunk_redirect",
			Text:       "OIDC invalid redirect URI callback mismatch is fixed by making the application request exactly match the configured redirect URI.",
			Score:      0.20,
			Metadata:   map[string]string{"document_name": "OIDC Troubleshooting.md"},
		},
	}}
	service := NewSearchService(fixedSearchEmbedder{}, index)

	vectorResult, err := service.Search(ctx, SearchInput{
		TenantID: domain.TenantID("tenant_1"),
		Query:    "OIDC invalid redirect URI callback mismatch",
		Limit:    1,
		Strategy: SearchStrategyVector,
	})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if vectorResult.Hits[0].DocumentID != domain.DocumentID("doc_generic") {
		t.Fatalf("vector top document = %q, want doc_generic", vectorResult.Hits[0].DocumentID)
	}

	hybridResult, err := service.Search(ctx, SearchInput{
		TenantID: domain.TenantID("tenant_1"),
		Query:    "OIDC invalid redirect URI callback mismatch",
		Limit:    1,
		Strategy: SearchStrategyHybrid,
	})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if index.lastLimit <= 1 {
		t.Fatalf("hybrid candidate limit = %d, want wider than final limit", index.lastLimit)
	}
	if hybridResult.Hits[0].DocumentID != domain.DocumentID("doc_oidc") {
		t.Fatalf("hybrid top document = %q, want doc_oidc", hybridResult.Hits[0].DocumentID)
	}
	if hybridResult.Hits[0].Metadata["retrieval_strategy"] != string(SearchStrategyHybrid) {
		t.Fatalf("retrieval strategy metadata = %q, want hybrid", hybridResult.Hits[0].Metadata["retrieval_strategy"])
	}
	if hybridResult.Hits[0].Metadata["lexical_score"] == "" {
		t.Fatalf("expected lexical score metadata")
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

func TestSearchRejectsUnknownStrategy(t *testing.T) {
	service := NewSearchService(embeddinghash.New("test", 16), vectormemory.New())

	_, err := service.Search(context.Background(), SearchInput{
		TenantID: domain.TenantID("tenant_1"),
		Query:    "alpha",
		Strategy: SearchStrategy("keyword-only"),
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
