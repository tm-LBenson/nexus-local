package hash

import (
	"context"
	"hash/fnv"
	"math"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type Embedder struct {
	model      string
	dimensions int
}

func New(model string, dimensions int) Embedder {
	if model == "" {
		model = "hash-embedding"
	}
	if dimensions <= 0 {
		dimensions = 384
	}
	return Embedder{
		model:      model,
		dimensions: dimensions,
	}
}

func (e Embedder) Embed(ctx context.Context, input providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	if err := ctx.Err(); err != nil {
		return providers.EmbeddingResponse{}, err
	}
	vectors := make([][]float32, 0, len(input.Texts))
	for _, text := range input.Texts {
		vectors = append(vectors, e.embedText(text))
	}

	model := input.Model
	if model == "" {
		model = e.model
	}
	return providers.EmbeddingResponse{
		Model:   model,
		Vectors: vectors,
	}, nil
}

func (e Embedder) embedText(text string) []float32 {
	vector := make([]float32, e.dimensions)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		sum := h.Sum64()
		index := int(sum % uint64(e.dimensions))
		sign := float32(1)
		if (sum>>63)&1 == 1 {
			sign = -1
		}
		vector[index] += sign
	}

	var norm float64
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}
