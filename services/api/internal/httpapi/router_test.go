package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
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
	if body["provider_preset"] != "starter" {
		t.Fatalf("provider_preset = %v, want starter", body["provider_preset"])
	}
	if body["deployment_profile"] != "cpu-lite" {
		t.Fatalf("deployment_profile = %v, want cpu-lite", body["deployment_profile"])
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

func TestModelTargetCheckEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/model-targets/check", bytes.NewBufferString(`{"tenant_id":"tenant_1","target":"general"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Status       string               `json:"status"`
		Route        providers.ModelRoute `json:"route"`
		Model        string               `json:"model"`
		FinishReason string               `json:"finish_reason"`
		LatencyMS    int64                `json:"latency_ms"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.Route.Target != "general" || body.Route.Model != "general-model" {
		t.Fatalf("route = %#v", body.Route)
	}
	if body.Model != "fake-model" || body.FinishReason != "stop" {
		t.Fatalf("completion summary = %#v", body)
	}
	if body.LatencyMS < 0 {
		t.Fatalf("latency_ms = %d, want non-negative", body.LatencyMS)
	}
}

func TestModelTargetCheckEndpointUsesTinyProbe(t *testing.T) {
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
		AuthMode:            internalauth.ModeDev,
		DevUserID:           "user_1",
		DevUserEmail:        "dev@example.local",
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	}
	gateway := &capturingModelGateway{}
	server := NewRouter(cfg, Dependencies{
		ModelRouter:   router,
		ModelGateway:  gateway,
		Authenticator: internalauth.NewAuthenticator(cfg),
		Authorizer:    internalauth.NewAuthorizer(cfg, memory.New()),
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/model-targets/check", bytes.NewBufferString(`{"target":"general"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if gateway.request.MaxTokens != 8 {
		t.Fatalf("max tokens = %d, want 8", gateway.request.MaxTokens)
	}
	if gateway.request.Metadata["purpose"] != "model_target_check" {
		t.Fatalf("metadata = %#v", gateway.request.Metadata)
	}
	if gateway.deadlineSeconds < 55 || gateway.deadlineSeconds > 60 {
		t.Fatalf("deadlineSeconds = %d, want about 60", gateway.deadlineSeconds)
	}
}

func TestModelTargetCheckEndpointMapsGatewayFailure(t *testing.T) {
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
		AuthMode:            internalauth.ModeDev,
		DevUserID:           "user_1",
		DevUserEmail:        "dev@example.local",
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	}

	server := NewRouter(cfg, Dependencies{
		ModelRouter:   router,
		ModelGateway:  failingModelGateway{},
		Authenticator: internalauth.NewAuthenticator(cfg),
		Authorizer:    internalauth.NewAuthorizer(cfg, memory.New()),
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/model-targets/check", bytes.NewBufferString(`{"target":"general"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusBadGateway, resp.Body.String())
	}
}

func TestCurrentUserEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		User        userPayload         `json:"user"`
		Memberships []membershipPayload `json:"memberships"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.User.ID != "user_1" {
		t.Fatalf("user id = %q, want user_1", body.User.ID)
	}
	if len(body.Memberships) != 0 {
		t.Fatalf("memberships len = %d, want 0", len(body.Memberships))
	}
}

func TestCreateTenantEndpointCreatesOwnerMembership(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"Research Lab"}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	var body struct {
		Tenant     tenantPayload     `json:"tenant"`
		Membership membershipPayload `json:"membership"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Tenant.ID != "tenant_http" {
		t.Fatalf("tenant id = %q, want tenant_http", body.Tenant.ID)
	}
	if body.Membership.Role != string(domain.RoleOwner) {
		t.Fatalf("role = %q, want owner", body.Membership.Role)
	}
}

func TestTenantMemberEndpoints(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"Research Lab"}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant_http/members", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	var listBody struct {
		Members []tenantMemberPayload `json:"members"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Members) != 1 || listBody.Members[0].Role != "owner" {
		t.Fatalf("members = %#v", listBody.Members)
	}

	add := httptest.NewRecorder()
	addReq := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant_http/members", bytes.NewBufferString(`{
		"user_id": "user_2",
		"email": "User2@Example.Test",
		"name": "User Two",
		"role": "viewer"
	}`))
	server.ServeHTTP(add, addReq)
	if add.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body = %s", add.Code, add.Body.String())
	}
	var addBody struct {
		Member tenantMemberPayload `json:"member"`
	}
	if err := json.NewDecoder(add.Body).Decode(&addBody); err != nil {
		t.Fatalf("decode add: %v", err)
	}
	if addBody.Member.User.Email != "user2@example.test" || addBody.Member.Role != "viewer" {
		t.Fatalf("member = %#v", addBody.Member)
	}

	remove := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodDelete, "/v1/tenants/tenant_http/members/user_2", nil)
	server.ServeHTTP(remove, removeReq)
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", remove.Code, remove.Body.String())
	}

	lastOwner := httptest.NewRecorder()
	lastOwnerReq := httptest.NewRequest(http.MethodDelete, "/v1/tenants/tenant_http/members/user_1", nil)
	server.ServeHTTP(lastOwner, lastOwnerReq)
	if lastOwner.Code != http.StatusConflict {
		t.Fatalf("last owner status = %d, want %d, body = %s", lastOwner.Code, http.StatusConflict, lastOwner.Body.String())
	}
}

func TestDeleteTenantEndpoint(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"Temporary Workspace"}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}

	remove := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodDelete, "/v1/tenants/tenant_http", nil)
	server.ServeHTTP(remove, removeReq)
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", remove.Code, remove.Body.String())
	}
	var body struct {
		Tenant tenantPayload `json:"tenant"`
	}
	if err := json.NewDecoder(remove.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete: %v", err)
	}
	if body.Tenant.Name != "Temporary Workspace" {
		t.Fatalf("tenant name = %q", body.Tenant.Name)
	}

	current := httptest.NewRecorder()
	currentReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	server.ServeHTTP(current, currentReq)
	if current.Code != http.StatusOK {
		t.Fatalf("current status = %d, body = %s", current.Code, current.Body.String())
	}
	var currentBody struct {
		Memberships []membershipPayload `json:"memberships"`
	}
	if err := json.NewDecoder(current.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode current user: %v", err)
	}
	if len(currentBody.Memberships) != 0 {
		t.Fatalf("memberships len = %d, want 0", len(currentBody.Memberships))
	}
}

func TestDeleteTenantEndpointRejectsActiveDocument(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"Temporary Workspace"}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_http",
		"name": "Active.md",
		"storage_key": "tenants/tenant_http/documents/doc_http/Active.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", register.Code, register.Body.String())
	}

	remove := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodDelete, "/v1/tenants/tenant_http", nil)
	server.ServeHTTP(remove, removeReq)
	if remove.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want %d, body = %s", remove.Code, http.StatusConflict, remove.Body.String())
	}
}

func TestRegisterDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"owner_id": "spoofed_user",
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
	if body.Document.OwnerID != "user_1" {
		t.Fatalf("owner id = %q, want authenticated user", body.Document.OwnerID)
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

func TestListDocumentsEndpoint(t *testing.T) {
	server := newTestServer(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"owner_id": "user_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body = %s", register.Code, http.StatusCreated, register.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Documents []documentPayload `json:"documents"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(body.Documents))
	}
	if body.Documents[0].Name != "Handbook.md" {
		t.Fatalf("document name = %q, want Handbook.md", body.Documents[0].Name)
	}
}

