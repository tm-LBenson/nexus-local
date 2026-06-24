package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
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
		DatabaseURL:         "postgres://test",
		ObjectStoreEndpoint: "http://minio.test",
		VectorBackend:       "qdrant",
		QueueBackend:        "nats",
		ModelGatewayBaseURL: "http://gpu.local:8000/v1",
	}

	return NewRouter(cfg, router)
}
