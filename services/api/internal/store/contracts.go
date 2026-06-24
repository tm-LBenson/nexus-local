package store

import (
	"context"
	"errors"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

var ErrNotFound = errors.New("not found")

type TenantRepository interface {
	SaveTenant(ctx context.Context, tenant domain.Tenant) error
	GetTenant(ctx context.Context, id domain.TenantID) (domain.Tenant, error)
}

type UserRepository interface {
	SaveUser(ctx context.Context, user domain.User) error
	GetUser(ctx context.Context, id domain.UserID) (domain.User, error)
}

type MembershipRepository interface {
	SaveMembership(ctx context.Context, membership domain.Membership) error
	ListMembershipsForUser(ctx context.Context, userID domain.UserID) ([]domain.Membership, error)
}

type DocumentRepository interface {
	SaveDocument(ctx context.Context, document domain.Document) error
	GetDocument(ctx context.Context, tenantID domain.TenantID, id domain.DocumentID) (domain.Document, error)
	ListDocuments(ctx context.Context, tenantID domain.TenantID) ([]domain.Document, error)
}

type JobRepository interface {
	SaveJob(ctx context.Context, job domain.Job) error
	GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error)
	ClaimNextQueuedJob(ctx context.Context, now time.Time) (domain.Job, error)
}

type RepositorySet interface {
	TenantRepository
	UserRepository
	MembershipRepository
	DocumentRepository
	JobRepository
}
