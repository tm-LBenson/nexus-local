package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/qdrant"
)

func OpenVectorIndex(cfg config.Config) (providers.VectorIndex, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.VectorBackend)) {
	case "", "memory":
		return vectormemory.New(), nil
	case "qdrant":
		return qdrant.New(context.Background(), qdrant.Config{
			BaseURL:    cfg.VectorBaseURL,
			APIKey:     cfg.VectorAPIKey,
			Collection: cfg.VectorCollection,
			Dimensions: cfg.EmbeddingDimensions,
		})
	default:
		return nil, fmt.Errorf("unsupported vector backend %q", cfg.VectorBackend)
	}
}
