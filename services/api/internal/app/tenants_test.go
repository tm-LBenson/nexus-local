package app

import (
	"context"
	"testing"

	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestCreateTenantCreatesOwnerMembershipForPrincipal(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})

	result, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("user_1"),
			Email:  "user@example.test",
		},
		Name: "Research Lab",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	if result.Tenant.ID != domain.TenantID("tenant_fixed") {
		t.Fatalf("tenant id = %q, want tenant_fixed", result.Tenant.ID)
	}
	if result.Membership.Role != domain.RoleOwner {
		t.Fatalf("role = %q, want owner", result.Membership.Role)
	}

	memberships, err := repos.ListMembershipsForUser(ctx, domain.UserID("user_1"))
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].TenantID != domain.TenantID("tenant_fixed") {
		t.Fatalf("memberships = %#v", memberships)
	}
}

func TestCurrentUserReturnsMembershipSummaries(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})

	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("user_1"),
			Email:  "user@example.test",
		},
		Name: "Research Lab",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	result, err := service.CurrentUser(ctx, internalauth.Principal{
		UserID: domain.UserID("user_1"),
		Email:  "user@example.test",
	})
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if result.User.Email != "user@example.test" {
		t.Fatalf("email = %q, want user@example.test", result.User.Email)
	}
	if len(result.Memberships) != 1 {
		t.Fatalf("memberships len = %d, want 1", len(result.Memberships))
	}
	if result.Memberships[0].Tenant.Name != "Research Lab" {
		t.Fatalf("tenant name = %q, want Research Lab", result.Memberships[0].Tenant.Name)
	}
}

func TestCreateTenantRejectsMissingName(t *testing.T) {
	service := NewTenantService(memory.New(), tenantIDs{}, fixedClock{})

	_, err := service.CreateTenant(context.Background(), CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("user_1"),
			Email:  "user@example.test",
		},
	})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}

type tenantIDs struct{}

func (tenantIDs) NewTenantID() domain.TenantID {
	return domain.TenantID("tenant_fixed")
}
