package domain

import (
	"errors"
	"testing"
	"time"
)

func TestJobTransitionAllowsExpectedFlow(t *testing.T) {
	job, err := NewJob(JobCreate{
		ID:       JobID("job_1"),
		TenantID: TenantID("tenant_1"),
		Type:     JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}

	if job.State != JobStateQueued {
		t.Fatalf("state = %q, want queued", job.State)
	}

	if err := job.Transition(JobStateRunning, fixedTime().Add(time.Second)); err != nil {
		t.Fatalf("queued -> running: %v", err)
	}
	if err := job.Transition(JobStateSucceeded, fixedTime().Add(2*time.Second)); err != nil {
		t.Fatalf("running -> succeeded: %v", err)
	}
}

func TestJobTransitionRejectsTerminalTransition(t *testing.T) {
	job, err := NewJob(JobCreate{
		ID:       JobID("job_1"),
		TenantID: TenantID("tenant_1"),
		Type:     JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := job.Transition(JobStateRunning, fixedTime()); err != nil {
		t.Fatalf("queued -> running: %v", err)
	}
	if err := job.Transition(JobStateFailed, fixedTime()); err != nil {
		t.Fatalf("running -> failed: %v", err)
	}

	err = job.Transition(JobStateRunning, fixedTime())
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("err = %v, want ErrInvalidStateTransition", err)
	}
}

func TestJobRequiresTenantAndType(t *testing.T) {
	_, err := NewJob(JobCreate{
		ID:  JobID("job_1"),
		Now: fixedTime(),
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
