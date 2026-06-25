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

func TestJobFailRecordsErrorMessageAndTransitionClearsIt(t *testing.T) {
	job, err := NewJob(JobCreate{
		ID:       JobID("job_1"),
		TenantID: TenantID("tenant_1"),
		Type:     JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := job.Transition(JobStateRunning, fixedTime().Add(time.Second)); err != nil {
		t.Fatalf("queued -> running: %v", err)
	}
	if err := job.Fail(errors.New("unsupported document type"), fixedTime().Add(2*time.Second)); err != nil {
		t.Fatalf("fail job: %v", err)
	}
	if job.State != JobStateFailed {
		t.Fatalf("state = %q, want failed", job.State)
	}
	if job.ErrorMessage != "unsupported document type" {
		t.Fatalf("error message = %q", job.ErrorMessage)
	}

	retry, err := NewJob(JobCreate{
		ID:       JobID("job_2"),
		TenantID: TenantID("tenant_1"),
		Type:     JobTypeDocumentIngestion,
		Now:      fixedTime(),
	})
	if err != nil {
		t.Fatalf("new retry job: %v", err)
	}
	retry.ErrorMessage = "old error"
	if err := retry.Transition(JobStateRunning, fixedTime().Add(time.Second)); err != nil {
		t.Fatalf("queued -> running: %v", err)
	}
	if retry.ErrorMessage != "" {
		t.Fatalf("running error message = %q, want empty", retry.ErrorMessage)
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
