package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
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
		DatabaseURL:         "postgres://test",
		ObjectStoreEndpoint: "http://minio.test",
		VectorBackend:       "qdrant",
		QueueBackend:        "nats",
		ModelGatewayBaseURL: "http://gpu.local:8000/v1",
	}

	repos := memory.New()
	documents := app.NewDocumentService(repos, httpIDs{}, httpClock{})

	return NewRouter(cfg, Dependencies{
		ModelRouter: router,
		Documents:   documents,
	})
}

type httpIDs struct{}

func (httpIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_http")
}

func (httpIDs) NewJobID() domain.JobID {
	return domain.JobID("job_http")
}

type httpClock struct{}

func (httpClock) Now() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}
