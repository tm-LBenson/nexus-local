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
	TenantID  domain.TenantID
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

type EmbeddingRequest struct {
	Texts []string
	Model string
}

type EmbeddingResponse struct {
	Model   string
	Vectors [][]float32
}

type Embedder interface {
	Embed(ctx context.Context, input EmbeddingRequest) (EmbeddingResponse, error)
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

type ChatCompletionChunk struct {
	Model        string            `json:"model,omitempty"`
	Content      string            `json:"content,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	Usage        map[string]int    `json:"usage,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type ModelGateway interface {
	Complete(ctx context.Context, input ChatCompletionRequest) (ChatCompletion, error)
}

type StreamingModelGateway interface {
	ModelGateway
	StreamComplete(ctx context.Context, input ChatCompletionRequest, emit func(ChatCompletionChunk) error) (ChatCompletion, error)
}
