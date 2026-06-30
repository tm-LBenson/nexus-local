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
	DeleteTenant(ctx context.Context, id domain.TenantID) error
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

type DataSourceRepository interface {
	SaveDataSource(ctx context.Context, source domain.DataSource) error
	GetDataSource(ctx context.Context, tenantID domain.TenantID, id domain.DataSourceID) (domain.DataSource, error)
	ListDataSources(ctx context.Context, tenantID domain.TenantID) ([]domain.DataSource, error)
	ListDueDataSources(ctx context.Context, now time.Time, limit int) ([]domain.DataSource, error)
	SaveDataSourceScanEntry(ctx context.Context, entry domain.DataSourceScanEntry) error
	ListDataSourceScanEntries(ctx context.Context, tenantID domain.TenantID, sourceID domain.DataSourceID, limit int) ([]domain.DataSourceScanEntry, error)
	ListDataSourceScanEntryPage(ctx context.Context, tenantID domain.TenantID, sourceID domain.DataSourceID, filter DataSourceScanEntryFilter) (DataSourceScanEntryPage, error)
}

type SourceViewRepository interface {
	SaveSourceView(ctx context.Context, view domain.SourceView) error
	GetSourceView(ctx context.Context, tenantID domain.TenantID, id domain.SourceViewID) (domain.SourceView, error)
	ListSourceViews(ctx context.Context, tenantID domain.TenantID) ([]domain.SourceView, error)
	DeleteSourceView(ctx context.Context, tenantID domain.TenantID, id domain.SourceViewID) error
}

type SourcePolicyProfileRepository interface {
	SaveSourcePolicyProfile(ctx context.Context, profile domain.SourcePolicyProfile) error
	GetSourcePolicyProfile(ctx context.Context, tenantID domain.TenantID, id domain.SourcePolicyProfileID) (domain.SourcePolicyProfile, error)
	ListSourcePolicyProfiles(ctx context.Context, tenantID domain.TenantID) ([]domain.SourcePolicyProfile, error)
	DeleteSourcePolicyProfile(ctx context.Context, tenantID domain.TenantID, id domain.SourcePolicyProfileID) error
}

type DataSourceScanEntryFilter struct {
	Outcome domain.DataSourceScanOutcome
	Limit   int
	Offset  int
}

type DataSourceScanEntryPage struct {
	Entries []domain.DataSourceScanEntry
	Total   int
	Limit   int
	Offset  int
}

type JobRepository interface {
	SaveJob(ctx context.Context, job domain.Job) error
	GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error)
	ListJobs(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.Job, error)
	ClaimNextQueuedJob(ctx context.Context, now time.Time, types ...domain.JobType) (domain.Job, error)
}

type ConversationRepository interface {
	SaveConversation(ctx context.Context, conversation domain.Conversation) error
	GetConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) (domain.Conversation, error)
	ListConversations(ctx context.Context, tenantID domain.TenantID) ([]domain.Conversation, error)
	DeleteConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) error
	SaveMessage(ctx context.Context, message domain.Message) error
	ListMessages(ctx context.Context, tenantID domain.TenantID, conversationID domain.ConversationID) ([]domain.Message, error)
}

type AuditRepository interface {
	SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, tenantID domain.TenantID, filter AuditEventFilter) ([]domain.AuditEvent, error)
}

type AuditEventFilter struct {
	Limit       int
	Action      string
	Outcome     domain.AuditOutcome
	ActorUserID domain.UserID
	Query       string
	From        *time.Time
	To          *time.Time
}

type RepositorySet interface {
	TenantRepository
	UserRepository
	MembershipRepository
	DocumentRepository
	DataSourceRepository
	SourceViewRepository
	SourcePolicyProfileRepository
	JobRepository
	ConversationRepository
	AuditRepository
}
