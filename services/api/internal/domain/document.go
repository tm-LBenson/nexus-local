package domain

import (
	"fmt"
	"strings"
	"time"
)

type DocumentStatus string

const (
	DocumentStatusUploaded   DocumentStatus = "uploaded"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusReady      DocumentStatus = "ready"
	DocumentStatusFailed     DocumentStatus = "failed"
	DocumentStatusDeleted    DocumentStatus = "deleted"
)

type Document struct {
	ID         DocumentID
	TenantID   TenantID
	OwnerID    UserID
	Name       string
	StorageKey string
	SizeBytes  int64
	Status     DocumentStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DocumentCreate struct {
	ID         DocumentID
	TenantID   TenantID
	OwnerID    UserID
	Name       string
	StorageKey string
	SizeBytes  int64
	Now        time.Time
}

func NewDocument(input DocumentCreate) (Document, error) {
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.OwnerID)) ||
		strings.TrimSpace(input.Name) == "" ||
		strings.TrimSpace(input.StorageKey) == "" ||
		input.SizeBytes < 0 {
		return Document{}, fmt.Errorf("document: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Document{
		ID:         input.ID,
		TenantID:   input.TenantID,
		OwnerID:    input.OwnerID,
		Name:       strings.TrimSpace(input.Name),
		StorageKey: strings.TrimSpace(input.StorageKey),
		SizeBytes:  input.SizeBytes,
		Status:     DocumentStatusUploaded,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (d *Document) Transition(next DocumentStatus, now time.Time) error {
	if !d.Status.CanTransition(next) {
		return fmt.Errorf("%s -> %s: %w", d.Status, next, ErrInvalidStateTransition)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	d.Status = next
	d.UpdatedAt = now
	return nil
}

func (s DocumentStatus) CanTransition(next DocumentStatus) bool {
	switch s {
	case DocumentStatusUploaded:
		return next == DocumentStatusProcessing ||
			next == DocumentStatusDeleted ||
			next == DocumentStatusFailed
	case DocumentStatusProcessing:
		return next == DocumentStatusReady ||
			next == DocumentStatusFailed ||
			next == DocumentStatusDeleted
	case DocumentStatusReady:
		return next == DocumentStatusDeleted
	case DocumentStatusFailed:
		return next == DocumentStatusProcessing ||
			next == DocumentStatusDeleted
	default:
		return false
	}
}
