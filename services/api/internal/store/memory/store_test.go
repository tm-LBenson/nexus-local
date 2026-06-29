package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

func TestDocumentsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := New()

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

func TestGetDocumentRequiresMatchingTenant(t *testing.T) {
	ctx := context.Background()
	repo := New()
	doc := newTestDocument(t, domain.TenantID("tenant_a"), domain.DocumentID("doc_1"))
	if err := repo.SaveDocument(ctx, doc); err != nil {
		t.Fatalf("save doc: %v", err)
	}

	_, err := repo.GetDocument(ctx, domain.TenantID("tenant_b"), domain.DocumentID("doc_1"))
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMembershipsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := New()

	for _, userID := range []domain.UserID{"user_a", "user_b"} {
		if err := repo.SaveUser(ctx, newTestUser(t, userID, string(userID)+"@example.test")); err != nil {
			t.Fatalf("save user %s: %v", userID, err)
		}
	}
	memberships := []domain.Membership{
		newTestMembership(t, domain.TenantID("tenant_a"), domain.UserID("user_b"), domain.RoleMember),
		newTestMembership(t, domain.TenantID("tenant_a"), domain.UserID("user_a"), domain.RoleOwner),
		newTestMembership(t, domain.TenantID("tenant_b"), domain.UserID("user_a"), domain.RoleViewer),
	}
	for _, membership := range memberships {
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

func TestClaimNextQueuedJobTransitionsJob(t *testing.T) {
	ctx := context.Background()
	repo := New()
	job, err := domain.NewJob(domain.JobCreate{
		ID:       domain.JobID("job_1"),
		TenantID: domain.TenantID("tenant_a"),
		Type:     domain.JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	job.ErrorMessage = "old failure"
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save updated job: %v", err)
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
	if claimed.ErrorMessage != "" {
		t.Fatalf("error message = %q, want empty after claim", claimed.ErrorMessage)
	}
}

func TestListJobsReturnsRecentTenantJobs(t *testing.T) {
	ctx := context.Background()
	repo := New()
	first := newTestJob(t, domain.TenantID("tenant_a"), domain.JobID("job_1"), fixedTime())
	second := newTestJob(t, domain.TenantID("tenant_a"), domain.JobID("job_2"), fixedTime().Add(time.Minute))
	otherTenant := newTestJob(t, domain.TenantID("tenant_b"), domain.JobID("job_3"), fixedTime().Add(2*time.Minute))

	for _, job := range []domain.Job{first, second, otherTenant} {
		if err := repo.SaveJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", job.ID, err)
		}
	}

	jobs, err := repo.ListJobs(ctx, domain.TenantID("tenant_a"), 1)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(jobs))
	}
	if jobs[0].ID != domain.JobID("job_2") {
		t.Fatalf("job id = %s, want job_2", jobs[0].ID)
	}
}

func TestClaimNextQueuedJobIgnoresTerminalJobs(t *testing.T) {
	ctx := context.Background()
	repo := New()
	job, err := domain.NewJob(domain.JobCreate{
		ID:       domain.JobID("job_1"),
		TenantID: domain.TenantID("tenant_a"),
		Type:     domain.JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := job.Transition(domain.JobStateRunning, fixedTime()); err != nil {
		t.Fatalf("running: %v", err)
	}
	if err := job.Transition(domain.JobStateSucceeded, fixedTime()); err != nil {
		t.Fatalf("succeeded: %v", err)
	}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	_, err = repo.ClaimNextQueuedJob(ctx, fixedTime())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConversationsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repo := New()

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
	if len(conversations) != 1 {
		t.Fatalf("conversations len = %d, want 1", len(conversations))
	}
	if conversations[0].TenantID != domain.TenantID("tenant_a") {
		t.Fatalf("tenant = %q, want tenant_a", conversations[0].TenantID)
	}

	_, err = repo.GetConversation(ctx, domain.TenantID("tenant_b"), domain.ConversationID("conv_missing"))
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMessagesAreConversationScopedAndOrdered(t *testing.T) {
	ctx := context.Background()
	repo := New()

	conversation := newTestConversation(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), fixedTime())
	if err := repo.SaveConversation(ctx, conversation); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	second := newTestMessage(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), domain.MessageID("msg_2"), domain.MessageRoleAssistant, "second", fixedTime().Add(time.Minute))
	first := newTestMessage(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), domain.MessageID("msg_1"), domain.MessageRoleUser, "first", fixedTime())
	other := newTestMessage(t, domain.TenantID("tenant_b"), domain.ConversationID("conv_1"), domain.MessageID("msg_3"), domain.MessageRoleUser, "other tenant", fixedTime())

	for _, message := range []domain.Message{second, first, other} {
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

func TestDeleteConversationRemovesMessages(t *testing.T) {
	ctx := context.Background()
	repo := New()

	conversation := newTestConversation(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), fixedTime())
	if err := repo.SaveConversation(ctx, conversation); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	message := newTestMessage(t, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"), domain.MessageID("msg_1"), domain.MessageRoleUser, "first", fixedTime())
	if err := repo.SaveMessage(ctx, message); err != nil {
		t.Fatalf("save message: %v", err)
	}

	if err := repo.DeleteConversation(ctx, domain.TenantID("tenant_a"), domain.ConversationID("conv_1")); err != nil {
		t.Fatalf("delete conversation: %v", err)
	}
	if _, err := repo.GetConversation(ctx, domain.TenantID("tenant_a"), domain.ConversationID("conv_1")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	messages, err := repo.ListMessages(ctx, domain.TenantID("tenant_a"), domain.ConversationID("conv_1"))
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages len = %d, want 0", len(messages))
	}
	if err := repo.DeleteConversation(ctx, domain.TenantID("tenant_a"), domain.ConversationID("missing")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAuditEventsAreTenantScopedAndRecentFirst(t *testing.T) {
	ctx := context.Background()
	repo := New()

	first := newTestAuditEvent(t, domain.TenantID("tenant_a"), domain.AuditEventID("audit_1"), fixedTime())
	second := newTestAuditEvent(t, domain.TenantID("tenant_a"), domain.AuditEventID("audit_2"), fixedTime().Add(time.Minute))
	otherTenant := newTestAuditEvent(t, domain.TenantID("tenant_b"), domain.AuditEventID("audit_3"), fixedTime().Add(2*time.Minute))
	for _, event := range []domain.AuditEvent{first, second, otherTenant} {
		if err := repo.SaveAuditEvent(ctx, event); err != nil {
			t.Fatalf("save audit %s: %v", event.ID, err)
		}
	}

	events, err := repo.ListAuditEvents(ctx, domain.TenantID("tenant_a"), 1)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ID != domain.AuditEventID("audit_2") {
		t.Fatalf("event id = %s, want audit_2", events[0].ID)
	}
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

func newTestJob(t *testing.T, tenantID domain.TenantID, jobID domain.JobID, now time.Time) domain.Job {
	t.Helper()

	job, err := domain.NewJob(domain.JobCreate{
		ID:           jobID,
		TenantID:     tenantID,
		Type:         domain.JobTypeDocumentIngestion,
		ResourceType: "document",
		ResourceID:   "doc_1",
		Now:          now,
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	return job
}

func newTestAuditEvent(t *testing.T, tenantID domain.TenantID, eventID domain.AuditEventID, now time.Time) domain.AuditEvent {
	t.Helper()

	event, err := domain.NewAuditEvent(domain.AuditEventCreate{
		ID:           eventID,
		TenantID:     tenantID,
		ActorUserID:  domain.UserID("user_1"),
		Action:       "search.completed",
		ResourceType: "search",
		Outcome:      domain.AuditOutcomeSucceeded,
		Now:          now,
	})
	if err != nil {
		t.Fatalf("new audit event: %v", err)
	}
	return event
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
