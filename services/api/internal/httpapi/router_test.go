package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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

func newTestServerWithConfig(t *testing.T, authCfg config.Config) http.Handler {
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
		ModelGatewayBaseURL: "http://gpu.local:8000/v1",
	}

	repos := memory.New()
	ids := &httpIDs{}
	embedder := embeddinghash.New("test", 16)
	vectorIndex := vectormemory.New()
	documents := app.NewDocumentService(repos, ids, httpClock{}).
		WithObjectStore(objectmemory.New()).
		WithVectorIndex(vectorIndex)
	jobs := app.NewJobService(repos)
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
	tenants := app.NewTenantService(repos, ids, httpClock{})

	return NewRouter(cfg, Dependencies{
		ModelRouter:   router,
		Tenants:       tenants,
		Documents:     documents,
		Jobs:          jobs,
		Search:        search,
		Conversations: conversations,
		Authenticator: internalauth.NewAuthenticator(cfg),
		Authorizer:    internalauth.NewAuthorizer(cfg, repos),
	})
}

type httpIDs struct {
	message int
}

func (httpIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_http")
}

func (httpIDs) NewTenantID() domain.TenantID {
	return domain.TenantID("tenant_http")
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
