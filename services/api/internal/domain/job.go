package domain

import (
	"fmt"
	"strings"
	"time"
)

type JobType string

const (
	JobTypeDocumentIngestion JobType = "document_ingestion"
	JobTypeSourceScan        JobType = "source_scan"
	JobTypeSourcePreflight   JobType = "source_preflight"
	JobTypeSourcePlan        JobType = "source_plan"
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
	ID           JobID
	TenantID     TenantID
	Type         JobType
	ResourceType string
	ResourceID   string
	State        JobState
	Attempts     int
	ErrorMessage string
	ResultJSON   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type JobCreate struct {
	ID           JobID
	TenantID     TenantID
	Type         JobType
	ResourceType string
	ResourceID   string
	Now          time.Time
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
		ID:           input.ID,
		TenantID:     input.TenantID,
		Type:         input.Type,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		State:        JobStateQueued,
		CreatedAt:    now,
		UpdatedAt:    now,
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
	if next == JobStateQueued || next == JobStateRunning || next == JobStateSucceeded {
		j.ErrorMessage = ""
	}
	if next == JobStateQueued || next == JobStateRunning {
		j.ResultJSON = ""
	}
	j.State = next
	j.UpdatedAt = now
	return nil
}

func (j *Job) Fail(cause error, now time.Time) error {
	if err := j.Transition(JobStateFailed, now); err != nil {
		return err
	}
	message := "unknown failure"
	if cause != nil {
		message = strings.TrimSpace(cause.Error())
	}
	if message == "" {
		message = "unknown failure"
	}
	j.ErrorMessage = message
	return nil
}

func (t JobType) Valid() bool {
	switch t {
	case JobTypeDocumentIngestion, JobTypeSourceScan, JobTypeSourcePreflight, JobTypeSourcePlan, JobTypeEmbeddingBackfill, JobTypeAITurn, JobTypeEvaluation:
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
