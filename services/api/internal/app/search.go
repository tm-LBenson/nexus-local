package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

var ErrSearchUnavailable = errors.New("search pipeline is not configured")

const (
	defaultSearchLimit = 5
	maxSearchLimit     = 20
)

type SearchService struct {
	embedder providers.Embedder
	vectors  providers.VectorIndex
}

type SearchInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
	Query      string
	Limit      int
	Filters    map[string]string
}

type SearchResult struct {
	Hits []providers.VectorHit
}

func NewSearchService(embedder providers.Embedder, vectors providers.VectorIndex) SearchService {
	return SearchService{
		embedder: embedder,
		vectors:  vectors,
	}
}

func (s SearchService) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if s.embedder == nil || s.vectors == nil {
		return SearchResult{}, ErrSearchUnavailable
	}

	query := strings.TrimSpace(input.Query)
	if strings.TrimSpace(string(input.TenantID)) == "" || query == "" {
		return SearchResult{}, fmt.Errorf("search: %w", domain.ErrInvalidEntity)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	embedded, err := s.embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{query}})
	if err != nil {
		return SearchResult{}, err
	}
	if len(embedded.Vectors) != 1 {
		return SearchResult{}, fmt.Errorf("embedding count %d does not match query count 1", len(embedded.Vectors))
	}

	hits, err := s.vectors.Search(ctx, providers.VectorSearch{
		TenantID:   input.TenantID,
		DocumentID: input.DocumentID,
		Query:      embedded.Vectors[0],
		Limit:      limit,
		Filters:    input.Filters,
	})
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Hits: hits}, nil
}
