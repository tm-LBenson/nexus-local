package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourceViewIDs interface {
	NewSourceViewID() domain.SourceViewID
}

type SourceViewService struct {
	repos store.RepositorySet
	ids   SourceViewIDs
	clock Clock
}

type SourceViewFiltersInput struct {
	Health   string
	Query    string
	Schedule string
	Type     string
}

type ListSourceViewsInput struct {
	TenantID domain.TenantID
}

type CreateSourceViewInput struct {
	TenantID domain.TenantID
	OwnerID  domain.UserID
	Name     string
	Filters  SourceViewFiltersInput
}

type UpdateSourceViewInput struct {
	TenantID     domain.TenantID
	SourceViewID domain.SourceViewID
	Name         string
	Filters      SourceViewFiltersInput
}

type DeleteSourceViewInput struct {
	TenantID     domain.TenantID
	SourceViewID domain.SourceViewID
}

type ListSourceViewsResult struct {
	Views []domain.SourceView
}

type SourceViewResult struct {
	View domain.SourceView
}

const maxSourceViewsPerTenant = 50

func NewSourceViewService(repos store.RepositorySet, ids SourceViewIDs, clock Clock) SourceViewService {
	return SourceViewService{repos: repos, ids: ids, clock: clock}
}

func (s SourceViewService) List(ctx context.Context, input ListSourceViewsInput) (ListSourceViewsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListSourceViewsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListSourceViewsResult{}, fmt.Errorf("source views: %w", domain.ErrInvalidEntity)
	}
	views, err := s.repos.ListSourceViews(ctx, input.TenantID)
	if err != nil {
		return ListSourceViewsResult{}, err
	}
	return ListSourceViewsResult{Views: views}, nil
}

func (s SourceViewService) Create(ctx context.Context, input CreateSourceViewInput) (SourceViewResult, error) {
	if err := ctx.Err(); err != nil {
		return SourceViewResult{}, err
	}
	views, err := s.repos.ListSourceViews(ctx, input.TenantID)
	if err != nil {
		return SourceViewResult{}, err
	}
	if len(views) >= maxSourceViewsPerTenant {
		return SourceViewResult{}, fmt.Errorf("source view limit reached: %w", domain.ErrInvalidStateTransition)
	}
	view, err := domain.NewSourceView(domain.SourceViewCreate{
		ID:       s.ids.NewSourceViewID(),
		TenantID: input.TenantID,
		OwnerID:  input.OwnerID,
		Name:     input.Name,
		Filters:  sourceViewFiltersFromInput(input.Filters),
		Now:      s.clock.Now(),
	})
	if err != nil {
		return SourceViewResult{}, err
	}
	if err := s.repos.SaveSourceView(ctx, view); err != nil {
		return SourceViewResult{}, err
	}
	return SourceViewResult{View: view}, nil
}

func (s SourceViewService) Update(ctx context.Context, input UpdateSourceViewInput) (SourceViewResult, error) {
	if err := ctx.Err(); err != nil {
		return SourceViewResult{}, err
	}
	view, err := s.repos.GetSourceView(ctx, input.TenantID, input.SourceViewID)
	if err != nil {
		return SourceViewResult{}, err
	}
	if err := view.Update(input.Name, sourceViewFiltersFromInput(input.Filters), s.clock.Now()); err != nil {
		return SourceViewResult{}, err
	}
	if err := s.repos.SaveSourceView(ctx, view); err != nil {
		return SourceViewResult{}, err
	}
	return SourceViewResult{View: view}, nil
}

func (s SourceViewService) Delete(ctx context.Context, input DeleteSourceViewInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.repos.DeleteSourceView(ctx, input.TenantID, input.SourceViewID)
}

func sourceViewFiltersFromInput(input SourceViewFiltersInput) domain.SourceViewFilters {
	return domain.SourceViewFilters{
		Health:   input.Health,
		Query:    input.Query,
		Schedule: input.Schedule,
		Type:     domain.DataSourceType(input.Type),
	}
}
