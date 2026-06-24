package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	objectmemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/objectstore/memory"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestHealthCheck(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %v, want ok", body["status"])
	}
}

func TestReadinessIncludesPersistenceBackend(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["persistence_backend"] != "memory" {
		t.Fatalf("persistence_backend = %v, want memory", body["persistence_backend"])
	}
}

func TestModelRouteEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/models/route", bytes.NewBufferString(`{"target":"general"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Route providers.ModelRoute `json:"route"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Route.BaseURL != "http://gpu.local:8000/v1" {
		t.Fatalf("base url = %q", body.Route.BaseURL)
	}
}

func TestModelRouteEndpointRejectsUnknownTarget(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/models/route", bytes.NewBufferString(`{"target":"missing"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestRegisterDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"owner_id": "user_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	var body struct {
		Document documentPayload `json:"document"`
		Job      jobPayload      `json:"job"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Document.ID != "doc_http" {
		t.Fatalf("document id = %q, want doc_http", body.Document.ID)
	}
	if body.Job.State != string(domain.JobStateQueued) {
		t.Fatalf("job state = %q, want queued", body.Job.State)
	}
	if body.Job.ResourceType != "document" || body.Job.ResourceID != "doc_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_http", body.Job.ResourceType, body.Job.ResourceID)
	}
}

func TestRegisterDocumentEndpointRejectsInvalidInput(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{"tenant_id":"tenant_1"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestUploadDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tenant_id", "tenant_1"); err != nil {
		t.Fatalf("tenant field: %v", err)
	}
	if err := writer.WriteField("owner_id", "user_1"); err != nil {
		t.Fatalf("owner field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "Handbook.md")
	if err != nil {
		t.Fatalf("file field: %v", err)
	}
	if _, err := part.Write([]byte("hello world")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	var response struct {
		Document documentPayload `json:"document"`
		Job      jobPayload      `json:"job"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if response.Document.StorageKey != "tenants/tenant_1/documents/doc_http/Handbook.md" {
		t.Fatalf("storage key = %q", response.Document.StorageKey)
	}
	if response.Job.State != string(domain.JobStateQueued) {
		t.Fatalf("job state = %q, want queued", response.Job.State)
	}
	if response.Job.ResourceType != "document" || response.Job.ResourceID != "doc_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_http", response.Job.ResourceType, response.Job.ResourceID)
	}
}

func TestSearchEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"query": "alpha beta",
		"limit": 1
	}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Hits []searchHitPayload `json:"hits"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(body.Hits))
	}
	if body.Hits[0].DocumentID != "doc_search" {
		t.Fatalf("document id = %q, want doc_search", body.Hits[0].DocumentID)
	}
	if body.Hits[0].Metadata["section"] != "planning" {
		t.Fatalf("metadata = %#v", body.Hits[0].Metadata)
	}
}

func TestSearchEndpointRejectsInvalidInput(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{"tenant_id":"tenant_1"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestAskConversationEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"owner_id": "user_1",
		"question": "What is the alpha beta plan?",
		"limit": 1
	}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Conversation     conversationPayload `json:"conversation"`
		AssistantMessage messagePayload      `json:"assistant_message"`
		Hits             []searchHitPayload  `json:"hits"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Conversation.ID != "conv_http" {
		t.Fatalf("conversation id = %q, want conv_http", body.Conversation.ID)
	}
	if body.AssistantMessage.Content != "Answer from fake model" {
		t.Fatalf("assistant content = %q", body.AssistantMessage.Content)
	}
	if len(body.Hits) != 1 || body.Hits[0].DocumentID != "doc_search" {
		t.Fatalf("hits = %#v", body.Hits)
	}
}

func TestAskConversationEndpointRejectsInvalidInput(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{"tenant_id":"tenant_1"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestCORSPreflightForAllowedOrigin(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/models/route", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow origin = %q, want dev origin", got)
	}
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	router, err := providers.NewModelRouter([]providers.TargetConfig{
		{Name: "general", Provider: "openai-compatible", BaseURL: "http://gpu.local:8000/v1", Model: "general-model"},
	}, "general")
	if err != nil {
		t.Fatalf("model router: %v", err)
	}

	cfg := config.Config{
		Env:                 "test",
		Version:             "test",
		CORSAllowedOrigin:   "http://localhost:5173",
		PersistenceBackend:  "memory",
		RunMigrations:       false,
		DatabaseURL:         "postgres://test",
		ObjectStoreEndpoint: "http://minio.test",
		VectorBackend:       "qdrant",
		QueueBackend:        "nats",
		ModelGatewayBaseURL: "http://gpu.local:8000/v1",
	}

	repos := memory.New()
	ids := &httpIDs{}
	documents := app.NewDocumentService(repos, ids, httpClock{}).WithObjectStore(objectmemory.New())
	embedder := embeddinghash.New("test", 16)
	vectorIndex := vectormemory.New()
	seed, err := embedder.Embed(context.Background(), providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed seed: %v", err)
	}
	if err := vectorIndex.Upsert(context.Background(), []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_search"),
			ChunkID:    "chunk_1",
			Values:     seed.Vectors[0],
			Text:       "alpha beta launch plan",
			Metadata:   map[string]string{"section": "planning"},
		},
	}); err != nil {
		t.Fatalf("seed vectors: %v", err)
	}
	search := app.NewSearchService(embedder, vectorIndex)
	conversations := app.NewConversationService(repos, ids, httpClock{}, search, httpModelGateway{})

	return NewRouter(cfg, Dependencies{
		ModelRouter:   router,
		Documents:     documents,
		Search:        search,
		Conversations: conversations,
	})
}

type httpIDs struct {
	message int
}

func (httpIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_http")
}

func (httpIDs) NewJobID() domain.JobID {
	return domain.JobID("job_http")
}

func (httpIDs) NewConversationID() domain.ConversationID {
	return domain.ConversationID("conv_http")
}

func (g *httpIDs) NewMessageID() domain.MessageID {
	g.message++
	if g.message == 1 {
		return domain.MessageID("msg_user_http")
	}
	return domain.MessageID("msg_assistant_http")
}

type httpClock struct{}

func (httpClock) Now() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}

type httpModelGateway struct{}

func (httpModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	return providers.ChatCompletion{
		Model:        "fake-model",
		Content:      "Answer from fake model",
		FinishReason: "stop",
	}, nil
}
