package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	minioobject "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/minio"
)

func OpenObjectStore(ctx context.Context, cfg config.Config) (providers.ObjectStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.ObjectStoreBackend)) {
	case "", "memory":
		return objectmemory.New(), nil
	case "minio":
		openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		store, err := minioobject.New(openCtx, minioobject.Config{
			Endpoint:  cfg.ObjectStoreEndpoint,
			AccessKey: cfg.ObjectStoreAccessKey,
			SecretKey: cfg.ObjectStoreSecretKey,
			Bucket:    cfg.ObjectStoreBucket,
		})
		if err != nil {
			return nil, fmt.Errorf("object store: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported object storage backend %q", cfg.ObjectStoreBackend)
	}
}
