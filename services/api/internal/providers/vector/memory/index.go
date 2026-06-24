package memory

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type Index struct {
	mu      sync.RWMutex
	vectors map[key]providers.Vector
}

type key struct {
	tenantID   domain.TenantID
	documentID domain.DocumentID
	chunkID    string
}

func New() *Index {
	return &Index{vectors: map[key]providers.Vector{}}
}

func (i *Index) Upsert(ctx context.Context, vectors []providers.Vector) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, vector := range vectors {
		i.vectors[key{
			tenantID:   vector.TenantID,
			documentID: vector.DocumentID,
			chunkID:    vector.ChunkID,
		}] = vector
	}
	return nil
}

func (i *Index) Search(ctx context.Context, input providers.VectorSearch) ([]providers.VectorHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	i.mu.RLock()
	defer i.mu.RUnlock()

	hits := make([]providers.VectorHit, 0)
	for _, vector := range i.vectors {
		if vector.TenantID != input.TenantID {
			continue
		}
		if !matchesFilters(vector.Metadata, input.Filters) {
			continue
		}
		hits = append(hits, providers.VectorHit{
			DocumentID: vector.DocumentID,
			ChunkID:    vector.ChunkID,
			Text:       vector.Text,
			Score:      cosine(input.Query, vector.Values),
			Metadata:   vector.Metadata,
		})
	}
	sort.Slice(hits, func(a, b int) bool {
		return hits[a].Score > hits[b].Score
	})
	if input.Limit > 0 && len(hits) > input.Limit {
		hits = hits[:input.Limit]
	}
	return hits, nil
}

func (i *Index) DeleteDocument(ctx context.Context, tenantID domain.TenantID, documentID domain.DocumentID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	for key := range i.vectors {
		if key.tenantID == tenantID && key.documentID == documentID {
			delete(i.vectors, key)
		}
	}
	return nil
}

func (i *Index) Count() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.vectors)
}

func matchesFilters(metadata map[string]string, filters map[string]string) bool {
	for key, value := range filters {
		if metadata[key] != value {
			return false
		}
	}
	return true
}

func cosine(a []float32, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	var normA float64
	var normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
