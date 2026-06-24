package app

import (
	"context"
	"fmt"
	"strings"

	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type TenantIDs interface {
	NewTenantID() domain.TenantID
}

type TenantService struct {
	repos store.RepositorySet
	ids   TenantIDs
	clock Clock
}

type MembershipSummary struct {
	Tenant     domain.Tenant
	Membership domain.Membership
}

type CurrentUserResult struct {
	User        domain.User
	Memberships []MembershipSummary
}

type CreateTenantInput struct {
	Principal internalauth.Principal
	Name      string
}

type CreateTenantResult struct {
	User       domain.User
	Tenant     domain.Tenant
	Membership domain.Membership
}

func NewTenantService(repos store.RepositorySet, ids TenantIDs, clock Clock) TenantService {
	return TenantService{
		repos: repos,
		ids:   ids,
		clock: clock,
	}
}

func (s TenantService) CurrentUser(ctx context.Context, principal internalauth.Principal) (CurrentUserResult, error) {
	if err := ctx.Err(); err != nil {
		return CurrentUserResult{}, err
	}
	user, err := s.ensureUser(ctx, principal)
	if err != nil {
		return CurrentUserResult{}, err
	}
	memberships, err := s.membershipSummaries(ctx, user.ID)
	if err != nil {
		return CurrentUserResult{}, err
	}
	return CurrentUserResult{
		User:        user,
		Memberships: memberships,
	}, nil
}

func (s TenantService) CreateTenant(ctx context.Context, input CreateTenantInput) (CreateTenantResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateTenantResult{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return CreateTenantResult{}, fmt.Errorf("tenant name: %w", domain.ErrInvalidEntity)
	}

	user, err := s.ensureUser(ctx, input.Principal)
	if err != nil {
		return CreateTenantResult{}, err
	}
	now := s.clock.Now()
	tenant, err := domain.NewTenant(domain.TenantCreate{
		ID:   s.ids.NewTenantID(),
		Name: input.Name,
		Now:  now,
	})
	if err != nil {
		return CreateTenantResult{}, err
	}
	membership, err := domain.NewMembership(domain.MembershipCreate{
		TenantID: tenant.ID,
		UserID:   user.ID,
		Role:     domain.RoleOwner,
	})
	if err != nil {
		return CreateTenantResult{}, err
	}

	if err := s.repos.SaveTenant(ctx, tenant); err != nil {
		return CreateTenantResult{}, err
	}
	if err := s.repos.SaveMembership(ctx, membership); err != nil {
		return CreateTenantResult{}, err
	}

	return CreateTenantResult{
		User:       user,
		Tenant:     tenant,
		Membership: membership,
	}, nil
}

func (s TenantService) ensureUser(ctx context.Context, principal internalauth.Principal) (domain.User, error) {
	email := strings.TrimSpace(principal.Email)
	if email == "" {
		email = string(principal.UserID) + "@unknown.local"
	}
	user, err := domain.NewUser(domain.UserCreate{
		ID:    principal.UserID,
		Email: email,
		Name:  "",
		Now:   s.clock.Now(),
	})
	if err != nil {
		return domain.User{}, err
	}
	if err := s.repos.SaveUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s TenantService) membershipSummaries(ctx context.Context, userID domain.UserID) ([]MembershipSummary, error) {
	memberships, err := s.repos.ListMembershipsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	summaries := make([]MembershipSummary, 0, len(memberships))
	for _, membership := range memberships {
		tenant, err := s.repos.GetTenant(ctx, membership.TenantID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, MembershipSummary{
			Tenant:     tenant,
			Membership: membership,
		})
	}
	return summaries, nil
}
