package runtime

import (
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
)

func OpenVectorIndex(cfg config.Config) (providers.VectorIndex, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.VectorBackend)) {
	case "", "memory":
		return vectormemory.New(), nil
	default:
		return nil, fmt.Errorf("unsupported vector backend %q", cfg.VectorBackend)
	}
}
