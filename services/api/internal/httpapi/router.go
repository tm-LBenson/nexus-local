package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type envelope map[string]any

type Dependencies struct {
	ModelRouter   *providers.ModelRouter
	Tenants       app.TenantService
	Documents     app.DocumentService
	Jobs          app.JobService
	Search        app.SearchService
	Conversations app.ConversationService
	Authenticator internalauth.Authenticator
	Authorizer    internalauth.Authorizer
}

func NewRouter(cfg config.Config, deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler(cfg))
	mux.HandleFunc("GET /readyz", readinessHandler(cfg))
	mux.HandleFunc("GET /v1/me", currentUserHandler(deps.Tenants))
	mux.HandleFunc("POST /v1/tenants", createTenantHandler(deps.Tenants))
	mux.HandleFunc("GET /v1/model-targets", modelTargetsHandler(deps.ModelRouter))
	mux.HandleFunc("POST /v1/models/route", modelRouteHandler(deps.ModelRouter, deps.Authorizer))
	mux.HandleFunc("GET /v1/documents", listDocumentsHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("GET /v1/documents/{document_id}", getDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("POST /v1/documents/register", registerDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("POST /v1/documents/upload", uploadDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("POST /v1/documents/{document_id}/retry", retryDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("DELETE /v1/documents/{document_id}", deleteDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("GET /v1/jobs", listJobsHandler(deps.Jobs, deps.Authorizer))
	mux.HandleFunc("POST /v1/search", searchHandler(deps.Search, deps.Authorizer))
	mux.HandleFunc("GET /v1/conversations", listConversationsHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("GET /v1/conversations/{conversation_id}/messages", listConversationMessagesHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("DELETE /v1/conversations/{conversation_id}", deleteConversationHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("POST /v1/conversations/ask", askConversationHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("POST /v1/conversations/ask/stream", askConversationStreamHandler(deps.Conversations, deps.Authorizer))

	return loggingMiddleware(corsMiddleware(cfg, authMiddleware(deps.Authenticator, mux)))
}

func healthHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, envelope{
			"status":  "ok",
			"env":     cfg.Env,
			"version": cfg.Version,
		})
	}
}

func readinessHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, envelope{
			"status":                  "ready",
			"auth_mode":               cfg.AuthMode,
			"persistence_backend":     cfg.PersistenceBackend,
			"run_migrations":          cfg.RunMigrations,
			"object_storage_backend":  cfg.ObjectStoreBackend,
			"embedding_backend":       cfg.EmbeddingBackend,
			"embedding_model":         cfg.EmbeddingModel,
			"embedding_dimensions":    cfg.EmbeddingDimensions,
			"embedding_gateway":       cfg.EmbeddingBaseURL,
			"embedding_gateway_auth":  cfg.EmbeddingAPIKey != "",
			"database_configured":     cfg.DatabaseURL != "",
			"object_store_configured": cfg.ObjectStoreEndpoint != "",
			"vector_backend":          cfg.VectorBackend,
			"vector_collection":       cfg.VectorCollection,
			"queue_backend":           cfg.QueueBackend,
			"model_gateway":           cfg.ModelGatewayBaseURL,
			"model_gateway_auth":      cfg.ModelGatewayAPIKey != "",
		})
	}
}

func modelTargetsHandler(modelRouter *providers.ModelRouter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if modelRouter == nil {
			writeError(w, http.StatusServiceUnavailable, "model router is not configured")
			return
		}
		writeJSON(w, http.StatusOK, envelope{"targets": modelRouter.Targets()})
	}
}

func modelRouteHandler(modelRouter *providers.ModelRouter, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if modelRouter == nil {
			writeError(w, http.StatusServiceUnavailable, "model router is not configured")
			return
		}

		var req providers.ModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if strings.TrimSpace(req.TenantID) != "" {
			if _, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionUseAI); !ok {
				return
			}
		}

		route, err := modelRouter.Route(r.Context(), req)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, providers.ErrUnknownTarget) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, envelope{"route": route})
	}
}

type registerDocumentRequest struct {
	TenantID   string `json:"tenant_id"`
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	StorageKey string `json:"storage_key"`
	SizeBytes  int64  `json:"size_bytes"`
}

