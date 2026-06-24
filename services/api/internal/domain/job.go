package domain

import (
	"fmt"
	"time"
)

type JobType string

const (
	JobTypeDocumentIngestion JobType = "document_ingestion"
	JobTypeEmbeddingBackfill JobType = "embedding_backfill"
	JobTypeAITurn            JobType = "ai_turn"
	JobTypeEvaluation        JobType = "evaluation"
)

type JobState string

const (
	JobStateQueued    JobState = "queued"
	JobStateRunning   JobState = "running"
	JobStateRetrying  JobState = "retrying"
	JobStateSucceeded JobState = "succeeded"
	JobStateFailed    JobState = "failed"
	JobStateCanceled  JobState = "canceled"
)

type Job struct {
	ID        JobID
	TenantID  TenantID
	Type      JobType
	State     JobState
	Attempts  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobCreate struct {
	ID       JobID
	TenantID TenantID
	Type     JobType
	Now      time.Time
}

func NewJob(input JobCreate) (Job, error) {
	if emptyID(string(input.ID)) || emptyID(string(input.TenantID)) || !input.Type.Valid() {
		return Job{}, fmt.Errorf("job: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Job{
		ID:        input.ID,
		TenantID:  input.TenantID,
		Type:      input.Type,
		State:     JobStateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (j *Job) Transition(next JobState, now time.Time) error {
	if !j.State.CanTransition(next) {
		return fmt.Errorf("%s -> %s: %w", j.State, next, ErrInvalidStateTransition)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if next == JobStateRunning {
		j.Attempts++
	}
	j.State = next
	j.UpdatedAt = now
	return nil
}

func (t JobType) Valid() bool {
	switch t {
	case JobTypeDocumentIngestion, JobTypeEmbeddingBackfill, JobTypeAITurn, JobTypeEvaluation:
		return true
	default:
		return false
	}
}

func (s JobState) CanTransition(next JobState) bool {
	switch s {
	case JobStateQueued:
		return next == JobStateRunning || next == JobStateCanceled
	case JobStateRunning:
		return next == JobStateSucceeded ||
			next == JobStateFailed ||
			next == JobStateRetrying ||
			next == JobStateCanceled
	case JobStateRetrying:
		return next == JobStateQueued ||
			next == JobStateRunning ||
			next == JobStateFailed ||
			next == JobStateCanceled
	default:
		return false
	}
}
