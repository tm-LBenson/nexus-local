package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

const (
	defaultAuditListLimit = 50
	maxAuditListLimit     = 200
)

type AuditIDs interface {
	NewAuditEventID() domain.AuditEventID
}

type AuditService struct {
	repos store.RepositorySet
	ids   AuditIDs
	clock Clock
}

type RecordAuditInput struct {
	TenantID     domain.TenantID
	ActorUserID  domain.UserID
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      domain.AuditOutcome
	Metadata     map[string]string
}

type RecordAuditResult struct {
	Event domain.AuditEvent
}

type ListAuditEventsInput struct {
	TenantID domain.TenantID
	Limit    int
}

type ListAuditEventsResult struct {
	Events []domain.AuditEvent
}

func NewAuditService(repos store.RepositorySet, ids AuditIDs, clock Clock) AuditService {
	return AuditService{repos: repos, ids: ids, clock: clock}
}

func (s AuditService) Record(ctx context.Context, input RecordAuditInput) (RecordAuditResult, error) {
	if err := ctx.Err(); err != nil {
		return RecordAuditResult{}, err
	}
	if s.repos == nil || s.ids == nil {
		return RecordAuditResult{}, fmt.Errorf("audit service: %w", domain.ErrInvalidEntity)
	}
	event, err := domain.NewAuditEvent(domain.AuditEventCreate{
		ID:           s.ids.NewAuditEventID(),
		TenantID:     input.TenantID,
		ActorUserID:  input.ActorUserID,
		Action:       input.Action,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		Outcome:      input.Outcome,
		Metadata:     input.Metadata,
		Now:          s.clock.Now(),
	})
	if err != nil {
		return RecordAuditResult{}, err
	}
	if err := s.repos.SaveAuditEvent(ctx, event); err != nil {
		return RecordAuditResult{}, err
	}
	return RecordAuditResult{Event: event}, nil
}

func (s AuditService) List(ctx context.Context, input ListAuditEventsInput) (ListAuditEventsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListAuditEventsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListAuditEventsResult{}, fmt.Errorf("audit events: %w", domain.ErrInvalidEntity)
	}
	events, err := s.repos.ListAuditEvents(ctx, input.TenantID, normalizeAuditLimit(input.Limit))
	if err != nil {
		return ListAuditEventsResult{}, err
	}
	return ListAuditEventsResult{Events: events}, nil
}

func normalizeAuditLimit(limit int) int {
	if limit <= 0 {
		return defaultAuditListLimit
	}
	if limit > maxAuditListLimit {
		return maxAuditListLimit
	}
	return limit
}