type createTenantRequest struct {
	Name string `json:"name"`
}

type userPayload struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type tenantPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type membershipPayload struct {
	Tenant tenantPayload `json:"tenant"`
	Role   string        `json:"role"`
}

type documentPayload struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	StorageKey string `json:"storage_key"`
	SizeBytes  int64  `json:"size_bytes"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type jobPayload struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Type         string `json:"type"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	State        string `json:"state"`
	Attempts     int    `json:"attempts"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type searchRequest struct {
	TenantID string            `json:"tenant_id"`
	Query    string            `json:"query"`
	Limit    int               `json:"limit"`
	Filters  map[string]string `json:"filters"`
}

type searchHitPayload struct {
	DocumentID string            `json:"document_id"`
	ChunkID    string            `json:"chunk_id"`
	Source     sourcePayload     `json:"source"`
	Text       string            `json:"text"`
	Score      float32           `json:"score"`
	Metadata   map[string]string `json:"metadata"`
}

type sourcePayload struct {
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	ChunkID      string `json:"chunk_id"`
	ChunkIndex   string `json:"chunk_index,omitempty"`
	StorageKey   string `json:"storage_key,omitempty"`
}

type askConversationRequest struct {
	TenantID       string `json:"tenant_id"`
	OwnerID        string `json:"owner_id"`
	ConversationID string `json:"conversation_id"`
	ModelTarget    string `json:"model_target"`
	Question       string `json:"question"`
	Limit          int    `json:"limit"`
}

type conversationPayload struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	OwnerID     string `json:"owner_id"`
	Title       string `json:"title"`
	ModelTarget string `json:"model_target"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type messagePayload struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	CreatedAt      string `json:"created_at"`
}

func currentUserHandler(service app.TenantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromRequest(w, r)
		if !ok {
			return
		}
		result, err := service.CurrentUser(r.Context(), principal)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("current user: %v", err))
			return
		}

		memberships := make([]membershipPayload, 0, len(result.Memberships))
		for _, membership := range result.Memberships {
			memberships = append(memberships, encodeMembershipSummary(membership))
		}
		writeJSON(w, http.StatusOK, envelope{
			"user":        encodeUser(result.User),
			"memberships": memberships,
		})
	}
}

func createTenantHandler(service app.TenantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromRequest(w, r)
		if !ok {
			return
		}

		var req createTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		result, err := service.CreateTenant(r.Context(), app.CreateTenantInput{
			Principal: principal,
			Name:      req.Name,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("create tenant: %v", err))
			return
		}

		writeJSON(w, http.StatusCreated, envelope{
			"user":   encodeUser(result.User),
			"tenant": encodeTenant(result.Tenant),
			"membership": encodeMembershipSummary(app.MembershipSummary{
				Tenant:     result.Tenant,
				Membership: result.Membership,
			}),
		})
	}
}

func registerDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		principal, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionUploadDocuments)
		if !ok {
			return
		}

		result, err := service.RegisterDocument(r.Context(), app.RegisterDocumentInput{
			TenantID:   domain.TenantID(req.TenantID),
			OwnerID:    principal.UserID,
			Name:       req.Name,
			StorageKey: req.StorageKey,
			SizeBytes:  req.SizeBytes,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, envelope{
			"document": encodeDocument(result.Document),
			"job":      encodeJob(result.Job),
		})
	}
}

func listDocumentsHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		result, err := service.ListDocuments(r.Context(), app.ListDocumentsInput{
			TenantID: tenantID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("list documents: %v", err))
			return
		}

		documents := make([]documentPayload, 0, len(result.Documents))
		for _, document := range result.Documents {
			documents = append(documents, encodeDocument(document))
		}
		writeJSON(w, http.StatusOK, envelope{"documents": documents})
	}
}

func getDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		documentID := domain.DocumentID(r.PathValue("document_id"))

		result, err := service.GetDocumentDetail(r.Context(), app.DocumentDetailInput{
			TenantID:   tenantID,
			DocumentID: documentID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("get document: %v", err))
			return
		}

		jobs := make([]jobPayload, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, encodeJob(job))
		}
		writeJSON(w, http.StatusOK, envelope{
			"document": encodeDocument(result.Document),
			"jobs":     jobs,
		})
	}
}

func uploadDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		tenantID := domain.TenantID(r.FormValue("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}

		result, err := service.UploadDocument(r.Context(), app.UploadDocumentInput{
			TenantID:    tenantID,
			OwnerID:     principal.UserID,
			Name:        header.Filename,
			ContentType: contentType,
			SizeBytes:   header.Size,
			Body:        file,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, app.ErrObjectStoreUnavailable) {
				status = http.StatusServiceUnavailable
			}
			writeError(w, status, fmt.Sprintf("upload document: %v", err))
			return
		}

		writeJSON(w, http.StatusCreated, envelope{
			"document": encodeDocument(result.Document),
			"job":      encodeJob(result.Job),
		})
	}
}

func retryDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments); !ok {
			return
		}
		documentID := domain.DocumentID(r.PathValue("document_id"))

		result, err := service.RetryDocumentIngestion(r.Context(), app.RetryDocumentInput{
			TenantID:   tenantID,
			DocumentID: documentID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, domain.ErrInvalidStateTransition) {
				status = http.StatusConflict
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("retry document: %v", err))
			return
		}

		writeJSON(w, http.StatusCreated, envelope{
			"document": encodeDocument(result.Document),
			"job":      encodeJob(result.Job),
		})
	}
}

func deleteDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments); !ok {
			return
		}
		documentID := domain.DocumentID(r.PathValue("document_id"))

		result, err := service.DeleteDocument(r.Context(), app.DeleteDocumentInput{
			TenantID:   tenantID,
			DocumentID: documentID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("delete document: %v", err))
			return
		}

		writeJSON(w, http.StatusOK, envelope{"document": encodeDocument(result.Document)})
	}
}

func listJobsHandler(service app.JobService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		limit, err := queryInt(r, "limit")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}

		result, err := service.ListJobs(r.Context(), app.ListJobsInput{
			TenantID: tenantID,
			Limit:    limit,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("list jobs: %v", err))
			return
		}

		jobs := make([]jobPayload, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, encodeJob(job))
		}
		writeJSON(w, http.StatusOK, envelope{"jobs": jobs})
	}
}

func searchHandler(service app.SearchService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if _, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionReadDocuments); !ok {
			return
		}

		result, err := service.Search(r.Context(), app.SearchInput{
			TenantID: domain.TenantID(req.TenantID),
			Query:    req.Query,
			Limit:    req.Limit,
			Filters:  req.Filters,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, app.ErrSearchUnavailable) {
				status = http.StatusServiceUnavailable
			}
			writeError(w, status, fmt.Sprintf("search: %v", err))
			return
		}

		hits := make([]searchHitPayload, 0, len(result.Hits))
		for _, hit := range result.Hits {
			hits = append(hits, encodeSearchHit(hit))
		}
		writeJSON(w, http.StatusOK, envelope{"hits": hits})
	}
}

func listConversationsHandler(service app.ConversationService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUseAI); !ok {
			return
		}
		limit, err := queryInt(r, "limit")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}

		result, err := service.ListConversations(r.Context(), app.ListConversationsInput{
			TenantID: tenantID,
			Limit:    limit,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("list conversations: %v", err))
			return
		}

		conversations := make([]conversationPayload, 0, len(result.Conversations))
		for _, conversation := range result.Conversations {
			conversations = append(conversations, encodeConversation(conversation))
		}
		writeJSON(w, http.StatusOK, envelope{"conversations": conversations})
	}
}

func listConversationMessagesHandler(service app.ConversationService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUseAI); !ok {
			return
		}
		conversationID := domain.ConversationID(r.PathValue("conversation_id"))

		result, err := service.ListMessages(r.Context(), app.ListMessagesInput{
			TenantID:       tenantID,
			ConversationID: conversationID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("list conversation messages: %v", err))
			return
		}

		messages := make([]messagePayload, 0, len(result.Messages))
		for _, message := range result.Messages {
			messages = append(messages, encodeMessage(message))
		}
		writeJSON(w, http.StatusOK, envelope{"messages": messages})
	}
}

