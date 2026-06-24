package providers

import (
	"context"
	"io"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

type ObjectPut struct {
	TenantID    domain.TenantID
	Key         string
	Body        io.Reader
	ContentType string
	SizeBytes   int64
}

type ObjectInfo struct {
	Key         string
	ContentType string
	SizeBytes   int64
	UpdatedAt   time.Time
}

type PresignRequest struct {
	Key       string
	TTL       time.Duration
	Operation string
}

type ObjectStore interface {
	PutObject(ctx context.Context, input ObjectPut) (ObjectInfo, error)
	GetObject(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, ObjectInfo, error)
	DeleteObject(ctx context.Context, tenantID domain.TenantID, key string) error
	PresignURL(ctx context.Context, input PresignRequest) (string, error)
}

type Vector struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
	ChunkID    string
	Values     []float32
	Text       string
	Metadata   map[string]string
}

type VectorSearch struct {
	TenantID domain.TenantID
	Query    []float32
	Limit    int
	Filters  map[string]string
}

type VectorHit struct {
	DocumentID domain.DocumentID
	ChunkID    string
	Text       string
	Score      float32
	Metadata   map[string]string
}

type VectorIndex interface {
	Upsert(ctx context.Context, vectors []Vector) error
	Search(ctx context.Context, input VectorSearch) ([]VectorHit, error)
	DeleteDocument(ctx context.Context, tenantID domain.TenantID, documentID domain.DocumentID) error
}

type JobEnvelope struct {
	ID       domain.JobID
	TenantID domain.TenantID
	Type     domain.JobType
	Payload  []byte
}

type JobQueue interface {
	Publish(ctx context.Context, job JobEnvelope) error
	Ack(ctx context.Context, jobID domain.JobID) error
	Nack(ctx context.Context, jobID domain.JobID, retryAfter time.Duration) error
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Target      string            `json:"target"`
	Model       string            `json:"model,omitempty"`
	Messages    []ChatMessage     `json:"messages"`
	Temperature float32           `json:"temperature,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ChatCompletion struct {
	Model        string            `json:"model"`
	Content      string            `json:"content"`
	FinishReason string            `json:"finish_reason"`
	Usage        map[string]int    `json:"usage,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type ModelGateway interface {
	Complete(ctx context.Context, input ChatCompletionRequest) (ChatCompletion, error)
}
