package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

const (
	ModeDev           = "dev"
	ModeTrustedHeader = "trusted-header"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
)

type Principal struct {
	UserID domain.UserID
	Email  string
	Mode   string
}

type Authenticator struct {
	mode         string
	devUserID    domain.UserID
	devEmail     string
	userIDHeader string
	emailHeader  string
}

type Authorizer struct {
	mode  string
	repos store.MembershipRepository
}

type principalContextKey struct{}

func NewAuthenticator(cfg config.Config) Authenticator {
	return Authenticator{
		mode:         normalizeMode(cfg.AuthMode),
		devUserID:    domain.UserID(strings.TrimSpace(cfg.DevUserID)),
		devEmail:     strings.TrimSpace(cfg.DevUserEmail),
		userIDHeader: strings.TrimSpace(cfg.TrustedUserIDHeader),
		emailHeader:  strings.TrimSpace(cfg.TrustedEmailHeader),
	}
}

func NewAuthorizer(cfg config.Config, repos store.MembershipRepository) Authorizer {
	return Authorizer{
		mode:  normalizeMode(cfg.AuthMode),
		repos: repos,
	}
}

func (a Authenticator) Authenticate(r *http.Request) (Principal, error) {
	switch a.mode {
	case ModeDev:
		userID := domain.UserID(strings.TrimSpace(r.Header.Get(a.userIDHeader)))
		if userID == "" {
			userID = a.devUserID
		}
		if userID == "" {
			return Principal{}, ErrUnauthenticated
		}
		email := strings.TrimSpace(r.Header.Get(a.emailHeader))
		if email == "" {
			email = a.devEmail
		}
		return Principal{UserID: userID, Email: email, Mode: ModeDev}, nil
	case ModeTrustedHeader:
		userID := domain.UserID(strings.TrimSpace(r.Header.Get(a.userIDHeader)))
		if userID == "" {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{
			UserID: userID,
			Email:  strings.TrimSpace(r.Header.Get(a.emailHeader)),
			Mode:   ModeTrustedHeader,
		}, nil
	default:
		return Principal{}, fmt.Errorf("unsupported auth mode %q", a.mode)
	}
}

func (a Authorizer) Require(ctx context.Context, principal Principal, tenantID domain.TenantID, permission domain.Permission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(string(principal.UserID)) == "" {
		return ErrUnauthenticated
	}
	if strings.TrimSpace(string(tenantID)) == "" {
		return fmt.Errorf("tenant id is required: %w", domain.ErrInvalidEntity)
	}
	if a.mode == ModeDev {
		return nil
	}
	if a.repos == nil {
		return ErrForbidden
	}

	memberships, err := a.repos.ListMembershipsForUser(ctx, principal.UserID)
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if membership.TenantID == tenantID && membership.Role.Allows(permission) {
			return nil
		}
	}
	return ErrForbidden
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ModeDev
	}
	return mode
}