func deleteConversationHandler(service app.ConversationService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUseAI); !ok {
			return
		}
		conversationID := domain.ConversationID(r.PathValue("conversation_id"))

		result, err := service.DeleteConversation(r.Context(), app.DeleteConversationInput{
			TenantID:       tenantID,
			ConversationID: conversationID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("delete conversation: %v", err))
			return
		}

		writeJSON(w, http.StatusOK, envelope{"conversation": encodeConversation(result.Conversation)})
	}
}

func queryInt(r *http.Request, key string) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func askConversationHandler(service app.ConversationService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, tenantID, principal, ok := prepareAskConversation(w, r, authorizer)
		if !ok {
			return
		}
		result, err := service.Ask(r.Context(), askConversationInput(req, tenantID, principal.UserID))
		if err != nil {
			writeError(w, askConversationStatus(err), fmt.Sprintf("ask conversation: %v", err))
			return
		}

		writeJSON(w, http.StatusOK, encodeAskConversationResult(result))
	}
}

func askConversationStreamHandler(service app.ConversationService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming is not supported")
			return
		}

		req, tenantID, principal, ok := prepareAskConversation(w, r, authorizer)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		result, err := service.AskStream(r.Context(), askConversationInput(req, tenantID, principal.UserID), func(event app.AskStreamEvent) error {
			switch event.Type {
			case app.AskStreamStatus:
				if err := writeSSE(w, "status", envelope{"message": event.Message}); err != nil {
					return err
				}
			case app.AskStreamDelta:
				if err := writeSSE(w, "delta", envelope{"content": event.Delta}); err != nil {
					return err
				}
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			if writeErr := writeSSE(w, "error", envelope{"message": fmt.Sprintf("ask conversation: %v", err)}); writeErr != nil {
				log.Printf("write sse error: %v", writeErr)
			}
			flusher.Flush()
			return
		}

		if err := writeSSE(w, "done", encodeAskConversationResult(result)); err != nil {
			log.Printf("write sse done: %v", err)
			return
		}
		flusher.Flush()
	}
}

func prepareAskConversation(w http.ResponseWriter, r *http.Request, authorizer internalauth.Authorizer) (askConversationRequest, domain.TenantID, internalauth.Principal, bool) {
	var req askConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return askConversationRequest{}, "", internalauth.Principal{}, false
	}
	tenantID := domain.TenantID(req.TenantID)
	principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments)
	if !ok {
		return askConversationRequest{}, "", internalauth.Principal{}, false
	}
	if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUseAI); !ok {
		return askConversationRequest{}, "", internalauth.Principal{}, false
	}
	return req, tenantID, principal, true
}

func askConversationInput(req askConversationRequest, tenantID domain.TenantID, ownerID domain.UserID) app.AskInput {
	return app.AskInput{
		TenantID:       tenantID,
		OwnerID:        ownerID,
		ConversationID: domain.ConversationID(req.ConversationID),
		ModelTarget:    req.ModelTarget,
		Question:       req.Question,
		Limit:          req.Limit,
	}
}

func askConversationStatus(err error) int {
	status := http.StatusInternalServerError
	if errors.Is(err, domain.ErrInvalidEntity) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, providers.ErrUnknownTarget) {
		status = http.StatusNotFound
	}
	if errors.Is(err, app.ErrSearchUnavailable) || errors.Is(err, app.ErrConversationModelUnavailable) {
		status = http.StatusServiceUnavailable
	}
	return status
}

func encodeAskConversationResult(result app.AskResult) envelope {
	hits := make([]searchHitPayload, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, encodeSearchHit(hit))
	}
	return envelope{
		"conversation":      encodeConversation(result.Conversation),
		"user_message":      encodeMessage(result.UserMessage),
		"assistant_message": encodeMessage(result.AssistantMessage),
		"hits":              hits,
		"completion":        result.Completion,
	}
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", encoded)
	return err
}

