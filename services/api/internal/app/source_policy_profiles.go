package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourcePolicyProfileIDs interface {
	NewSourcePolicyProfileID() domain.SourcePolicyProfileID
}

type SourcePolicyProfileService struct {
	repos store.RepositorySet
	ids   SourcePolicyProfileIDs
	clock Clock
}

type ListSourcePolicyProfilesInput struct {
	TenantID domain.TenantID
}

type CreateSourcePolicyProfileInput struct {
	TenantID            domain.TenantID
	OwnerID             domain.UserID
	Name                string
	Detail              string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
}

type UpdateSourcePolicyProfileInput struct {
	TenantID              domain.TenantID
	SourcePolicyProfileID domain.SourcePolicyProfileID
	Name                  string
	Detail                string
	IncludePatterns       []string
	ExcludePatterns       []string
	ScanIntervalMinutes   int
}

type DeleteSourcePolicyProfileInput struct {
	TenantID              domain.TenantID
	SourcePolicyProfileID domain.SourcePolicyProfileID
}

type ListSourcePolicyProfilesResult struct {
	Profiles []domain.SourcePolicyProfile
}

type SourcePolicyProfileResult struct {
	Profile domain.SourcePolicyProfile
}

const maxSourcePolicyProfilesPerTenant = 50

func NewSourcePolicyProfileService(repos store.RepositorySet, ids SourcePolicyProfileIDs, clock Clock) SourcePolicyProfileService {
	return SourcePolicyProfileService{repos: repos, ids: ids, clock: clock}
}

func (s SourcePolicyProfileService) List(ctx context.Context, input ListSourcePolicyProfilesInput) (ListSourcePolicyProfilesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListSourcePolicyProfilesResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListSourcePolicyProfilesResult{}, fmt.Errorf("source policy profiles: %w", domain.ErrInvalidEntity)
	}
	profiles, err := s.repos.ListSourcePolicyProfiles(ctx, input.TenantID)
	if err != nil {
		return ListSourcePolicyProfilesResult{}, err
	}
	return ListSourcePolicyProfilesResult{Profiles: profiles}, nil
}

func (s SourcePolicyProfileService) Create(ctx context.Context, input CreateSourcePolicyProfileInput) (SourcePolicyProfileResult, error) {
	if err := ctx.Err(); err != nil {
		return SourcePolicyProfileResult{}, err
	}
	profiles, err := s.repos.ListSourcePolicyProfiles(ctx, input.TenantID)
	if err != nil {
		return SourcePolicyProfileResult{}, err
	}
	if len(profiles) >= maxSourcePolicyProfilesPerTenant {
		return SourcePolicyProfileResult{}, fmt.Errorf("source policy profile limit reached: %w", domain.ErrInvalidStateTransition)
	}
	profile, err := domain.NewSourcePolicyProfile(domain.SourcePolicyProfileCreate{
		ID:                  s.ids.NewSourcePolicyProfileID(),
		TenantID:            input.TenantID,
		OwnerID:             input.OwnerID,
		Name:                input.Name,
		Detail:              input.Detail,
		IncludePatterns:     input.IncludePatterns,
		ExcludePatterns:     input.ExcludePatterns,
		ScanIntervalMinutes: input.ScanIntervalMinutes,
		Now:                 s.clock.Now(),
	})
	if err != nil {
		return SourcePolicyProfileResult{}, err
	}
	if err := s.repos.SaveSourcePolicyProfile(ctx, profile); err != nil {
		return SourcePolicyProfileResult{}, err
	}
	return SourcePolicyProfileResult{Profile: profile}, nil
}

func (s SourcePolicyProfileService) Update(ctx context.Context, input UpdateSourcePolicyProfileInput) (SourcePolicyProfileResult, error) {
	if err := ctx.Err(); err != nil {
		return SourcePolicyProfileResult{}, err
	}
	profile, err := s.repos.GetSourcePolicyProfile(ctx, input.TenantID, input.SourcePolicyProfileID)
	if err != nil {
		return SourcePolicyProfileResult{}, err
	}
	if err := profile.Update(input.Name, input.Detail, input.IncludePatterns, input.ExcludePatterns, input.ScanIntervalMinutes, s.clock.Now()); err != nil {
		return SourcePolicyProfileResult{}, err
	}
	if err := s.repos.SaveSourcePolicyProfile(ctx, profile); err != nil {
		return SourcePolicyProfileResult{}, err
	}
	return SourcePolicyProfileResult{Profile: profile}, nil
}

func (s SourcePolicyProfileService) Delete(ctx context.Context, input DeleteSourcePolicyProfileInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.repos.DeleteSourcePolicyProfile(ctx, input.TenantID, input.SourcePolicyProfileID)
}
