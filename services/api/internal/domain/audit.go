package domain

import (
	"fmt"
	"strings"
	"time"
)

type AuditOutcome string

const (
	AuditOutcomeSucceeded AuditOutcome = "succeeded"
	AuditOutcomeFailed    AuditOutcome = "failed"
	AuditOutcomeDenied    AuditOutcome = "denied"
)

type AuditEvent struct {
	ID           AuditEventID
	TenantID     TenantID
	ActorUserID  UserID
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      AuditOutcome
	Metadata     map[string]string
	CreatedAt    time.Time
}

type AuditEventCreate struct {
	ID           AuditEventID
	TenantID     TenantID
	ActorUserID  UserID
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      AuditOutcome
	Metadata     map[string]string
	Now          time.Time
}

func NewAuditEvent(input AuditEventCreate) (AuditEvent, error) {
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.ActorUserID)) ||
		strings.TrimSpace(input.Action) == "" ||
		!input.Outcome.Valid() {
		return AuditEvent{}, fmt.Errorf("audit event: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	metadata := map[string]string{}
	for key, value := range input.Metadata {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = strings.TrimSpace(value)
	}

	return AuditEvent{
		ID:           input.ID,
		TenantID:     input.TenantID,
		ActorUserID:  input.ActorUserID,
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		Outcome:      input.Outcome,
		Metadata:     metadata,
		CreatedAt:    now,
	}, nil
}

func (o AuditOutcome) Valid() bool {
	switch o {
	case AuditOutcomeSucceeded, AuditOutcomeFailed, AuditOutcomeDenied:
		return true
	default:
		return false
	}
}
