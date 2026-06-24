package runtime

import (
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
)

func OpenEmbedder(cfg config.Config) (providers.Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.EmbeddingBackend)) {
	case "", "hash":
		return embeddinghash.New(cfg.EmbeddingModel, cfg.EmbeddingDimensions), nil
	default:
		return nil, fmt.Errorf("unsupported embedding backend %q", cfg.EmbeddingBackend)
	}
}