func TestListDocumentsEndpointRejectsInvalidTenant(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestGetDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body = %s", register.Code, http.StatusCreated, register.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents/doc_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Document documentPayload `json:"document"`
		Jobs     []jobPayload    `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Document.ID != "doc_http" {
		t.Fatalf("document id = %q, want doc_http", body.Document.ID)
	}
	if len(body.Jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(body.Jobs))
	}
	if body.Jobs[0].ResourceType != "document" || body.Jobs[0].ResourceID != "doc_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_http", body.Jobs[0].ResourceType, body.Jobs[0].ResourceID)
	}
}

func TestRetryDocumentEndpoint(t *testing.T) {
	server := newTestServerWithSeed(t, func(repos *memory.Store) {
		document := newHTTPFailedDocument(t)
		if err := repos.SaveDocument(context.Background(), document); err != nil {
			t.Fatalf("save document: %v", err)
		}
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/doc_failed_http/retry?tenant_id=tenant_1", nil)
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
	if body.Document.ID != "doc_failed_http" || body.Document.Status != string(domain.DocumentStatusFailed) {
		t.Fatalf("document = %s/%s, want failed doc_failed_http", body.Document.ID, body.Document.Status)
	}
	if body.Job.State != string(domain.JobStateQueued) {
		t.Fatalf("job state = %q, want queued", body.Job.State)
	}
	if body.Job.ResourceType != "document" || body.Job.ResourceID != "doc_failed_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_failed_http", body.Job.ResourceType, body.Job.ResourceID)
	}
}

func TestRetryDocumentEndpointRejectsUploadedDocument(t *testing.T) {
	server := newTestServer(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body = %s", register.Code, http.StatusCreated, register.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/doc_http/retry?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusConflict, resp.Body.String())
	}
}

func TestDeleteDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body = %s", register.Code, http.StatusCreated, register.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/documents/doc_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Document documentPayload `json:"document"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Document.Status != string(domain.DocumentStatusDeleted) {
		t.Fatalf("status = %q, want deleted", body.Document.Status)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/documents?tenant_id=tenant_1", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", list.Code, http.StatusOK, list.Body.String())
	}
	var listBody struct {
		Documents []documentPayload `json:"documents"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	if len(listBody.Documents) != 0 {
		t.Fatalf("documents len = %d, want 0", len(listBody.Documents))
	}
}

func TestDataSourceEndpoints(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"type": "synced_folder",
		"name": "OneDrive Support Docs",
		"root_path": "C:\\Users\\team\\OneDrive\\Support",
		"connector_config": {
			"provider": "sharepoint-sync",
			"resource_id": "site:team-support",
			"credential_ref": "secret/sharepoint/support",
			"notes": "customer-owned app"
		},
		"include_patterns": ["**/*.md", "**/*.pdf"],
		"exclude_patterns": ["archive/**"],
		"scan_interval_minutes": 60
	}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", create.Code, http.StatusCreated, create.Body.String())
	}
	var createBody struct {
		Source dataSourcePayload `json:"source"`
	}
	if err := json.NewDecoder(create.Body).Decode(&createBody); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createBody.Source.ID != "src_http" || createBody.Source.OwnerID != "user_1" {
		t.Fatalf("source = %#v", createBody.Source)
	}
	if createBody.Source.Status != "active" {
		t.Fatalf("status = %q, want active", createBody.Source.Status)
	}
	if len(createBody.Source.IncludePatterns) != 2 ||
		createBody.Source.IncludePatterns[0] != "**/*.md" ||
		len(createBody.Source.ExcludePatterns) != 1 ||
		createBody.Source.ExcludePatterns[0] != "archive/**" {
		t.Fatalf("create patterns = %#v/%#v", createBody.Source.IncludePatterns, createBody.Source.ExcludePatterns)
	}
	if createBody.Source.ScanIntervalMinutes != 60 || createBody.Source.NextScanAt == "" {
		t.Fatalf("create schedule = %d/%q", createBody.Source.ScanIntervalMinutes, createBody.Source.NextScanAt)
	}
	if createBody.Source.ConnectorConfig.Provider != "sharepoint-sync" ||
		createBody.Source.ConnectorConfig.CredentialRef != "secret/sharepoint/support" {
		t.Fatalf("create connector config = %#v", createBody.Source.ConnectorConfig)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources?tenant_id=tenant_1", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", list.Code, http.StatusOK, list.Body.String())
	}
	var listBody struct {
		Sources []dataSourcePayload `json:"sources"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Sources) != 1 || listBody.Sources[0].Name != "OneDrive Support Docs" {
		t.Fatalf("sources = %#v", listBody.Sources)
	}

	update := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/v1/data-sources/src_http", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"type": "network_share",
		"name": "NAS Runbooks",
		"root_path": "\\\\nas\\runbooks",
		"connector_config": {
			"provider": "smb",
			"resource_id": "\\\\nas\\runbooks",
			"credential_ref": "secret/nas/read-only",
			"notes": "mounted by host"
		},
		"include_patterns": ["runbooks/**"],
		"exclude_patterns": ["drafts/**", "*.tmp"],
		"scan_interval_minutes": 1440
	}`))
	server.ServeHTTP(update, updateReq)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body = %s", update.Code, http.StatusOK, update.Body.String())
	}
	var updateBody struct {
		Source dataSourcePayload `json:"source"`
	}
	if err := json.NewDecoder(update.Body).Decode(&updateBody); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updateBody.Source.Type != "network_share" || updateBody.Source.Name != "NAS Runbooks" {
		t.Fatalf("updated source = %#v", updateBody.Source)
	}
	if len(updateBody.Source.IncludePatterns) != 1 ||
		updateBody.Source.IncludePatterns[0] != "runbooks/**" ||
		len(updateBody.Source.ExcludePatterns) != 2 ||
		updateBody.Source.ExcludePatterns[1] != "*.tmp" {
		t.Fatalf("updated patterns = %#v/%#v", updateBody.Source.IncludePatterns, updateBody.Source.ExcludePatterns)
	}
	if updateBody.Source.ScanIntervalMinutes != 1440 || updateBody.Source.NextScanAt == "" {
		t.Fatalf("updated schedule = %d/%q", updateBody.Source.ScanIntervalMinutes, updateBody.Source.NextScanAt)
	}
	if updateBody.Source.ConnectorConfig.Provider != "smb" ||
		updateBody.Source.ConnectorConfig.ResourceID != "\\\\nas\\runbooks" ||
		updateBody.Source.ConnectorConfig.CredentialRef != "secret/nas/read-only" {
		t.Fatalf("updated connector config = %#v", updateBody.Source.ConnectorConfig)
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d, body = %s", get.Code, http.StatusOK, get.Body.String())
	}
	var getBody struct {
		Source      dataSourcePayload            `json:"source"`
		Jobs        []jobPayload                 `json:"jobs"`
		ScanEntries []dataSourceScanEntryPayload `json:"scan_entries"`
	}
	if err := json.NewDecoder(get.Body).Decode(&getBody); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if getBody.Source.ID != "src_http" || len(getBody.Jobs) != 0 || len(getBody.ScanEntries) != 0 {
		t.Fatalf("get body = %#v, want source with no jobs", getBody)
	}
	if len(getBody.Source.IncludePatterns) != 1 || getBody.Source.IncludePatterns[0] != "runbooks/**" {
		t.Fatalf("get patterns = %#v", getBody.Source.IncludePatterns)
	}
	if getBody.Source.ScanIntervalMinutes != 1440 || getBody.Source.NextScanAt == "" {
		t.Fatalf("get schedule = %d/%q", getBody.Source.ScanIntervalMinutes, getBody.Source.NextScanAt)
	}
	if getBody.Source.ConnectorConfig.Provider != "smb" ||
		getBody.Source.ConnectorConfig.Notes != "mounted by host" {
		t.Fatalf("get connector config = %#v", getBody.Source.ConnectorConfig)
	}

	scan := httptest.NewRecorder()
	scanReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_http/scan?tenant_id=tenant_1", nil)
	server.ServeHTTP(scan, scanReq)
	if scan.Code != http.StatusAccepted {
		t.Fatalf("scan status = %d, want %d, body = %s", scan.Code, http.StatusAccepted, scan.Body.String())
	}
	var scanBody struct {
		Source dataSourcePayload `json:"source"`
		Job    jobPayload        `json:"job"`
	}
	if err := json.NewDecoder(scan.Body).Decode(&scanBody); err != nil {
		t.Fatalf("decode scan: %v", err)
	}
	if scanBody.Source.ID != "src_http" ||
		scanBody.Job.Type != "source_scan" ||
		scanBody.Job.ResourceType != "data_source" ||
		scanBody.Job.ResourceID != "src_http" ||
		scanBody.Job.State != "queued" {
		t.Fatalf("scan body = %#v", scanBody)
	}

	cancelScan := httptest.NewRecorder()
	cancelScanReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_http/scan/cancel?tenant_id=tenant_1", nil)
	server.ServeHTTP(cancelScan, cancelScanReq)
	if cancelScan.Code != http.StatusOK {
		t.Fatalf("cancel scan status = %d, want %d, body = %s", cancelScan.Code, http.StatusOK, cancelScan.Body.String())
	}
	var cancelScanBody struct {
		Source dataSourcePayload `json:"source"`
		Job    jobPayload        `json:"job"`
	}
	if err := json.NewDecoder(cancelScan.Body).Decode(&cancelScanBody); err != nil {
		t.Fatalf("decode cancel scan: %v", err)
	}
	if cancelScanBody.Source.ID != "src_http" ||
		cancelScanBody.Job.Type != "source_scan" ||
		cancelScanBody.Job.State != "canceled" {
		t.Fatalf("cancel scan body = %#v", cancelScanBody)
	}

	getAfterScan := httptest.NewRecorder()
	getAfterScanReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(getAfterScan, getAfterScanReq)
	if getAfterScan.Code != http.StatusOK {
		t.Fatalf("get after scan status = %d, want %d, body = %s", getAfterScan.Code, http.StatusOK, getAfterScan.Body.String())
	}
	var getAfterScanBody struct {
		Source      dataSourcePayload            `json:"source"`
		Jobs        []jobPayload                 `json:"jobs"`
		ScanEntries []dataSourceScanEntryPayload `json:"scan_entries"`
	}
	if err := json.NewDecoder(getAfterScan.Body).Decode(&getAfterScanBody); err != nil {
		t.Fatalf("decode get after scan: %v", err)
	}
	if len(getAfterScanBody.Jobs) != 1 || getAfterScanBody.Jobs[0].Type != "source_scan" {
		t.Fatalf("detail jobs = %#v, want source_scan job", getAfterScanBody.Jobs)
	}
	if len(getAfterScanBody.ScanEntries) != 0 {
		t.Fatalf("scan entries = %#v, want none before worker runs", getAfterScanBody.ScanEntries)
	}

	remove := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodDelete, "/v1/data-sources/src_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(remove, removeReq)
	if remove.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want %d, body = %s", remove.Code, http.StatusOK, remove.Body.String())
	}
	var removeBody struct {
		Source dataSourcePayload `json:"source"`
	}
	if err := json.NewDecoder(remove.Body).Decode(&removeBody); err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	if removeBody.Source.Status != "archived" {
		t.Fatalf("status = %q, want archived", removeBody.Source.Status)
	}

	listActive := httptest.NewRecorder()
	listActiveReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources?tenant_id=tenant_1", nil)
	server.ServeHTTP(listActive, listActiveReq)
	var listActiveBody struct {
		Sources []dataSourcePayload `json:"sources"`
	}
	if err := json.NewDecoder(listActive.Body).Decode(&listActiveBody); err != nil {
		t.Fatalf("decode list active: %v", err)
	}
	if len(listActiveBody.Sources) != 0 {
		t.Fatalf("active sources len = %d, want 0", len(listActiveBody.Sources))
	}

	audit := httptest.NewRecorder()
	auditReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1", nil)
	server.ServeHTTP(audit, auditReq)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d, body = %s", audit.Code, http.StatusOK, audit.Body.String())
	}
	var auditBody struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.NewDecoder(audit.Body).Decode(&auditBody); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if len(auditBody.Events) != 5 {
		t.Fatalf("audit events len = %d, want 5", len(auditBody.Events))
	}
	hasArchivedAudit := false
	hasScanAudit := false
	hasScanCanceledAudit := false
	for _, event := range auditBody.Events {
		if event.Action == "data_source.archived" && event.ResourceID == "src_http" {
			hasArchivedAudit = true
		}
		if event.Action == "data_source.scan_requested" && event.ResourceID == "src_http" {
			hasScanAudit = true
		}
		if event.Action == "data_source.scan_canceled" && event.ResourceID == "src_http" {
			hasScanCanceledAudit = true
		}
	}
	if !hasArchivedAudit {
		t.Fatalf("audit events = %#v, want archived source event", auditBody.Events)
	}
	if !hasScanAudit {
		t.Fatalf("audit events = %#v, want source scan event", auditBody.Events)
	}
	if !hasScanCanceledAudit {
		t.Fatalf("audit events = %#v, want source scan canceled event", auditBody.Events)
	}
}

func TestDataSourcePreflightEndpointQueuesJob(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"type": "folder",
		"name": "Support Docs",
		"root_path": "/sources/support",
		"include_patterns": ["**/*.md"],
		"exclude_patterns": [],
		"scan_interval_minutes": 0
	}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", create.Code, http.StatusCreated, create.Body.String())
	}

	preflight := httptest.NewRecorder()
	preflightReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_http/preflight?tenant_id=tenant_1", nil)
	server.ServeHTTP(preflight, preflightReq)
	if preflight.Code != http.StatusAccepted {
		t.Fatalf("preflight status = %d, want %d, body = %s", preflight.Code, http.StatusAccepted, preflight.Body.String())
	}
	var preflightBody struct {
		Source dataSourcePayload `json:"source"`
		Job    jobPayload        `json:"job"`
	}
	if err := json.NewDecoder(preflight.Body).Decode(&preflightBody); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}
	if preflightBody.Source.ID != "src_http" ||
		preflightBody.Job.Type != "source_preflight" ||
		preflightBody.Job.ResourceType != "data_source" ||
		preflightBody.Job.ResourceID != "src_http" ||
		preflightBody.Job.State != "queued" {
		t.Fatalf("preflight body = %#v", preflightBody)
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(detail, detailReq)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d, body = %s", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailBody struct {
		Jobs []jobPayload `json:"jobs"`
	}
	if err := json.NewDecoder(detail.Body).Decode(&detailBody); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detailBody.Jobs) != 1 || detailBody.Jobs[0].Type != "source_preflight" {
		t.Fatalf("detail jobs = %#v, want source_preflight job", detailBody.Jobs)
	}
}

func TestDataSourcePlanEndpointQueuesJob(t *testing.T) {
	server := newTestServer(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"type": "folder",
		"name": "Support Docs",
		"root_path": "/sources/support",
		"include_patterns": ["**/*.md"],
		"exclude_patterns": [],
		"scan_interval_minutes": 0
	}`))
	server.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", create.Code, http.StatusCreated, create.Body.String())
	}

	plan := httptest.NewRecorder()
	planReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_http/plan?tenant_id=tenant_1", nil)
	server.ServeHTTP(plan, planReq)
	if plan.Code != http.StatusAccepted {
		t.Fatalf("plan status = %d, want %d, body = %s", plan.Code, http.StatusAccepted, plan.Body.String())
	}
	var planBody struct {
		Source dataSourcePayload `json:"source"`
		Job    jobPayload        `json:"job"`
	}
	if err := json.NewDecoder(plan.Body).Decode(&planBody); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if planBody.Source.ID != "src_http" ||
		planBody.Job.Type != "source_plan" ||
		planBody.Job.ResourceType != "data_source" ||
		planBody.Job.ResourceID != "src_http" ||
		planBody.Job.State != "queued" {
		t.Fatalf("plan body = %#v", planBody)
	}
}

