package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDocumentLifecycleAllowsReadyPath(t *testing.T) {
	doc, err := NewDocument(DocumentCreate{
		ID:         DocumentID("doc_1"),
		TenantID:   TenantID("tenant_1"),
		OwnerID:    UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/doc_1/source.md",
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}

	if doc.Status != DocumentStatusUploaded {
		t.Fatalf("status = %q, want uploaded", doc.Status)
	}
	if err := doc.Transition(DocumentStatusProcessing, fixedTime().Add(time.Second)); err != nil {
		t.Fatalf("uploaded -> processing: %v", err)
	}
	if err := doc.Transition(DocumentStatusReady, fixedTime().Add(2*time.Second)); err != nil {
		t.Fatalf("processing -> ready: %v", err)
	}
}

func TestDocumentLifecycleRejectsReadyToProcessing(t *testing.T) {
	doc, err := NewDocument(DocumentCreate{
		ID:         DocumentID("doc_1"),
		TenantID:   TenantID("tenant_1"),
		OwnerID:    UserID("user_1"),
		Name:       "Handbook.md",
		StorageKey: "tenants/tenant_1/documents/doc_1/source.md",
		SizeBytes:  42,
		Now:        fixedTime(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	if err := doc.Transition(DocumentStatusProcessing, fixedTime()); err != nil {
		t.Fatalf("uploaded -> processing: %v", err)
	}
	if err := doc.Transition(DocumentStatusReady, fixedTime()); err != nil {
		t.Fatalf("processing -> ready: %v", err)
	}

	err = doc.Transition(DocumentStatusProcessing, fixedTime())
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want ErrInvalidStateTransition", err)
	}
}

func TestDocumentRequiresStorageKey(t *testing.T) {
	_, err := NewDocument(DocumentCreate{
		ID:       DocumentID("doc_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "Handbook.md",
		Now:      fixedTime(),
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}
