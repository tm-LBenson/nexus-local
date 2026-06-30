package app

import (
	"context"
	"errors"
	"testing"

	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
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

func TestAddTenantMemberCreatesUserMembershipAndListEntry(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Research Lab",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	result, err := service.AddTenantMember(ctx, AddTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("user_2"),
		Email:    "User2@Example.Test",
		Name:     "User Two",
		Role:     domain.RoleMember,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if result.Member.User.Email != "user2@example.test" {
		t.Fatalf("email = %q, want normalized email", result.Member.User.Email)
	}
	if result.Member.Membership.Role != domain.RoleMember {
		t.Fatalf("role = %q, want member", result.Member.Membership.Role)
	}

	members, err := service.ListTenantMembers(ctx, ListTenantMembersInput{
		TenantID: domain.TenantID("tenant_fixed"),
	})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members.Members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members.Members))
	}
	if members.Members[1].User.ID != domain.UserID("user_2") {
		t.Fatalf("second user = %q, want user_2", members.Members[1].User.ID)
	}

	currentUser, err := service.CurrentUser(ctx, internalauth.Principal{
		UserID: domain.UserID("user_2"),
		Email:  "user2@example.test",
	})
	if err != nil {
		t.Fatalf("current added user: %v", err)
	}
	if len(currentUser.Memberships) != 1 || currentUser.Memberships[0].Tenant.ID != domain.TenantID("tenant_fixed") {
		t.Fatalf("memberships = %#v", currentUser.Memberships)
	}
}

func TestAddTenantMemberRejectsInvalidRole(t *testing.T) {
	ctx := context.Background()
	service := NewTenantService(memory.New(), tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Research Lab",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	_, err := service.AddTenantMember(ctx, AddTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("user_2"),
		Email:    "user2@example.test",
		Role:     domain.Role("superuser"),
	})
	if !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}

func TestDeleteTenantMemberRemovesMembership(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Research Lab",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if _, err := service.AddTenantMember(ctx, AddTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("user_2"),
		Email:    "user2@example.test",
		Role:     domain.RoleViewer,
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	deleted, err := service.DeleteTenantMember(ctx, DeleteTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("user_2"),
	})
	if err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if deleted.Member.User.Email != "user2@example.test" {
		t.Fatalf("deleted email = %q", deleted.Member.User.Email)
	}
	memberships, err := repos.ListMembershipsForUser(ctx, domain.UserID("user_2"))
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 0 {
		t.Fatalf("memberships len = %d, want 0", len(memberships))
	}
}

func TestDeleteTenantMemberProtectsLastOwner(t *testing.T) {
	ctx := context.Background()
	service := NewTenantService(memory.New(), tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Research Lab",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	_, err := service.DeleteTenantMember(ctx, DeleteTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("owner_1"),
	})
	if !errors.Is(err, ErrLastOwner) {
		t.Fatalf("err = %v, want ErrLastOwner", err)
	}

	_, err = service.DeleteTenantMember(ctx, DeleteTenantMemberInput{
		TenantID: domain.TenantID("tenant_fixed"),
		UserID:   domain.UserID("missing"),
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteTenantRemovesWorkspaceRows(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Smoke Workspace",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID("doc_deleted"),
		TenantID:   domain.TenantID("tenant_fixed"),
		OwnerID:    domain.UserID("owner_1"),
		Name:       "deleted.md",
		StorageKey: "tenants/tenant_fixed/documents/doc_deleted/deleted.md",
		SizeBytes:  12,
		Now:        fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	if err := document.Transition(domain.DocumentStatusDeleted, fixedClock{}.Now()); err != nil {
		t.Fatalf("delete document transition: %v", err)
	}
	if err := repos.SaveDocument(ctx, document); err != nil {
		t.Fatalf("save document: %v", err)
	}

	result, err := service.DeleteTenant(ctx, DeleteTenantInput{
		TenantID: domain.TenantID("tenant_fixed"),
	})
	if err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	if result.Tenant.Name != "Smoke Workspace" {
		t.Fatalf("deleted tenant name = %q", result.Tenant.Name)
	}
	if _, err := repos.GetTenant(ctx, domain.TenantID("tenant_fixed")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get tenant err = %v, want ErrNotFound", err)
	}
	memberships, err := repos.ListMembershipsForUser(ctx, domain.UserID("owner_1"))
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 0 {
		t.Fatalf("memberships len = %d, want 0", len(memberships))
	}
	documents, err := repos.ListDocuments(ctx, domain.TenantID("tenant_fixed"))
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(documents) != 0 {
		t.Fatalf("documents len = %d, want 0", len(documents))
	}
}

func TestDeleteTenantRejectsActiveDocumentsOrSources(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	service := NewTenantService(repos, tenantIDs{}, fixedClock{})
	if _, err := service.CreateTenant(ctx, CreateTenantInput{
		Principal: internalauth.Principal{
			UserID: domain.UserID("owner_1"),
			Email:  "owner@example.test",
		},
		Name: "Active Workspace",
	}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID("doc_active"),
		TenantID:   domain.TenantID("tenant_fixed"),
		OwnerID:    domain.UserID("owner_1"),
		Name:       "active.md",
		StorageKey: "tenants/tenant_fixed/documents/doc_active/active.md",
		SizeBytes:  12,
		Now:        fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	if err := repos.SaveDocument(ctx, document); err != nil {
		t.Fatalf("save document: %v", err)
	}
	_, err = service.DeleteTenant(ctx, DeleteTenantInput{
		TenantID: domain.TenantID("tenant_fixed"),
	})
	if !errors.Is(err, ErrTenantNotEmpty) {
		t.Fatalf("err = %v, want ErrTenantNotEmpty", err)
	}
	if err := document.Transition(domain.DocumentStatusDeleted, fixedClock{}.Now()); err != nil {
		t.Fatalf("delete document transition: %v", err)
	}
	if err := repos.SaveDocument(ctx, document); err != nil {
		t.Fatalf("save deleted document: %v", err)
	}
	source, err := domain.NewDataSource(domain.DataSourceCreate{
		ID:       domain.DataSourceID("src_active"),
		TenantID: domain.TenantID("tenant_fixed"),
		OwnerID:  domain.UserID("owner_1"),
		Type:     domain.DataSourceTypeFolder,
		Name:     "Active Source",
		RootPath: "/sources/primary",
		Now:      fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if err := repos.SaveDataSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	_, err = service.DeleteTenant(ctx, DeleteTenantInput{
		TenantID: domain.TenantID("tenant_fixed"),
	})
	if !errors.Is(err, ErrTenantNotEmpty) {
		t.Fatalf("err = %v, want ErrTenantNotEmpty", err)
	}
}

type tenantIDs struct{}

func (tenantIDs) NewTenantID() domain.TenantID {
	return domain.TenantID("tenant_fixed")
}
