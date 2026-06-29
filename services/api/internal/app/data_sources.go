package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type DataSourceIDs interface {
	NewDataSourceID() domain.DataSourceID
}

type DataSourceService struct {
	repos store.RepositorySet
	ids   DataSourceIDs
	clock Clock
}

type CreateDataSourceInput struct {
	TenantID domain.TenantID
	OwnerID  domain.UserID
	Type     domain.DataSourceType
	Name     string
	RootPath string
}

type UpdateDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
	Type         domain.DataSourceType
	Name         string
	RootPath     string
}

type DataSourceDetailInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type ListDataSourcesInput struct {
	TenantID        domain.TenantID
	IncludeArchived bool
}

type ArchiveDataSourceInput struct {
	TenantID     domain.TenantID
	DataSourceID domain.DataSourceID
}

type DataSourceResult struct {
	Source domain.DataSource
}

type ListDataSourcesResult struct {
	Sources []domain.DataSource
}

func NewDataSourceService(repos store.RepositorySet, ids DataSourceIDs, clock Clock) DataSourceService {
	return DataSourceService{repos: repos, ids: ids, clock: clock}
}

func (s DataSourceService) Create(ctx context.Context, input CreateDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	source, err := domain.NewDataSource(domain.DataSourceCreate{
		ID:       s.ids.NewDataSourceID(),
		TenantID: input.TenantID,
		OwnerID:  input.OwnerID,
		Type:     input.Type,
		Name:     input.Name,
		RootPath: input.RootPath,
		Now:      s.clock.Now(),
	})
	if err != nil {
		return DataSourceResult{}, err
	}
	if err := s.repos.SaveDataSource(ctx, source); err != nil {
		return DataSourceResult{}, err
	}
	return DataSourceResult{Source: source}, nil
}

func (s DataSourceService) Update(ctx context.Context, input UpdateDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceResult{}, err
	}
	if err := source.Update(input.Name, input.Type, input.RootPath, s.clock.Now()); err != nil {
		return DataSourceResult{}, err
	}
	if err := s.repos.SaveDataSource(ctx, source); err != nil {
		return DataSourceResult{}, err
	}
	return DataSourceResult{Source: source}, nil
}

func (s DataSourceService) Get(ctx context.Context, input DataSourceDetailInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.DataSourceID)) == "" {
		return DataSourceResult{}, fmt.Errorf("data source: %w", domain.ErrInvalidEntity)
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceResult{}, err
	}
	return DataSourceResult{Source: source}, nil
}

func (s DataSourceService) List(ctx context.Context, input ListDataSourcesInput) (ListDataSourcesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDataSourcesResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListDataSourcesResult{}, fmt.Errorf("data sources: %w", domain.ErrInvalidEntity)
	}
	sources, err := s.repos.ListDataSources(ctx, input.TenantID)
	if err != nil {
		return ListDataSourcesResult{}, err
	}
	if !input.IncludeArchived {
		sources = filterActiveDataSources(sources)
	}
	return ListDataSourcesResult{Sources: sources}, nil
}

func (s DataSourceService) Archive(ctx context.Context, input ArchiveDataSourceInput) (DataSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return DataSourceResult{}, err
	}
	source, err := s.repos.GetDataSource(ctx, input.TenantID, input.DataSourceID)
	if err != nil {
		return DataSourceResult{}, err
	}
	if source.Status != domain.DataSourceStatusArchived {
		if err := source.Transition(domain.DataSourceStatusArchived, s.clock.Now()); err != nil {
			return DataSourceResult{}, err
		}
		if err := s.repos.SaveDataSource(ctx, source); err != nil {
			return DataSourceResult{}, err
		}
	}
	return DataSourceResult{Source: source}, nil
}

func filterActiveDataSources(sources []domain.DataSource) []domain.DataSource {
	active := sources[:0]
	for _, source := range sources {
		if source.Status != domain.DataSourceStatusArchived {
			active = append(active, source)
		}
	}
	return active
}