func TestGetDataSourceReturnsScanSummary(t *testing.T) {
	server := newTestServerWithSeed(t, func(repos *memory.Store) {
		source, err := domain.NewDataSource(domain.DataSourceCreate{
			ID:       domain.DataSourceID("src_summary"),
			TenantID: domain.TenantID("tenant_1"),
			OwnerID:  domain.UserID("user_1"),
			Type:     domain.DataSourceTypeFolder,
			Name:     "Summary Source",
			RootPath: "/sources/summary",
			Now:      httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new source: %v", err)
		}
		if err := repos.SaveDataSource(context.Background(), source); err != nil {
			t.Fatalf("save source: %v", err)
		}
		for _, entry := range []domain.DataSourceScanEntry{
			newHTTPScanEntry(t, source, domain.JobID("job_old"), "old.md", domain.DataSourceScanOutcomeImported, "", httpClock{}.Now().Add(-time.Hour)),
			newHTTPScanEntry(t, source, domain.JobID("job_latest"), "ok.md", domain.DataSourceScanOutcomeImported, "", httpClock{}.Now()),
			newHTTPScanEntry(t, source, domain.JobID("job_latest"), "skipped.tmp", domain.DataSourceScanOutcomeSkipped, "unsupported_type", httpClock{}.Now()),
			newHTTPScanEntry(t, source, domain.JobID("job_latest"), "broken.pdf", domain.DataSourceScanOutcomeFailed, "parse_failed", httpClock{}.Now()),
		} {
			if err := repos.SaveDataSourceScanEntry(context.Background(), entry); err != nil {
				t.Fatalf("save scan entry: %v", err)
			}
		}
	})

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_summary?tenant_id=tenant_1", nil)
	server.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d, body = %s", get.Code, http.StatusOK, get.Body.String())
	}
	var body struct {
		Source      dataSourcePayload              `json:"source"`
		ScanEntries []dataSourceScanEntryPayload   `json:"scan_entries"`
		ScanSummary dataSourceScanSummaryPayload   `json:"scan_summary"`
		ScanPage    dataSourceScanEntryPagePayload `json:"scan_entries_page"`
	}
	if err := json.NewDecoder(get.Body).Decode(&body); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if body.Source.ID != "src_summary" || len(body.ScanEntries) != 4 {
		t.Fatalf("body = %#v, want source with four recent entries", body)
	}
	if body.ScanSummary.LatestJobID != "job_latest" ||
		body.ScanSummary.Total != 3 ||
		body.ScanSummary.Imported != 1 ||
		body.ScanSummary.Skipped != 1 ||
		body.ScanSummary.Failed != 1 ||
		body.ScanSummary.Deleted != 0 ||
		body.ScanSummary.LatestAt == "" {
		t.Fatalf("scan summary = %#v, want latest scan counts", body.ScanSummary)
	}
	if body.ScanSummary.Reasons["unsupported_type"] != 1 ||
		body.ScanSummary.Reasons["parse_failed"] != 1 {
		t.Fatalf("scan summary reasons = %#v", body.ScanSummary.Reasons)
	}
	if body.ScanPage.Total != 4 ||
		body.ScanPage.Limit != 100 ||
		body.ScanPage.Offset != 0 ||
		body.ScanPage.HasMore {
		t.Fatalf("scan page = %#v, want first unfiltered page", body.ScanPage)
	}

	filtered := httptest.NewRecorder()
	filteredReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_summary?tenant_id=tenant_1&scan_entry_outcome=failed&scan_entry_limit=1&scan_entry_offset=0", nil)
	server.ServeHTTP(filtered, filteredReq)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered status = %d, want %d, body = %s", filtered.Code, http.StatusOK, filtered.Body.String())
	}
	var filteredBody struct {
		ScanEntries []dataSourceScanEntryPayload   `json:"scan_entries"`
		ScanPage    dataSourceScanEntryPagePayload `json:"scan_entries_page"`
	}
	if err := json.NewDecoder(filtered.Body).Decode(&filteredBody); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if filteredBody.ScanPage.Total != 1 ||
		filteredBody.ScanPage.Limit != 1 ||
		filteredBody.ScanPage.Offset != 0 ||
		filteredBody.ScanPage.Outcome != "failed" ||
		filteredBody.ScanPage.HasMore {
		t.Fatalf("filtered page = %#v, want one failed entry page", filteredBody.ScanPage)
	}
	if len(filteredBody.ScanEntries) != 1 || filteredBody.ScanEntries[0].Path != "broken.pdf" {
		t.Fatalf("filtered entries = %#v, want broken.pdf", filteredBody.ScanEntries)
	}

	exported := httptest.NewRecorder()
	exportReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_summary/scan-entries.csv?tenant_id=tenant_1&outcome=failed", nil)
	server.ServeHTTP(exported, exportReq)
	if exported.Code != http.StatusOK {
		t.Fatalf("export status = %d, want %d, body = %s", exported.Code, http.StatusOK, exported.Body.String())
	}
	if contentType := exported.Header().Get("Content-Type"); !strings.Contains(contentType, "text/csv") {
		t.Fatalf("export content type = %q, want csv", contentType)
	}
	if exported.Header().Get("X-Nexus-Scan-Entry-Total") != "1" ||
		exported.Header().Get("X-Nexus-Scan-Entry-Truncated") != "false" {
		t.Fatalf("export headers = %#v", exported.Header())
	}
	if !strings.Contains(exported.Body.String(), "path,outcome") ||
		!strings.Contains(exported.Body.String(), "broken.pdf,failed") {
		t.Fatalf("export body = %q, want failed row", exported.Body.String())
	}
}