func encodeDocument(document domain.Document) documentPayload {
	return documentPayload{
		ID:         string(document.ID),
		TenantID:   string(document.TenantID),
		OwnerID:    string(document.OwnerID),
		Name:       document.Name,
		StorageKey: document.StorageKey,
		SizeBytes:  document.SizeBytes,
		Status:     string(document.Status),
		CreatedAt:  document.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  document.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeUser(user domain.User) userPayload {
	return userPayload{
		ID:    string(user.ID),
		Email: user.Email,
		Name:  user.Name,
	}
}

func encodeTenant(tenant domain.Tenant) tenantPayload {
	return tenantPayload{
		ID:        string(tenant.ID),
		Name:      tenant.Name,
		CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
		UpdatedAt: tenant.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeMembershipSummary(summary app.MembershipSummary) membershipPayload {
	return membershipPayload{
		Tenant: encodeTenant(summary.Tenant),
		Role:   string(summary.Membership.Role),
	}
}

func encodeJob(job domain.Job) jobPayload {
	return jobPayload{
		ID:           string(job.ID),
		TenantID:     string(job.TenantID),
		Type:         string(job.Type),
		ResourceType: job.ResourceType,
		ResourceID:   job.ResourceID,
		State:        string(job.State),
		Attempts:     job.Attempts,
		CreatedAt:    job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    job.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeSearchHit(hit providers.VectorHit) searchHitPayload {
	return searchHitPayload{
		DocumentID: string(hit.DocumentID),
		ChunkID:    hit.ChunkID,
		Source:     encodeSearchSource(hit),
		Text:       hit.Text,
		Score:      hit.Score,
		Metadata:   hit.Metadata,
	}
}

func encodeSearchSource(hit providers.VectorHit) sourcePayload {
	documentName := strings.TrimSpace(hit.Metadata["document_name"])
	if documentName == "" {
		documentName = string(hit.DocumentID)
	}
	return sourcePayload{
		DocumentID:   string(hit.DocumentID),
		DocumentName: documentName,
		ChunkID:      hit.ChunkID,
		ChunkIndex:   strings.TrimSpace(hit.Metadata["chunk_index"]),
		StorageKey:   strings.TrimSpace(hit.Metadata["storage_key"]),
	}
}

func encodeConversation(conversation domain.Conversation) conversationPayload {
	return conversationPayload{
		ID:          string(conversation.ID),
		TenantID:    string(conversation.TenantID),
		OwnerID:     string(conversation.OwnerID),
		Title:       conversation.Title,
		ModelTarget: conversation.ModelTarget,
		CreatedAt:   conversation.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   conversation.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeMessage(message domain.Message) messagePayload {
	return messagePayload{
		ID:             string(message.ID),
		TenantID:       string(message.TenantID),
		ConversationID: string(message.ConversationID),
		Role:           string(message.Role),
		Content:        message.Content,
		CreatedAt:      message.CreatedAt.Format(time.RFC3339),
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, envelope{"error": message})
}

func principalFromRequest(w http.ResponseWriter, r *http.Request) (internalauth.Principal, bool) {
	principal, ok := internalauth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, internalauth.ErrUnauthenticated.Error())
		return internalauth.Principal{}, false
	}
	return principal, true
}

func authMiddleware(authenticator internalauth.Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		principal, err := authenticator.Authenticate(r)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, internalauth.ErrUnauthenticated) {
				status = http.StatusUnauthorized
			}
			writeError(w, status, err.Error())
			return
		}
		next.ServeHTTP(w, r.WithContext(internalauth.WithPrincipal(r.Context(), principal)))
	})
}

func requireTenantPermission(w http.ResponseWriter, r *http.Request, authorizer internalauth.Authorizer, tenantID domain.TenantID, permission domain.Permission) (internalauth.Principal, bool) {
	principal, ok := internalauth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, internalauth.ErrUnauthenticated.Error())
		return internalauth.Principal{}, false
	}
	if err := authorizer.Require(r.Context(), principal, tenantID, permission); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, internalauth.ErrUnauthenticated):
			status = http.StatusUnauthorized
		case errors.Is(err, internalauth.ErrForbidden):
			status = http.StatusForbidden
		case errors.Is(err, domain.ErrInvalidEntity):
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return internalauth.Principal{}, false
	}
	return principal, true
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}

func corsMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == cfg.CORSAllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID, X-User-Email")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
