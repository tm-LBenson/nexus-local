package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestAuthenticatorUsesDevPrincipalFallback(t *testing.T) {
	authenticator := NewAuthenticator(config.Config{
		AuthMode:            ModeDev,
		DevUserID:           "user_dev",
		DevUserEmail:        "dev@example.local",
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	})

	principal, err := authenticator.Authenticate(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.UserID != domain.UserID("user_dev") {
		t.Fatalf("user id = %q, want user_dev", principal.UserID)
	}
}

func TestAuthenticatorRequiresTrustedHeaderUser(t *testing.T) {
	authenticator := NewAuthenticator(config.Config{
		AuthMode:            ModeTrustedHeader,
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	})

	_, err := authenticator.Authenticate(httptest.NewRequest("GET", "/", nil))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-ID", "user_1")
	req.Header.Set("X-User-Email", "user@example.test")
	principal, err := authenticator.Authenticate(req)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.UserID != domain.UserID("user_1") || principal.Email != "user@example.test" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestAuthorizerRequiresTenantMembershipPermission(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	membership, err := domain.NewMembership(domain.MembershipCreate{
		TenantID: domain.TenantID("tenant_1"),
		UserID:   domain.UserID("user_1"),
		Role:     domain.RoleViewer,
	})
	if err != nil {
		t.Fatalf("new membership: %v", err)
	}
	if err := repos.SaveMembership(ctx, membership); err != nil {
		t.Fatalf("save membership: %v", err)
	}
	authorizer := NewAuthorizer(config.Config{AuthMode: ModeTrustedHeader}, repos)

	err = authorizer.Require(ctx, Principal{UserID: domain.UserID("user_1")}, domain.TenantID("tenant_1"), domain.PermissionReadDocuments)
	if err != nil {
		t.Fatalf("require read: %v", err)
	}

	err = authorizer.Require(ctx, Principal{UserID: domain.UserID("user_1")}, domain.TenantID("tenant_1"), domain.PermissionUploadDocuments)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestAuthorizerAllowsDevModeWithoutMembership(t *testing.T) {
	authorizer := NewAuthorizer(config.Config{AuthMode: ModeDev}, memory.New())

	err := authorizer.Require(context.Background(), Principal{UserID: domain.UserID("user_dev")}, domain.TenantID("tenant_any"), domain.PermissionManageTenant)
	if err != nil {
		t.Fatalf("require in dev: %v", err)
	}
}