func TestDataSourceEndpointCanArchiveAndDeleteDocuments(t *testing.T) {
	server := newTestServerWithSeed(t, func(repos *memory.Store) {
		source, err := domain.NewDataSource(domain.DataSourceCreate{
			ID:       domain.DataSourceID("src_cleanup"),
			TenantID: domain.TenantID("tenant_1"),
			OwnerID:  domain.UserID("user_1"),
			Type:     domain.DataSourceTypeFolder,
			Name:     "Cleanup Source",
			RootPath: "/sources/cleanup",
			Now:      httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new source: %v", err)
		}
		if err := repos.SaveDataSource(context.Background(), source); err != nil {
			t.Fatalf("save source: %v", err)
		}
		document, err := domain.NewDocument(domain.DocumentCreate{
			ID:         domain.DocumentID("doc_cleanup"),
			TenantID:   source.TenantID,
			OwnerID:    source.OwnerID,
			Name:       "cleanup.md",
			StorageKey: "tenants/tenant_1/documents/doc_cleanup/cleanup.md",
			SizeBytes:  42,
			Now:        httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new document: %v", err)
		}
		if err := repos.SaveDocument(context.Background(), document); err != nil {
			t.Fatalf("save document: %v", err)
		}
		entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
			TenantID:    source.TenantID,
			JobID:       domain.JobID("job_scan_cleanup"),
			SourceID:    source.ID,
			Path:        "cleanup.md",
			Outcome:     domain.DataSourceScanOutcomeImported,
			DocumentID:  document.ID,
			SizeBytes:   document.SizeBytes,
			ContentHash: "sha256:cleanup",
			Now:         httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new scan entry: %v", err)
		}
		if err := repos.SaveDataSourceScanEntry(context.Background(), entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	})

	remove := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodDelete, "/v1/data-sources/src_cleanup?tenant_id=tenant_1&delete_documents=true", nil)
	server.ServeHTTP(remove, removeReq)
	if remove.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want %d, body = %s", remove.Code, http.StatusOK, remove.Body.String())
	}
	var removeBody struct {
		Source           dataSourcePayload `json:"source"`
		DeletedDocuments int               `json:"deleted_documents"`
	}
	if err := json.NewDecoder(remove.Body).Decode(&removeBody); err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	if removeBody.Source.Status != "archived" || removeBody.DeletedDocuments != 1 {
		t.Fatalf("remove body = %#v, want archived with one deleted document", removeBody)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/documents?tenant_id=tenant_1", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list documents status = %d, want %d, body = %s", list.Code, http.StatusOK, list.Body.String())
	}
	var listBody struct {
		Documents []documentPayload `json:"documents"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list documents: %v", err)
	}
	if len(listBody.Documents) != 0 {
		t.Fatalf("documents = %#v, want deleted source document hidden", listBody.Documents)
	}

	audit := httptest.NewRecorder()
	auditReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1", nil)
	server.ServeHTTP(audit, auditReq)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d, body = %s", audit.Code, http.StatusOK, audit.Body.String())
	}
	var auditBody struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.NewDecoder(audit.Body).Decode(&auditBody); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	hasDeleteAudit := false
	for _, event := range auditBody.Events {
		if event.Action == "data_source.documents_deleted" &&
			event.ResourceID == "src_cleanup" &&
			event.Metadata["deleted_documents"] == "1" {
			hasDeleteAudit = true
		}
	}
	if !hasDeleteAudit {
		t.Fatalf("audit events = %#v, want documents_deleted event", auditBody.Events)
	}
}

func TestDataSourceEndpointCanReindexDocuments(t *testing.T) {
	server := newTestServerWithSeed(t, func(repos *memory.Store) {
		source, err := domain.NewDataSource(domain.DataSourceCreate{
			ID:       domain.DataSourceID("src_reindex"),
			TenantID: domain.TenantID("tenant_1"),
			OwnerID:  domain.UserID("user_1"),
			Type:     domain.DataSourceTypeFolder,
			Name:     "Reindex Source",
			RootPath: "/sources/reindex",
			Now:      httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new source: %v", err)
		}
		if err := repos.SaveDataSource(context.Background(), source); err != nil {
			t.Fatalf("save source: %v", err)
		}
		document, err := domain.NewDocument(domain.DocumentCreate{
			ID:         domain.DocumentID("doc_reindex"),
			TenantID:   source.TenantID,
			OwnerID:    source.OwnerID,
			Name:       "reindex.md",
			StorageKey: "tenants/tenant_1/documents/doc_reindex/reindex.md",
			SizeBytes:  42,
			Now:        httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new document: %v", err)
		}
		if err := repos.SaveDocument(context.Background(), document); err != nil {
			t.Fatalf("save document: %v", err)
		}
		entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
			TenantID:    source.TenantID,
			JobID:       domain.JobID("job_scan_reindex"),
			SourceID:    source.ID,
			Path:        "reindex.md",
			Outcome:     domain.DataSourceScanOutcomeImported,
			DocumentID:  document.ID,
			SizeBytes:   document.SizeBytes,
			ContentHash: "sha256:reindex",
			Now:         httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new scan entry: %v", err)
		}
		if err := repos.SaveDataSourceScanEntry(context.Background(), entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	})

	reindex := httptest.NewRecorder()
	reindexReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_reindex/reindex?tenant_id=tenant_1", nil)
	server.ServeHTTP(reindex, reindexReq)
	if reindex.Code != http.StatusAccepted {
		t.Fatalf("reindex status = %d, want %d, body = %s", reindex.Code, http.StatusAccepted, reindex.Body.String())
	}
	var reindexBody struct {
		Source           dataSourcePayload `json:"source"`
		Jobs             []jobPayload      `json:"jobs"`
		QueuedDocuments  int               `json:"queued_documents"`
		SkippedDocuments int               `json:"skipped_documents"`
	}
	if err := json.NewDecoder(reindex.Body).Decode(&reindexBody); err != nil {
		t.Fatalf("decode reindex: %v", err)
	}
	if reindexBody.Source.ID != "src_reindex" ||
		reindexBody.QueuedDocuments != 1 ||
		reindexBody.SkippedDocuments != 0 ||
		len(reindexBody.Jobs) != 1 ||
		reindexBody.Jobs[0].Type != "document_ingestion" ||
		reindexBody.Jobs[0].ResourceID != "doc_reindex" {
		t.Fatalf("reindex body = %#v", reindexBody)
	}

	audit := httptest.NewRecorder()
	auditReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1", nil)
	server.ServeHTTP(audit, auditReq)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d, body = %s", audit.Code, http.StatusOK, audit.Body.String())
	}
	var auditBody struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.NewDecoder(audit.Body).Decode(&auditBody); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	hasReindexAudit := false
	for _, event := range auditBody.Events {
		if event.Action == "data_source.reindex_requested" &&
			event.ResourceID == "src_reindex" &&
			event.Metadata["queued_documents"] == "1" {
			hasReindexAudit = true
		}
	}
	if !hasReindexAudit {
		t.Fatalf("audit events = %#v, want reindex_requested event", auditBody.Events)
	}
}

func TestDataSourceEndpointCanRetryFailedDocuments(t *testing.T) {
	server := newTestServerWithSeed(t, func(repos *memory.Store) {
		source, err := domain.NewDataSource(domain.DataSourceCreate{
			ID:       domain.DataSourceID("src_retry_failed"),
			TenantID: domain.TenantID("tenant_1"),
			OwnerID:  domain.UserID("user_1"),
			Type:     domain.DataSourceTypeFolder,
			Name:     "Retry Failed Source",
			RootPath: "/sources/retry-failed",
			Now:      httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new source: %v", err)
		}
		if err := repos.SaveDataSource(context.Background(), source); err != nil {
			t.Fatalf("save source: %v", err)
		}
		document, err := domain.NewDocument(domain.DocumentCreate{
			ID:         domain.DocumentID("doc_retry_failed"),
			TenantID:   source.TenantID,
			OwnerID:    source.OwnerID,
			Name:       "failed.md",
			StorageKey: "tenants/tenant_1/documents/doc_retry_failed/failed.md",
			SizeBytes:  42,
			Now:        httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new document: %v", err)
		}
		if err := document.Transition(domain.DocumentStatusFailed, httpClock{}.Now()); err != nil {
			t.Fatalf("mark failed document: %v", err)
		}
		if err := repos.SaveDocument(context.Background(), document); err != nil {
			t.Fatalf("save document: %v", err)
		}
		entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
			TenantID:    source.TenantID,
			JobID:       domain.JobID("job_scan_retry_failed"),
			SourceID:    source.ID,
			Path:        "failed.md",
			Outcome:     domain.DataSourceScanOutcomeImported,
			DocumentID:  document.ID,
			SizeBytes:   document.SizeBytes,
			ContentHash: "sha256:retry-failed",
			Now:         httpClock{}.Now(),
		})
		if err != nil {
			t.Fatalf("new scan entry: %v", err)
		}
		if err := repos.SaveDataSourceScanEntry(context.Background(), entry); err != nil {
			t.Fatalf("save scan entry: %v", err)
		}
	})

	retry := httptest.NewRecorder()
	retryReq := httptest.NewRequest(http.MethodPost, "/v1/data-sources/src_retry_failed/retry-failed-documents?tenant_id=tenant_1", nil)
	server.ServeHTTP(retry, retryReq)
	if retry.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, want %d, body = %s", retry.Code, http.StatusAccepted, retry.Body.String())
	}
	var retryBody struct {
		Source           dataSourcePayload `json:"source"`
		Jobs             []jobPayload      `json:"jobs"`
		QueuedDocuments  int               `json:"queued_documents"`
		SkippedDocuments int               `json:"skipped_documents"`
	}
	if err := json.NewDecoder(retry.Body).Decode(&retryBody); err != nil {
		t.Fatalf("decode retry: %v", err)
	}
	if retryBody.Source.ID != "src_retry_failed" ||
		retryBody.QueuedDocuments != 1 ||
		retryBody.SkippedDocuments != 0 ||
		len(retryBody.Jobs) != 1 ||
		retryBody.Jobs[0].Type != "document_ingestion" ||
		retryBody.Jobs[0].ResourceID != "doc_retry_failed" {
		t.Fatalf("retry body = %#v", retryBody)
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/v1/data-sources/src_retry_failed?tenant_id=tenant_1", nil)
	server.ServeHTTP(detail, detailReq)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d, body = %s", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailBody struct {
		FailedDocuments int `json:"failed_documents"`
	}
	if err := json.NewDecoder(detail.Body).Decode(&detailBody); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detailBody.FailedDocuments != 1 {
		t.Fatalf("failed documents = %d, want 1", detailBody.FailedDocuments)
	}

	audit := httptest.NewRecorder()
	auditReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1", nil)
	server.ServeHTTP(audit, auditReq)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d, body = %s", audit.Code, http.StatusOK, audit.Body.String())
	}
	var auditBody struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.NewDecoder(audit.Body).Decode(&auditBody); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	hasRetryAudit := false
	for _, event := range auditBody.Events {
		if event.Action == "data_source.failed_documents_retry_requested" &&
			event.ResourceID == "src_retry_failed" &&
			event.Metadata["queued_documents"] == "1" {
			hasRetryAudit = true
		}
	}
	if !hasRetryAudit {
		t.Fatalf("audit events = %#v, want failed_documents_retry_requested event", auditBody.Events)
	}
}

func TestSourceViewEndpointRoundTrip(t *testing.T) {
	server := newTestServer(t)

	createResp := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/source-views", strings.NewReader(`{
		"tenant_id": "tenant_1",
		"name": "Blocked",
		"filters": {
			"health": "blocked",
			"query": "oidc",
			"schedule": "",
			"type": "synced_folder"
		}
	}`))
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}
	var createBody struct {
		View sourceViewPayload `json:"view"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createBody.View.ID != "view_http" || createBody.View.Filters.Health != "blocked" {
		t.Fatalf("created view = %#v", createBody.View)
	}

	listResp := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/source-views?tenant_id=tenant_1", nil)
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", listResp.Code, http.StatusOK, listResp.Body.String())
	}
	var listBody struct {
		Views []sourceViewPayload `json:"views"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Views) != 1 || listBody.Views[0].Name != "Blocked" {
		t.Fatalf("listed views = %#v", listBody.Views)
	}

	updateResp := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/v1/source-views/view_http", strings.NewReader(`{
		"tenant_id": "tenant_1",
		"name": "Needs Review",
		"filters": {
			"health": "review",
			"query": "",
			"schedule": "overdue",
			"type": ""
		}
	}`))
	server.ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body = %s", updateResp.Code, http.StatusOK, updateResp.Body.String())
	}
	var updateBody struct {
		View sourceViewPayload `json:"view"`
	}
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updateBody); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updateBody.View.Name != "Needs Review" || updateBody.View.Filters.Schedule != "overdue" {
		t.Fatalf("updated view = %#v", updateBody.View)
	}

	deleteResp := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/source-views/view_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body = %s", deleteResp.Code, http.StatusNoContent, deleteResp.Body.String())
	}
}

func TestSourcePolicyProfileEndpointRoundTrip(t *testing.T) {
	server := newTestServer(t)

	createResp := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/source-policy-profiles", strings.NewReader(`{
		"tenant_id": "tenant_1",
		"name": "Support exports",
		"detail": "Cases and logs",
		"include_patterns": ["**/*.json", "**/*.csv"],
		"exclude_patterns": ["**/tmp/**"],
		"scan_interval_minutes": 1440
	}`))
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}
	var createBody struct {
		Profile sourcePolicyProfilePayload `json:"profile"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createBody.Profile.ID != "policy_http" || createBody.Profile.IncludePatterns[0] != "**/*.json" {
		t.Fatalf("created profile = %#v", createBody.Profile)
	}

	listResp := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/source-policy-profiles?tenant_id=tenant_1", nil)
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", listResp.Code, http.StatusOK, listResp.Body.String())
	}
	var listBody struct {
		Profiles []sourcePolicyProfilePayload `json:"profiles"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Profiles) != 1 || listBody.Profiles[0].Name != "Support exports" {
		t.Fatalf("listed profiles = %#v", listBody.Profiles)
	}

	updateResp := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/v1/source-policy-profiles/policy_http", strings.NewReader(`{
		"tenant_id": "tenant_1",
		"name": "Support logs",
		"detail": "Refined",
		"include_patterns": ["**/*.txt"],
		"exclude_patterns": [],
		"scan_interval_minutes": 0
	}`))
	server.ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body = %s", updateResp.Code, http.StatusOK, updateResp.Body.String())
	}
	var updateBody struct {
		Profile sourcePolicyProfilePayload `json:"profile"`
	}
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updateBody); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updateBody.Profile.Name != "Support logs" || updateBody.Profile.IncludePatterns[0] != "**/*.txt" {
		t.Fatalf("updated profile = %#v", updateBody.Profile)
	}

	deleteResp := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/source-policy-profiles/policy_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body = %s", deleteResp.Code, http.StatusNoContent, deleteResp.Body.String())
	}
}

func TestCreateDataSourceEndpointRejectsInvalidInput(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/data-sources", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"type": "folder",
		"name": "",
		"root_path": ""
	}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func TestListJobsEndpoint(t *testing.T) {
	server := newTestServer(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/documents/register", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"owner_id": "user_1",
		"name": "Handbook.md",
		"storage_key": "tenants/tenant_1/documents/source.md",
		"size_bytes": 42
	}`))
	server.ServeHTTP(register, registerReq)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body = %s", register.Code, http.StatusCreated, register.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?tenant_id=tenant_1&limit=5", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Jobs []jobPayload `json:"jobs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(body.Jobs))
	}
	if body.Jobs[0].ResourceType != "document" || body.Jobs[0].ResourceID != "doc_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_http", body.Jobs[0].ResourceType, body.Jobs[0].ResourceID)
	}
}

func TestListJobsEndpointRejectsInvalidLimit(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?tenant_id=tenant_1&limit=nope", nil)
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
	if err := writer.WriteField("owner_id", "spoofed_user"); err != nil {
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
	if response.Document.OwnerID != "user_1" {
		t.Fatalf("owner id = %q, want authenticated user", response.Document.OwnerID)
	}
	if response.Job.State != string(domain.JobStateQueued) {
		t.Fatalf("job state = %q, want queued", response.Job.State)
	}
	if response.Job.ResourceType != "document" || response.Job.ResourceID != "doc_http" {
		t.Fatalf("job resource = %s/%s, want document/doc_http", response.Job.ResourceType, response.Job.ResourceID)
	}
}

func TestUploadDocumentEndpointPreservesProvidedName(t *testing.T) {
	server := newTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tenant_id", "tenant_1"); err != nil {
		t.Fatalf("tenant field: %v", err)
	}
	if err := writer.WriteField("name", "Engineering/Runbooks/Handbook.md"); err != nil {
		t.Fatalf("name field: %v", err)
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
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if response.Document.Name != "Engineering/Runbooks/Handbook.md" {
		t.Fatalf("document name = %q", response.Document.Name)
	}
	if response.Document.StorageKey != "tenants/tenant_1/documents/doc_http/Handbook.md" {
		t.Fatalf("storage key = %q", response.Document.StorageKey)
	}
}

func TestUploadDocumentEndpointRejectsUnsupportedType(t *testing.T) {
	server := newTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tenant_id", "tenant_1"); err != nil {
		t.Fatalf("tenant field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "photo.png")
	if err != nil {
		t.Fatalf("file field: %v", err)
	}
	if _, err := part.Write([]byte("not really a png")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusUnsupportedMediaType, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "supported files") {
		t.Fatalf("body = %s, want supported files message", resp.Body.String())
	}
}

func TestDownloadDocumentEndpoint(t *testing.T) {
	server := newTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tenant_id", "tenant_1"); err != nil {
		t.Fatalf("tenant field: %v", err)
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

	upload := httptest.NewRecorder()
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/documents/upload", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	server.ServeHTTP(upload, uploadReq)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d, body = %s", upload.Code, http.StatusCreated, upload.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents/doc_http/download?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if resp.Body.String() != "hello world" {
		t.Fatalf("body = %q, want hello world", resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content type = %q, want application/octet-stream", got)
	}
	if got := resp.Header().Get("Content-Length"); got != "11" {
		t.Fatalf("content length = %q, want 11", got)
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.Contains(got, "Handbook.md") {
		t.Fatalf("content disposition = %q, want filename", got)
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
	if body.Hits[0].Source.DocumentName != "Alpha Plan.md" {
		t.Fatalf("source name = %q, want Alpha Plan.md", body.Hits[0].Source.DocumentName)
	}
}

func TestSearchEndpointCanScopeToDocument(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"document_id": "doc_notes",
		"query": "alpha beta",
		"limit": 5
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
	if body.Hits[0].DocumentID != "doc_notes" {
		t.Fatalf("document id = %q, want doc_notes", body.Hits[0].DocumentID)
	}
	if body.Hits[0].Source.DocumentName != "Omega Notes.md" {
		t.Fatalf("source name = %q, want Omega Notes.md", body.Hits[0].Source.DocumentName)
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
	if body.Hits[0].Source.DocumentName != "Alpha Plan.md" {
		t.Fatalf("source name = %q, want Alpha Plan.md", body.Hits[0].Source.DocumentName)
	}
}

func TestAskConversationEndpointCanScopeToDocument(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"document_id": "doc_notes",
		"question": "What is the alpha beta plan?",
		"limit": 5
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
	if body.Hits[0].DocumentID != "doc_notes" {
		t.Fatalf("document id = %q, want doc_notes", body.Hits[0].DocumentID)
	}
}

func TestAuditEventsEndpointListsUserActions(t *testing.T) {
	server := newTestServer(t)

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	if err := writer.WriteField("tenant_id", "tenant_1"); err != nil {
		t.Fatalf("tenant field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "Audit.md")
	if err != nil {
		t.Fatalf("file field: %v", err)
	}
	if _, err := part.Write([]byte("audit event fixture")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	upload := httptest.NewRecorder()
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/documents/upload", &uploadBody)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	server.ServeHTTP(upload, uploadReq)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d, body = %s", upload.Code, http.StatusCreated, upload.Body.String())
	}

	search := httptest.NewRecorder()
	searchReq := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"query": "alpha beta",
		"limit": 1
	}`))
	server.ServeHTTP(search, searchReq)
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d, body = %s", search.Code, http.StatusOK, search.Body.String())
	}

	ask := httptest.NewRecorder()
	askReq := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"question": "What is the alpha beta plan?",
		"limit": 1
	}`))
	server.ServeHTTP(ask, askReq)
	if ask.Code != http.StatusOK {
		t.Fatalf("ask status = %d, want %d, body = %s", ask.Code, http.StatusOK, ask.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1&limit=10", nil)
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Events) != 3 {
		t.Fatalf("events len = %d, want 3: %#v", len(body.Events), body.Events)
	}
	actions := map[string]auditEventPayload{}
	for _, event := range body.Events {
		actions[event.Action] = event
		if event.ActorUserID != "user_1" {
			t.Fatalf("actor user id = %q, want user_1", event.ActorUserID)
		}
		if event.Outcome != string(domain.AuditOutcomeSucceeded) {
			t.Fatalf("event outcome = %q, want succeeded", event.Outcome)
		}
	}
	for _, action := range []string{"document.uploaded", "search.completed", "conversation.ask"} {
		if _, ok := actions[action]; !ok {
			t.Fatalf("missing audit action %q in %#v", action, actions)
		}
	}
	if actions["document.uploaded"].ResourceID != "doc_http" {
		t.Fatalf("document resource id = %q", actions["document.uploaded"].ResourceID)
	}
	if actions["search.completed"].Metadata["hit_count"] != "1" {
		t.Fatalf("search metadata = %#v", actions["search.completed"].Metadata)
	}
	if actions["conversation.ask"].ResourceID != "conv_http" {
		t.Fatalf("conversation resource id = %q", actions["conversation.ask"].ResourceID)
	}

	filtered := httptest.NewRecorder()
	filteredReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1&action=search.completed&outcome=succeeded&actor_user_id=user_1&query=hit_count&from=2026-06-23&to=2026-06-23&limit=10", nil)
	server.ServeHTTP(filtered, filteredReq)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered status = %d, want %d, body = %s", filtered.Code, http.StatusOK, filtered.Body.String())
	}
	var filteredBody struct {
		Events []auditEventPayload `json:"events"`
	}
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredBody); err != nil {
		t.Fatalf("decode filtered body: %v", err)
	}
	if len(filteredBody.Events) != 1 {
		t.Fatalf("filtered events len = %d, want 1: %#v", len(filteredBody.Events), filteredBody.Events)
	}
	if filteredBody.Events[0].Action != "search.completed" {
		t.Fatalf("filtered action = %q, want search.completed", filteredBody.Events[0].Action)
	}

	invalidDate := httptest.NewRecorder()
	invalidDateReq := httptest.NewRequest(http.MethodGet, "/v1/audit-events?tenant_id=tenant_1&from=not-a-date", nil)
	server.ServeHTTP(invalidDate, invalidDateReq)
	if invalidDate.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status = %d, want %d, body = %s", invalidDate.Code, http.StatusBadRequest, invalidDate.Body.String())
	}
}

func TestAskConversationStreamEndpoint(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask/stream", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"question": "What is the alpha beta plan?",
		"limit": 1
	}`))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", contentType)
	}

	body := resp.Body.String()
	for _, want := range []string{
		"event: status",
		`"message":"Retrieving"`,
		`"message":"Generating"`,
		"event: delta",
		`"content":"Answer "`,
		`"content":"from fake model"`,
		"event: done",
		`"assistant_message"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body missing %q: %s", want, body)
		}
	}
}

func TestConversationHistoryEndpoints(t *testing.T) {
	server := newTestServer(t)

	ask := httptest.NewRecorder()
	askReq := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"question": "What is the alpha beta plan?",
		"limit": 1
	}`))
	server.ServeHTTP(ask, askReq)
	if ask.Code != http.StatusOK {
		t.Fatalf("ask status = %d, want %d, body = %s", ask.Code, http.StatusOK, ask.Body.String())
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations?tenant_id=tenant_1&limit=5", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", list.Code, http.StatusOK, list.Body.String())
	}

	var listBody struct {
		Conversations []conversationPayload `json:"conversations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	if len(listBody.Conversations) != 1 {
		t.Fatalf("conversations len = %d, want 1", len(listBody.Conversations))
	}
	if listBody.Conversations[0].ID != "conv_http" {
		t.Fatalf("conversation id = %q, want conv_http", listBody.Conversations[0].ID)
	}

	messages := httptest.NewRecorder()
	messagesReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/conv_http/messages?tenant_id=tenant_1", nil)
	server.ServeHTTP(messages, messagesReq)
	if messages.Code != http.StatusOK {
		t.Fatalf("messages status = %d, want %d, body = %s", messages.Code, http.StatusOK, messages.Body.String())
	}

	var messagesBody struct {
		Messages []messagePayload `json:"messages"`
	}
	if err := json.Unmarshal(messages.Body.Bytes(), &messagesBody); err != nil {
		t.Fatalf("decode messages body: %v", err)
	}
	if len(messagesBody.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messagesBody.Messages))
	}
	if messagesBody.Messages[0].Role != string(domain.MessageRoleUser) || messagesBody.Messages[1].Role != string(domain.MessageRoleAssistant) {
		t.Fatalf("roles = %s/%s, want user/assistant", messagesBody.Messages[0].Role, messagesBody.Messages[1].Role)
	}
}

func TestDeleteConversationEndpoint(t *testing.T) {
	server := newTestServer(t)

	ask := httptest.NewRecorder()
	askReq := httptest.NewRequest(http.MethodPost, "/v1/conversations/ask", bytes.NewBufferString(`{
		"tenant_id": "tenant_1",
		"question": "What is the alpha beta plan?",
		"limit": 1
	}`))
	server.ServeHTTP(ask, askReq)
	if ask.Code != http.StatusOK {
		t.Fatalf("ask status = %d, want %d, body = %s", ask.Code, http.StatusOK, ask.Body.String())
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/conversations/conv_http?tenant_id=tenant_1", nil)
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var body struct {
		Conversation conversationPayload `json:"conversation"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode delete body: %v", err)
	}
	if body.Conversation.ID != "conv_http" {
		t.Fatalf("conversation id = %q, want conv_http", body.Conversation.ID)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/conversations?tenant_id=tenant_1&limit=5", nil)
	server.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", list.Code, http.StatusOK, list.Body.String())
	}
	var listBody struct {
		Conversations []conversationPayload `json:"conversations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	if len(listBody.Conversations) != 0 {
		t.Fatalf("conversations len = %d, want 0", len(listBody.Conversations))
	}

	messages := httptest.NewRecorder()
	messagesReq := httptest.NewRequest(http.MethodGet, "/v1/conversations/conv_http/messages?tenant_id=tenant_1", nil)
	server.ServeHTTP(messages, messagesReq)
	if messages.Code != http.StatusNotFound {
		t.Fatalf("messages status = %d, want %d, body = %s", messages.Code, http.StatusNotFound, messages.Body.String())
	}
}

func TestConversationHistoryEndpointRejectsInvalidLimit(t *testing.T) {
	server := newTestServer(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/conversations?tenant_id=tenant_1&limit=nope", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
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

func TestCORSPreflightForCommaSeparatedAllowedOrigin(t *testing.T) {
	server := corsMiddleware(config.Config{
		Env:               "production",
		CORSAllowedOrigin: "https://nexus.example.test, http://localhost:5173",
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/models/route", nil)
	req.Header.Set("Origin", "https://nexus.example.test")
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "https://nexus.example.test" {
		t.Fatalf("allow origin = %q, want configured origin", got)
	}
}

func TestCORSPreflightAllowsLoopbackFallbackPortInLocalEnv(t *testing.T) {
	server := corsMiddleware(config.Config{
		Env:               "local",
		CORSAllowedOrigin: "http://localhost:5173",
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/me", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5175")
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5175" {
		t.Fatalf("allow origin = %q, want loopback fallback origin", got)
	}
}

func TestCORSPreflightRejectsUnknownOriginInProduction(t *testing.T) {
	server := corsMiddleware(config.Config{
		Env:               "production",
		CORSAllowedOrigin: "https://nexus.example.test",
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/me", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5175")
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow origin = %q, want empty for rejected production origin", got)
	}
}

func TestTrustedHeaderModeRejectsUserWithoutTenantPermission(t *testing.T) {
	server := newTestServerWithConfig(t, config.Config{
		AuthMode:            internalauth.ModeTrustedHeader,
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents?tenant_id=tenant_1", nil)
	req.Header.Set("X-User-ID", "user_without_membership")
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	return newTestServerWithConfig(t, config.Config{
		AuthMode:            internalauth.ModeDev,
		DevUserID:           "user_1",
		DevUserEmail:        "dev@example.local",
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	})
}

func newTestServerWithSeed(t *testing.T, seed func(*memory.Store)) http.Handler {
	t.Helper()

	return newTestServerWithConfigAndSeed(t, config.Config{
		AuthMode:            internalauth.ModeDev,
		DevUserID:           "user_1",
		DevUserEmail:        "dev@example.local",
		TrustedUserIDHeader: "X-User-ID",
		TrustedEmailHeader:  "X-User-Email",
	}, seed)
}

func newTestServerWithConfig(t *testing.T, authCfg config.Config) http.Handler {
	t.Helper()

	return newTestServerWithConfigAndSeed(t, authCfg, nil)
}

func newTestServerWithConfigAndSeed(t *testing.T, authCfg config.Config, seed func(*memory.Store)) http.Handler {
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
		DeploymentProfile:   "cpu-lite",
		AuthMode:            authCfg.AuthMode,
		DevUserID:           authCfg.DevUserID,
		DevUserEmail:        authCfg.DevUserEmail,
		TrustedUserIDHeader: authCfg.TrustedUserIDHeader,
		TrustedEmailHeader:  authCfg.TrustedEmailHeader,
		PersistenceBackend:  "memory",
		RunMigrations:       false,
		DatabaseURL:         "postgres://test",
		ObjectStoreEndpoint: "http://minio.test",
		VectorBackend:       "qdrant",
		QueueBackend:        "nats",
		ProviderPreset:      "starter",
		ModelGatewayBaseURL: "http://gpu.local:8000/v1",
	}

	repos := memory.New()
	if seed != nil {
		seed(repos)
	}
	ids := &httpIDs{}
	embedder := embeddinghash.New("test", 16)
	objectStore := objectmemory.New()
	vectorIndex := vectormemory.New()
	documents := app.NewDocumentService(repos, ids, httpClock{}).
		WithObjectStore(objectStore).
		WithVectorIndex(vectorIndex)
	dataSources := app.NewDataSourceService(repos, ids, httpClock{}).
		WithObjectStore(objectStore).
		WithVectorIndex(vectorIndex)
	jobs := app.NewJobService(repos)
	audit := app.NewAuditService(repos, ids, httpClock{})
	seedEmbedding, err := embedder.Embed(context.Background(), providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed seed: %v", err)
	}
	if err := vectorIndex.Upsert(context.Background(), []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_search"),
			ChunkID:    "chunk_1",
			Values:     seedEmbedding.Vectors[0],
			Text:       "alpha beta launch plan",
			Metadata: map[string]string{
				"document_name": "Alpha Plan.md",
				"section":       "planning",
				"chunk_index":   "0",
			},
		},
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_notes"),
			ChunkID:    "chunk_2",
			Values:     []float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Text:       "omega archive notes",
			Metadata: map[string]string{
				"document_name": "Omega Notes.md",
				"section":       "archive",
				"chunk_index":   "0",
			},
		},
	}); err != nil {
		t.Fatalf("seed vectors: %v", err)
	}
	search := app.NewSearchService(embedder, vectorIndex)
	conversations := app.NewConversationService(repos, ids, httpClock{}, search, httpModelGateway{})
	tenants := app.NewTenantService(repos, ids, httpClock{})
	sourceViews := app.NewSourceViewService(repos, ids, httpClock{})
	sourcePolicyProfiles := app.NewSourcePolicyProfileService(repos, ids, httpClock{})

	return NewRouter(cfg, Dependencies{
		ModelRouter:          router,
		ModelGateway:         httpModelGateway{},
		Tenants:              tenants,
		Documents:            documents,
		DataSources:          dataSources,
		SourceViews:          sourceViews,
		SourcePolicyProfiles: sourcePolicyProfiles,
		Jobs:                 jobs,
		Audit:                audit,
		Search:               search,
		Conversations:        conversations,
		Authenticator:        internalauth.NewAuthenticator(cfg),
		Authorizer:           internalauth.NewAuthorizer(cfg, repos),
	})
}

type httpIDs struct {
	job     int
	message int
	audit   int
}

func (httpIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_http")
}

func (httpIDs) NewDataSourceID() domain.DataSourceID {
	return domain.DataSourceID("src_http")
}

func (httpIDs) NewSourceViewID() domain.SourceViewID {
	return domain.SourceViewID("view_http")
}

func (httpIDs) NewSourcePolicyProfileID() domain.SourcePolicyProfileID {
	return domain.SourcePolicyProfileID("policy_http")
}

func (httpIDs) NewTenantID() domain.TenantID {
	return domain.TenantID("tenant_http")
}

func (g *httpIDs) NewJobID() domain.JobID {
	g.job++
	if g.job == 1 {
		return domain.JobID("job_http")
	}
	return domain.JobID(fmt.Sprintf("job_http_%d", g.job))
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

func (g *httpIDs) NewAuditEventID() domain.AuditEventID {
	g.audit++
	return domain.AuditEventID(fmt.Sprintf("audit_http_%d", g.audit))
}

type httpClock struct{}

func (httpClock) Now() time.Time {
	return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
}

func newHTTPFailedDocument(t *testing.T) domain.Document {
	t.Helper()

	document, err := domain.NewDocument(domain.DocumentCreate{
		ID:         domain.DocumentID("doc_failed_http"),
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		Name:       "Broken Handbook.md",
		StorageKey: "tenants/tenant_1/documents/doc_failed_http/Broken Handbook.md",
		SizeBytes:  42,
		Now:        httpClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	if err := document.Transition(domain.DocumentStatusFailed, httpClock{}.Now()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	return document
}

func newHTTPScanEntry(t *testing.T, source domain.DataSource, jobID domain.JobID, path string, outcome domain.DataSourceScanOutcome, reason string, now time.Time) domain.DataSourceScanEntry {
	t.Helper()
	documentID := domain.DocumentID("")
	if outcome == domain.DataSourceScanOutcomeImported {
		documentID = domain.DocumentID("doc_" + strings.ReplaceAll(strings.TrimSuffix(path, ".md"), "/", "_"))
	}
	entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
		TenantID:    source.TenantID,
		JobID:       jobID,
		SourceID:    source.ID,
		Path:        path,
		Outcome:     outcome,
		Reason:      reason,
		DocumentID:  documentID,
		SizeBytes:   42,
		ContentHash: "sha256:" + path,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("new scan entry: %v", err)
	}
	return entry
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

type failingModelGateway struct{}

func (failingModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	return providers.ChatCompletion{}, fmt.Errorf("gateway unavailable")
}

type capturingModelGateway struct {
	request         providers.ChatCompletionRequest
	deadlineSeconds int
}

func (g *capturingModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	g.request = input
	if deadline, ok := ctx.Deadline(); ok {
		g.deadlineSeconds = int(time.Until(deadline).Round(time.Second) / time.Second)
	}
	return providers.ChatCompletion{
		Model:        "fake-model",
		Content:      "ok",
		FinishReason: "stop",
	}, nil
}

func (httpModelGateway) StreamComplete(ctx context.Context, input providers.ChatCompletionRequest, emit func(providers.ChatCompletionChunk) error) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	for _, content := range []string{"Answer ", "from fake model"} {
		if err := emit(providers.ChatCompletionChunk{
			Model:   "fake-model",
			Content: content,
		}); err != nil {
			return providers.ChatCompletion{}, err
		}
	}
	return providers.ChatCompletion{
		Model:        "fake-model",
		Content:      "Answer from fake model",
		FinishReason: "stop",
	}, nil
}
