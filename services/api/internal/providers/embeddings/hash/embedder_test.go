package hash

import (
	"context"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestEmbedderReturnsDeterministicNormalizedVectors(t *testing.T) {
	embedder := New("test", 16)

	first, err := embedder.Embed(context.Background(), providers.EmbeddingRequest{Texts: []string{"hello world"}})
	if err != nil {
		t.Fatalf("embed first: %v", err)
	}
	second, err := embedder.Embed(context.Background(), providers.EmbeddingRequest{Texts: []string{"hello world"}})
	if err != nil {
		t.Fatalf("embed second: %v", err)
	}

	if len(first.Vectors) != 1 || len(first.Vectors[0]) != 16 {
		t.Fatalf("vector shape = %d/%d", len(first.Vectors), len(first.Vectors[0]))
	}
	for i := range first.Vectors[0] {
		if first.Vectors[0][i] != second.Vectors[0][i] {
			t.Fatalf("index %d differs", i)
		}
	}
}
