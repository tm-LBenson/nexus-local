package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/postgres"
)

func OpenRepositories(ctx context.Context, cfg config.Config) (store.RepositorySet, func(), error) {
	switch strings.ToLower(strings.TrimSpace(cfg.PersistenceBackend)) {
	case "", "memory":
		return memory.New(), func() {}, nil
	case "postgres":
		openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		repos, err := postgres.Open(openCtx, cfg.DatabaseURL)
		if err != nil {
			return nil, func() {}, fmt.Errorf("postgres store: %w", err)
		}
		if cfg.RunMigrations {
			if err := repos.Migrate(openCtx); err != nil {
				repos.Close()
				return nil, func() {}, fmt.Errorf("postgres migrations: %w", err)
			}
		}
		return repos, repos.Close, nil
	default:
		return nil, func() {}, fmt.Errorf("unsupported persistence backend %q", cfg.PersistenceBackend)
	}
}
