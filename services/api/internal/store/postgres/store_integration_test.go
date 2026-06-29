package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

func TestStoreIntegrationDocumentsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)

	docA := newTestDocument(t, domain.TenantID("tenant_a"), domain.DocumentID("doc_1"))
	docB := newTestDocument(t, domain.TenantID("tenant_b"), domain.DocumentID("doc_1"))

	if err := repo.SaveDocument(ctx, docA); err != nil {
		t.Fatalf("save docA: %v", err)
	}
	if err := repo.SaveDocument(ctx, docB); err != nil {
		t.Fatalf("save docB: %v", err)
	}

	docs, err := repo.ListDocuments(ctx, domain.TenantID("tenant_a"))
	if err != nil {
		t.Fatalf("list docs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	if docs[0].TenantID != domain.TenantID("tenant_a") {
		t.Fatalf("tenant = %q, want tenant_a", docs[0].TenantID)
	}
}

func TestStoreIntegrationMembershipsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)

	for _, userID := range []domain.UserID{"user_a", "user_b"} {
		if err := repo.SaveUser(ctx, newTestUser(t, userID, string(userID)+"@example.test")); err != nil {
			t.Fatalf("save user %s: %v", userID, err)
		}
	}
	for _, membership := range []domain.Membership{
		newTestMembership(t, domain.TenantID("tenant_a"), domain.UserID("user_b"), domain.RoleMember),
		newTestMembership(t, domain.TenantID("tenant_a"), domain.UserID("user_a"), domain.RoleOwner),
		newTestMembership(t, domain.TenantID("tenant_b"), domain.UserID("user_a"), domain.RoleViewer),
	} {
		if err := repo.SaveMembership(ctx, membership); err != nil {
			t.Fatalf("save membership %s/%s: %v", membership.TenantID, membership.UserID, err)
		}
	}

	got, err := repo.ListMembershipsForTenant(ctx, domain.TenantID("tenant_a"))
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("memberships len = %d, want 2", len(got))
	}
	if got[0].UserID != domain.UserID("user_a") || got[1].UserID != domain.UserID("user_b") {
		t.Fatalf("membership order = %s, %s; want user_a, user_b", got[0].UserID, got[1].UserID)
	}
	if err := repo.DeleteMembership(ctx, domain.TenantID("tenant_a"), domain.UserID("user_b")); err != nil {
		t.Fatalf("delete membership: %v", err)
	}
	got, err = repo.ListMembershipsForTenant(ctx, domain.TenantID("tenant_a"))
	if err != nil {
		t.Fatalf("list memberships after delete: %v", err)
	}
	if len(got) != 1 || got[0].UserID != domain.UserID("user_a") {
		t.Fatalf("memberships after delete = %#v", got)
	}
	if err := repo.DeleteMembership(ctx, domain.TenantID("tenant_a"), domain.UserID("missing")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreIntegrationClaimNextQueuedJob(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)
	job, err := domain.NewJob(domain.JobCreate{
		ID:           domain.JobID("job_1"),
		TenantID:     domain.TenantID("tenant_a"),
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   "doc_1",
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	jobs, err := repo.ListJobs(ctx, domain.TenantID("tenant_a"), 10)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != domain.JobID("job_1") {
		t.Fatalf("jobs = %#v", jobs)
	}

	claimed, err := repo.ClaimNextQueuedJob(ctx, fixedTime().Add(time.Minute))
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed.State != domain.JobStateRunning {
		t.Fatalf("state = %q, want running", claimed.State)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", claimed.Attempts)
	}
	if claimed.ResourceType != "document" || claimed.ResourceID != "doc_1" {
		t.Fatalf("resource = %s/%s, want document/doc_1", claimed.ResourceType, claimed.ResourceID)
	}

	_, err = repo.ClaimNextQueuedJob(ctx, fixedTime())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreIntegrationConversationsAndMessages(t *testing.T) {
	ctx := context.Background()
	repo := newIntegrationStore(t, ctx)

	conversationA := newTestConversation(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), fixedTime())
	conversationB := newTestConversation(t, domain.TenantID("tenant_b"), domain.ConversationID("conv_1"), fixedTime().Add(time.Minute))
	if err := repo.SaveConversation(ctx, conversationA); err != nil {
		t.Fatalf("save conversationA: %v", err)
	}
	if err := repo.SaveConversation(ctx, conversationB); err != nil {
		t.Fatalf("save conversationB: %v", err)
	}

	conversations, err := repo.ListConversations(ctx, domain.TenantID("tenant_a"))
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 || conversations[0].TenantID != domain.TenantID("tenant_a") {
		t.Fatalf("conversations = %#v", conversations)
	}

	second := newTestMessage(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), domain.MessageID("msg_2"), domain.MessageRoleAssistant, "second", fixedTime().Add(time.Minute))
	first := newTestMessage(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), domain.MessageID("msg_1"), domain.MessageRoleUser, "first", fixedTime())
	for _, message := range []domain.Message{second, first} {
		if err := repo.SaveMessage(ctx, message); err != nil {
			t.Fatalf("save message %s: %v", message.ID, err)
		}
	}

	messages, err := repo.ListMessages(ctx, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"))
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[0].ID != domain.MessageID("msg_1") || messages[1].ID != domain.MessageID("msg_2") {
		t.Fatalf("message order = %s, %s; want msg_1, msg_2", messages[0].ID, messages[1].ID)
	}
}

func newIntegrationStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Postgres integration tests")
	}

	repo, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(repo.Close)

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `
		TRUNCATE tenants, users, memberships, documents, jobs, messages, conversations, audit_events, data_sources, data_source_scan_entries
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return repo
}

func newTestDocument(t *testing.T, tenantID domain.TenantID, documentID domain.DocumentID) domain.Document {
	t.Helper()

	doc, err := domain.NewDocument(domain.DocumentCreate{
		ID:         documentID,
		TenantID:   tenantID,
		OwnerID:    domain.UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/" + string(tenantID) + "/documents/" + string(documentID),
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	return doc
}

func newTestUser(t *testing.T, userID domain.UserID, email string) domain.User {
	t.Helper()

	user, err := domain.NewUser(domain.UserCreate{
		ID:    userID,
		Email: email,
		Name:  string(userID),
		Now:   fixedTime(),
	})
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	return user
}

func newTestMembership(t *testing.T, tenantID domain.TenantID, userID domain.UserID, role domain.Role) domain.Membership {
	t.Helper()

	membership, err := domain.NewMembership(domain.MembershipCreate{
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	})
	if err != nil {
		t.Fatalf("new membership: %v", err)
	}
	return membership
}

func newTestConversation(t *testing.T, tenantID domain.TenantID, conversationID domain.ConversationID, now time.Time) domain.Conversation {
	t.Helper()

	conversation, err := domain.NewConversation(domain.ConversationCreate{
		ID:          conversationID,
		TenantID:    tenantID,
		OwnerID:     domain.UserID("user_1"),
		Title:       "Research",
		ModelTarget: "general",
		Now:         now,
	})
	if err != nil {
		t.Fatalf("new conversation: %v", err)
	}
	return conversation
}

func newTestMessage(t *testing.T, tenantID domain.TenantID, conversationID domain.ConversationID, messageID domain.MessageID, role domain.MessageRole, content string, now time.Time) domain.Message {
	t.Helper()

	message, err := domain.NewMessage(domain.MessageCreate{
		ID:             messageID,
		TenantID:       tenantID,
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return message
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
