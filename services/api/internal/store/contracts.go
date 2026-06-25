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
	ListMembershipsForTenant(ctx context.Context, tenantID domain.TenantID) ([]domain.Membership, error)
	DeleteMembership(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) error
}

type DocumentRepository interface {
	SaveDocument(ctx context.Context, document domain.Document) error
	GetDocument(ctx context.Context, tenantID domain.TenantID, id domain.DocumentID) (domain.Document, error)
	ListDocuments(ctx context.Context, tenantID domain.TenantID) ([]domain.Document, error)
}

type JobRepository interface {
	SaveJob(ctx context.Context, job domain.Job) error
	GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error)
	ListJobs(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.Job, error)
	ClaimNextQueuedJob(ctx context.Context, now time.Time) (domain.Job, error)
}

type ConversationRepository interface {
	SaveConversation(ctx context.Context, conversation domain.Conversation) error
	GetConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) (domain.Conversation, error)
	ListConversations(ctx context.Context, tenantID domain.TenantID) ([]domain.Conversation, error)
	DeleteConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) error
	SaveMessage(ctx context.Context, message domain.Message) error
	ListMessages(ctx context.Context, tenantID domain.TenantID, conversationID domain.ConversationID) ([]domain.Message, error)
}

type RepositorySet interface {
	TenantRepository
	UserRepository
	MembershipRepository
	DocumentRepository
	JobRepository
	ConversationRepository
}
