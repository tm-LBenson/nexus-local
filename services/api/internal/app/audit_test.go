package app

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestAuditServiceRecordsAndListsEvents(t *testing.T) {
	ctx := context.Background()
	service := NewAuditService(memory.New(), auditIDs{}, auditClock{})

	if _, err := service.Record(ctx, RecordAuditInput{
		TenantID:     domain.TenantID("tenant_1"),
		ActorUserID:  domain.UserID("user_1"),
		Action:       "conversation.ask",
		ResourceType: "conversation",
		ResourceID:   "conv_1",
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			" model ": " general-model ",
		},
	}); err != nil {
		t.Fatalf("record audit: %v", err)
	}

	result, err := service.List(ctx, ListAuditEventsInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(result.Events))
	}
	event := result.Events[0]
	if event.ID != domain.AuditEventID("audit_fixed") {
		t.Fatalf("event id = %q", event.ID)
	}
	if event.Metadata["model"] != "general-model" {
		t.Fatalf("metadata = %#v", event.Metadata)
	}
}

type auditIDs struct{}

func (auditIDs) NewAuditEventID() domain.AuditEventID {
	return domain.AuditEventID("audit_fixed")
}

type auditClock struct{}

func (auditClock) Now() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
