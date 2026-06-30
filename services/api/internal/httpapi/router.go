package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
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
	ModelRouter          *providers.ModelRouter
	ModelGateway         providers.ModelGateway
	Tenants              app.TenantService
	Documents            app.DocumentService
	DataSources          app.DataSourceService
	SourceViews          app.SourceViewService
	SourcePolicyProfiles app.SourcePolicyProfileService
	Jobs                 app.JobService
	Audit                app.AuditService
	Search               app.SearchService
	Conversations        app.ConversationService
	Authenticator        internalauth.Authenticator
	Authorizer           internalauth.Authorizer
}

func NewRouter(cfg config.Config, deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler(cfg))
	mux.HandleFunc("GET /readyz", readinessHandler(cfg))
	mux.HandleFunc("GET /v1/me", currentUserHandler(deps.Tenants))
	mux.HandleFunc("POST /v1/tenants", createTenantHandler(deps.Tenants))
	mux.HandleFunc("DELETE /v1/tenants/{tenant_id}", deleteTenantHandler(deps.Tenants, deps.Authorizer))
	mux.HandleFunc("GET /v1/tenants/{tenant_id}/members", listTenantMembersHandler(deps.Tenants, deps.Authorizer))
	mux.HandleFunc("POST /v1/tenants/{tenant_id}/members", addTenantMemberHandler(deps.Tenants, deps.Authorizer))
	mux.HandleFunc("DELETE /v1/tenants/{tenant_id}/members/{user_id}", deleteTenantMemberHandler(deps.Tenants, deps.Authorizer))
	mux.HandleFunc("GET /v1/model-targets", modelTargetsHandler(deps.ModelRouter))
	mux.HandleFunc("POST /v1/model-targets/check", modelTargetCheckHandler(deps.ModelRouter, deps.ModelGateway, deps.Authorizer))
	mux.HandleFunc("POST /v1/models/route", modelRouteHandler(deps.ModelRouter, deps.Authorizer))
	mux.HandleFunc("GET /v1/documents", listDocumentsHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("GET /v1/documents/{document_id}", getDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("GET /v1/documents/{document_id}/download", downloadDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("POST /v1/documents/register", registerDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("POST /v1/documents/upload", uploadDocumentHandler(deps.Documents, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/documents/{document_id}/retry", retryDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("DELETE /v1/documents/{document_id}", deleteDocumentHandler(deps.Documents, deps.Authorizer))
	mux.HandleFunc("GET /v1/data-sources", listDataSourcesHandler(deps.DataSources, deps.Authorizer))
	mux.HandleFunc("POST /v1/data-sources", createDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/preflight", preflightDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/plan", planDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/scan", scanDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/scan/cancel", cancelDataSourceScanHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/reindex", reindexDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/data-sources/{source_id}/retry-failed-documents", retryFailedDataSourceDocumentsHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("GET /v1/data-sources/{source_id}/scan-entries.csv", exportDataSourceScanEntriesHandler(deps.DataSources, deps.Authorizer))
	mux.HandleFunc("GET /v1/data-sources/{source_id}", getDataSourceHandler(deps.DataSources, deps.Authorizer))
	mux.HandleFunc("PATCH /v1/data-sources/{source_id}", updateDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("DELETE /v1/data-sources/{source_id}", archiveDataSourceHandler(deps.DataSources, deps.Authorizer, deps.Audit))
	mux.HandleFunc("GET /v1/source-views", listSourceViewsHandler(deps.SourceViews, deps.Authorizer))
	mux.HandleFunc("POST /v1/source-views", createSourceViewHandler(deps.SourceViews, deps.Authorizer, deps.Audit))
	mux.HandleFunc("PATCH /v1/source-views/{view_id}", updateSourceViewHandler(deps.SourceViews, deps.Authorizer, deps.Audit))
	mux.HandleFunc("DELETE /v1/source-views/{view_id}", deleteSourceViewHandler(deps.SourceViews, deps.Authorizer, deps.Audit))
	mux.HandleFunc("GET /v1/source-policy-profiles", listSourcePolicyProfilesHandler(deps.SourcePolicyProfiles, deps.Authorizer))
	mux.HandleFunc("POST /v1/source-policy-profiles", createSourcePolicyProfileHandler(deps.SourcePolicyProfiles, deps.Authorizer, deps.Audit))
	mux.HandleFunc("PATCH /v1/source-policy-profiles/{profile_id}", updateSourcePolicyProfileHandler(deps.SourcePolicyProfiles, deps.Authorizer, deps.Audit))
	mux.HandleFunc("DELETE /v1/source-policy-profiles/{profile_id}", deleteSourcePolicyProfileHandler(deps.SourcePolicyProfiles, deps.Authorizer, deps.Audit))
	mux.HandleFunc("GET /v1/jobs", listJobsHandler(deps.Jobs, deps.Authorizer))
	mux.HandleFunc("GET /v1/audit-events", listAuditEventsHandler(deps.Audit, deps.Authorizer))
	mux.HandleFunc("POST /v1/search", searchHandler(deps.Search, deps.Authorizer, deps.Audit))
	mux.HandleFunc("GET /v1/conversations", listConversationsHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("GET /v1/conversations/{conversation_id}/messages", listConversationMessagesHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("DELETE /v1/conversations/{conversation_id}", deleteConversationHandler(deps.Conversations, deps.Authorizer))
	mux.HandleFunc("POST /v1/conversations/ask", askConversationHandler(deps.Conversations, deps.Authorizer, deps.Audit))
	mux.HandleFunc("POST /v1/conversations/ask/stream", askConversationStreamHandler(deps.Conversations, deps.Authorizer, deps.Audit))

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
			"deployment_profile":      cfg.DeploymentProfile,
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
			"provider_preset":         cfg.ProviderPreset,
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

func modelTargetCheckHandler(modelRouter *providers.ModelRouter, modelGateway providers.ModelGateway, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if modelRouter == nil {
			writeError(w, http.StatusServiceUnavailable, "model router is not configured")
			return
		}
		if modelGateway == nil {
			writeError(w, http.StatusServiceUnavailable, "model gateway is not configured")
			return
		}

		var req modelTargetCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if strings.TrimSpace(req.TenantID) != "" {
			if _, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionUseAI); !ok {
				return
			}
		}

		route, err := modelRouter.Route(r.Context(), providers.ModelRequest{
			Target:   req.Target,
			TenantID: req.TenantID,
		})
		if err != nil {
			writeError(w, modelRouteStatus(err), err.Error())
			return
		}

		checkCtx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		started := time.Now()
		completion, err := modelGateway.Complete(checkCtx, providers.ChatCompletionRequest{
			Target: route.Target,
			Model:  route.Model,
			Messages: []providers.ChatMessage{
				{Role: "system", Content: "Reply with exactly: ok"},
				{Role: "user", Content: "Nexus Local model target check. Reply with exactly: ok."},
			},
			Temperature: 0,
			MaxTokens:   8,
			Metadata: map[string]string{
				"purpose": "model_target_check",
			},
		})
		if err != nil {
			writeError(w, modelTargetCheckStatus(err), fmt.Sprintf("model target check: %v", err))
			return
		}

		writeJSON(w, http.StatusOK, envelope{
			"status":        "ok",
			"route":         route,
			"model":         completion.Model,
			"finish_reason": completion.FinishReason,
			"latency_ms":    time.Since(started).Milliseconds(),
		})
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
			writeError(w, modelRouteStatus(err), err.Error())
			return
		}

		writeJSON(w, http.StatusOK, envelope{"route": route})
	}
}

func modelRouteStatus(err error) int {
	if errors.Is(err, providers.ErrUnknownTarget) {
		return http.StatusNotFound
	}
	if errors.Is(err, providers.ErrEmptyTarget) || errors.Is(err, domain.ErrInvalidEntity) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func modelTargetCheckStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if errors.Is(err, providers.ErrUnknownTarget) {
		return http.StatusNotFound
	}
	if errors.Is(err, providers.ErrEmptyTarget) || errors.Is(err, domain.ErrInvalidEntity) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

type registerDocumentRequest struct {
	TenantID   string `json:"tenant_id"`
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	StorageKey string `json:"storage_key"`
	SizeBytes  int64  `json:"size_bytes"`
}

type dataSourceRequest struct {
	TenantID            string   `json:"tenant_id"`
	Type                string   `json:"type"`
	Name                string   `json:"name"`
	RootPath            string   `json:"root_path"`
	IncludePatterns     []string `json:"include_patterns"`
	ExcludePatterns     []string `json:"exclude_patterns"`
	ScanIntervalMinutes int      `json:"scan_interval_minutes"`
}

type sourceViewFiltersRequest struct {
	Health   string `json:"health"`
	Query    string `json:"query"`
	Schedule string `json:"schedule"`
	Type     string `json:"type"`
}

type sourceViewRequest struct {
	TenantID string                   `json:"tenant_id"`
	Name     string                   `json:"name"`
	Filters  sourceViewFiltersRequest `json:"filters"`
}

type sourcePolicyProfileRequest struct {
	TenantID            string   `json:"tenant_id"`
	Name                string   `json:"name"`
	Detail              string   `json:"detail"`
	IncludePatterns     []string `json:"include_patterns"`
	ExcludePatterns     []string `json:"exclude_patterns"`
	ScanIntervalMinutes int      `json:"scan_interval_minutes"`
}

type createTenantRequest struct {
	Name string `json:"name"`
}

type tenantMemberRequest struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

type modelTargetCheckRequest struct {
	TenantID string `json:"tenant_id"`
	Target   string `json:"target"`
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

type tenantMemberPayload struct {
	User userPayload `json:"user"`
	Role string      `json:"role"`
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

type dataSourcePayload struct {
	ID                  string   `json:"id"`
	TenantID            string   `json:"tenant_id"`
	OwnerID             string   `json:"owner_id"`
	Type                string   `json:"type"`
	Name                string   `json:"name"`
	RootPath            string   `json:"root_path"`
	IncludePatterns     []string `json:"include_patterns"`
	ExcludePatterns     []string `json:"exclude_patterns"`
	ScanIntervalMinutes int      `json:"scan_interval_minutes"`
	NextScanAt          string   `json:"next_scan_at,omitempty"`
	Status              string   `json:"status"`
	LastScanAt          string   `json:"last_scan_at,omitempty"`
	LastScanImported    int      `json:"last_scan_imported"`
	LastScanSkipped     int      `json:"last_scan_skipped"`
	LastScanFailed      int      `json:"last_scan_failed"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

type dataSourceScanEntryPayload struct {
	TenantID    string `json:"tenant_id"`
	JobID       string `json:"job_id"`
	SourceID    string `json:"source_id"`
	Path        string `json:"path"`
	Outcome     string `json:"outcome"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
	DocumentID  string `json:"document_id,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type dataSourceScanSummaryPayload struct {
	Total       int            `json:"total"`
	Imported    int            `json:"imported"`
	Skipped     int            `json:"skipped"`
	Failed      int            `json:"failed"`
	Deleted     int            `json:"deleted"`
	LatestJobID string         `json:"latest_job_id,omitempty"`
	LatestAt    string         `json:"latest_at,omitempty"`
	Reasons     map[string]int `json:"reasons"`
}

type dataSourceScanEntryPagePayload struct {
	Total   int    `json:"total"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	Outcome string `json:"outcome,omitempty"`
	HasMore bool   `json:"has_more"`
}

type sourceViewFiltersPayload struct {
	Health   string `json:"health"`
	Query    string `json:"query"`
	Schedule string `json:"schedule"`
	Type     string `json:"type"`
}

type sourceViewPayload struct {
	ID        string                   `json:"id"`
	TenantID  string                   `json:"tenant_id"`
	OwnerID   string                   `json:"owner_id"`
	Name      string                   `json:"name"`
	Filters   sourceViewFiltersPayload `json:"filters"`
	CreatedAt string                   `json:"created_at"`
	UpdatedAt string                   `json:"updated_at"`
}

type sourcePolicyProfilePayload struct {
	ID                  string   `json:"id"`
	TenantID            string   `json:"tenant_id"`
	OwnerID             string   `json:"owner_id"`
	Name                string   `json:"name"`
	Detail              string   `json:"detail"`
	IncludePatterns     []string `json:"include_patterns"`
	ExcludePatterns     []string `json:"exclude_patterns"`
	ScanIntervalMinutes int      `json:"scan_interval_minutes"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

type jobPayload struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Type         string `json:"type"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	State        string `json:"state"`
	Attempts     int    `json:"attempts"`
	ErrorMessage string `json:"error_message"`
	ResultJSON   string `json:"result_json,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type auditEventPayload struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	ActorUserID  string            `json:"actor_user_id"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	Outcome      string            `json:"outcome"`
	Metadata     map[string]string `json:"metadata"`
	CreatedAt    string            `json:"created_at"`
}

type searchRequest struct {
	TenantID   string            `json:"tenant_id"`
	DocumentID string            `json:"document_id"`
	Query      string            `json:"query"`
	Limit      int               `json:"limit"`
	Filters    map[string]string `json:"filters"`
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
	DocumentID     string `json:"document_id"`
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

func listTenantMembersHandler(service app.TenantService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.PathValue("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionManageUsers); !ok {
			return
		}

		result, err := service.ListTenantMembers(r.Context(), app.ListTenantMembersInput{
			TenantID: tenantID,
		})
		if err != nil {
			writeTenantMemberError(w, "list tenant members", err)
			return
		}
		members := make([]tenantMemberPayload, 0, len(result.Members))
		for _, member := range result.Members {
			members = append(members, encodeTenantMember(member))
		}
		writeJSON(w, http.StatusOK, envelope{
			"tenant":  encodeTenant(result.Tenant),
			"members": members,
		})
	}
}

func addTenantMemberHandler(service app.TenantService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.PathValue("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionManageUsers); !ok {
			return
		}

		var req tenantMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		result, err := service.AddTenantMember(r.Context(), app.AddTenantMemberInput{
			TenantID: tenantID,
			UserID:   domain.UserID(req.UserID),
			Email:    req.Email,
			Name:     req.Name,
			Role:     domain.Role(req.Role),
		})
		if err != nil {
			writeTenantMemberError(w, "add tenant member", err)
			return
		}
		writeJSON(w, http.StatusCreated, envelope{
			"tenant": encodeTenant(result.Tenant),
			"member": encodeTenantMember(result.Member),
		})
	}
}

func deleteTenantMemberHandler(service app.TenantService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.PathValue("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionManageUsers); !ok {
			return
		}

		result, err := service.DeleteTenantMember(r.Context(), app.DeleteTenantMemberInput{
			TenantID: tenantID,
			UserID:   domain.UserID(r.PathValue("user_id")),
		})
		if err != nil {
			writeTenantMemberError(w, "delete tenant member", err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{
			"tenant": encodeTenant(result.Tenant),
			"member": encodeTenantMember(result.Member),
		})
	}
}

func deleteTenantHandler(service app.TenantService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.PathValue("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionManageTenant); !ok {
			return
		}

		result, err := service.DeleteTenant(r.Context(), app.DeleteTenantInput{
			TenantID: tenantID,
		})
		if err != nil {
			writeTenantMemberError(w, "delete tenant", err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{
			"tenant": encodeTenant(result.Tenant),
		})
	}
}

func writeTenantMemberError(w http.ResponseWriter, action string, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, domain.ErrInvalidEntity) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, app.ErrLastOwner) {
		status = http.StatusConflict
	}
	if errors.Is(err, app.ErrTenantNotEmpty) {
		status = http.StatusConflict
	}
	writeError(w, status, fmt.Sprintf("%s: %v", action, err))
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
			if errors.Is(err, app.ErrUnsupportedDocumentType) {
				status = http.StatusUnsupportedMediaType
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

func downloadDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		documentID := domain.DocumentID(r.PathValue("document_id"))

		result, err := service.DownloadDocument(r.Context(), app.DownloadDocumentInput{
			TenantID:   tenantID,
			DocumentID: documentID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, app.ErrObjectStoreUnavailable) {
				status = http.StatusServiceUnavailable
			}
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, fmt.Sprintf("download document: %v", err))
			return
		}
		defer result.Body.Close()

		contentType := result.Object.ContentType
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": result.Document.Name,
		}))
		if result.Object.SizeBytes >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(result.Object.SizeBytes, 10))
		}
		if _, err := io.Copy(w, result.Body); err != nil {
			log.Printf("download document %s failed: %v", result.Document.ID, err)
		}
	}
}

func uploadDocumentHandler(service app.DocumentService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
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

		documentName := strings.TrimSpace(r.FormValue("name"))
		if documentName == "" {
			documentName = header.Filename
		}

		result, err := service.UploadDocument(r.Context(), app.UploadDocumentInput{
			TenantID:    tenantID,
			OwnerID:     principal.UserID,
			Name:        documentName,
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
			if errors.Is(err, app.ErrUnsupportedDocumentType) {
				status = http.StatusUnsupportedMediaType
			}
			writeError(w, status, fmt.Sprintf("upload document: %v", err))
			return
		}
		recordAudit(r.Context(), audit, app.RecordAuditInput{
			TenantID:     tenantID,
			ActorUserID:  principal.UserID,
			Action:       "document.uploaded",
			ResourceType: "document",
			ResourceID:   string(result.Document.ID),
			Outcome:      domain.AuditOutcomeSucceeded,
			Metadata: map[string]string{
				"name":       result.Document.Name,
				"size_bytes": strconv.FormatInt(result.Document.SizeBytes, 10),
			},
		})

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

func listDataSourcesHandler(service app.DataSourceService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		includeArchived := strings.EqualFold(r.URL.Query().Get("include_archived"), "true")
		result, err := service.List(r.Context(), app.ListDataSourcesInput{
			TenantID:        tenantID,
			IncludeArchived: includeArchived,
		})
		if err != nil {
			writeDataSourceError(w, "list data sources", err)
			return
		}

		sources := make([]dataSourcePayload, 0, len(result.Sources))
		for _, source := range result.Sources {
			sources = append(sources, encodeDataSource(source))
		}
		writeJSON(w, http.StatusOK, envelope{"sources": sources})
	}
}

func createDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req dataSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}

		result, err := service.Create(r.Context(), app.CreateDataSourceInput{
			TenantID:            tenantID,
			OwnerID:             principal.UserID,
			Type:                domain.DataSourceType(req.Type),
			Name:                req.Name,
			RootPath:            req.RootPath,
			IncludePatterns:     req.IncludePatterns,
			ExcludePatterns:     req.ExcludePatterns,
			ScanIntervalMinutes: req.ScanIntervalMinutes,
		})
		if err != nil {
			writeDataSourceError(w, "create data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.created", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusCreated, envelope{"source": encodeDataSource(result.Source)})
	}
}

func getDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		limit, offset, outcome, err := scanEntryQuery(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.Get(r.Context(), app.DataSourceDetailInput{
			TenantID:         tenantID,
			DataSourceID:     domain.DataSourceID(r.PathValue("source_id")),
			ScanEntryLimit:   limit,
			ScanEntryOffset:  offset,
			ScanEntryOutcome: outcome,
		})
		if err != nil {
			writeDataSourceError(w, "get data source", err)
			return
		}
		jobs := make([]jobPayload, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, encodeJob(job))
		}
		entries := make([]dataSourceScanEntryPayload, 0, len(result.ScanEntries))
		for _, entry := range result.ScanEntries {
			entries = append(entries, encodeDataSourceScanEntry(entry))
		}
		writeJSON(w, http.StatusOK, envelope{
			"source":            encodeDataSource(result.Source),
			"jobs":              jobs,
			"scan_entries":      entries,
			"scan_summary":      encodeDataSourceScanSummary(result.ScanSummary),
			"scan_entries_page": encodeDataSourceScanEntryPage(result.ScanPage),
			"failed_documents":  result.FailedDocuments,
		})
	}
}

func exportDataSourceScanEntriesHandler(service app.DataSourceService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		_, _, outcome, err := scanEntryQuery(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.ExportScanEntries(r.Context(), app.ExportDataSourceScanEntriesInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
			Outcome:      outcome,
		})
		if err != nil {
			writeDataSourceError(w, "export data source scan entries", err)
			return
		}
		filename := fmt.Sprintf("nexus-local-%s-scan-entries.csv", result.Source.ID)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		w.Header().Set("X-Nexus-Scan-Entry-Total", strconv.Itoa(result.Total))
		w.Header().Set("X-Nexus-Scan-Entry-Truncated", strconv.FormatBool(result.Truncated))
		w.WriteHeader(http.StatusOK)
		writer := csv.NewWriter(w)
		if err := writer.Write([]string{"tenant_id", "job_id", "source_id", "path", "outcome", "reason", "message", "document_id", "size_bytes", "content_hash", "created_at"}); err != nil {
			log.Printf("write scan entries csv header: %v", err)
			return
		}
		for _, entry := range result.Entries {
			if err := writer.Write([]string{
				string(entry.TenantID),
				string(entry.JobID),
				string(entry.SourceID),
				entry.Path,
				string(entry.Outcome),
				entry.Reason,
				entry.Message,
				string(entry.DocumentID),
				strconv.FormatInt(entry.SizeBytes, 10),
				entry.ContentHash,
				entry.CreatedAt.Format(time.RFC3339),
			}); err != nil {
				log.Printf("write scan entries csv row: %v", err)
				return
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			log.Printf("flush scan entries csv: %v", err)
		}
	}
}

func updateDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req dataSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}

		result, err := service.Update(r.Context(), app.UpdateDataSourceInput{
			TenantID:            tenantID,
			DataSourceID:        domain.DataSourceID(r.PathValue("source_id")),
			Type:                domain.DataSourceType(req.Type),
			Name:                req.Name,
			RootPath:            req.RootPath,
			IncludePatterns:     req.IncludePatterns,
			ExcludePatterns:     req.ExcludePatterns,
			ScanIntervalMinutes: req.ScanIntervalMinutes,
		})
		if err != nil {
			writeDataSourceError(w, "update data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.updated", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusOK, envelope{"source": encodeDataSource(result.Source)})
	}
}

func scanDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.RequestScan(r.Context(), app.ScanDataSourceInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "scan data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.scan_requested", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusAccepted, envelope{
			"source": encodeDataSource(result.Source),
			"job":    encodeJob(result.Job),
		})
	}
}

func cancelDataSourceScanHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.CancelScan(r.Context(), app.CancelDataSourceScanInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "cancel data source scan", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.scan_canceled", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusOK, envelope{
			"source": encodeDataSource(result.Source),
			"job":    encodeJob(result.Job),
		})
	}
}

func preflightDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.RequestPreflight(r.Context(), app.PreflightDataSourceInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "preflight data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.preflight_requested", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusAccepted, envelope{
			"source": encodeDataSource(result.Source),
			"job":    encodeJob(result.Job),
		})
	}
}

func planDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.RequestPlan(r.Context(), app.PlanDataSourceInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "plan data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.plan_requested", result.Source, domain.AuditOutcomeSucceeded)
		writeJSON(w, http.StatusAccepted, envelope{
			"source": encodeDataSource(result.Source),
			"job":    encodeJob(result.Job),
		})
	}
}

func reindexDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.RequestReindex(r.Context(), app.ReindexDataSourceInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "reindex data source", err)
			return
		}
		jobs := make([]jobPayload, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, encodeJob(job))
		}
		recordDataSourceReindexAudit(r.Context(), audit, tenantID, principal.UserID, result.Source, len(result.Jobs), result.SkippedDocumentCount)
		writeJSON(w, http.StatusAccepted, envelope{
			"source":            encodeDataSource(result.Source),
			"jobs":              jobs,
			"queued_documents":  len(result.Jobs),
			"skipped_documents": result.SkippedDocumentCount,
		})
	}
}

func retryFailedDataSourceDocumentsHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.RequestRetryFailedDocuments(r.Context(), app.RetryFailedDataSourceDocumentsInput{
			TenantID:     tenantID,
			DataSourceID: domain.DataSourceID(r.PathValue("source_id")),
		})
		if err != nil {
			writeDataSourceError(w, "retry failed data source documents", err)
			return
		}
		jobs := make([]jobPayload, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, encodeJob(job))
		}
		recordDataSourceFailedDocumentRetryAudit(r.Context(), audit, tenantID, principal.UserID, result.Source, len(result.Jobs), result.SkippedDocumentCount)
		writeJSON(w, http.StatusAccepted, envelope{
			"source":            encodeDataSource(result.Source),
			"jobs":              jobs,
			"queued_documents":  len(result.Jobs),
			"skipped_documents": result.SkippedDocumentCount,
		})
	}
}

func archiveDataSourceHandler(service app.DataSourceService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		deleteDocuments := strings.EqualFold(r.URL.Query().Get("delete_documents"), "true")
		result, err := service.Archive(r.Context(), app.ArchiveDataSourceInput{
			TenantID:        tenantID,
			DataSourceID:    domain.DataSourceID(r.PathValue("source_id")),
			DeleteDocuments: deleteDocuments,
		})
		if err != nil {
			writeDataSourceError(w, "archive data source", err)
			return
		}
		recordDataSourceAudit(r.Context(), audit, tenantID, principal.UserID, "data_source.archived", result.Source, domain.AuditOutcomeSucceeded)
		if deleteDocuments {
			recordDataSourceDocumentsDeletedAudit(r.Context(), audit, tenantID, principal.UserID, result.Source, result.DeletedDocumentCount)
		}
		writeJSON(w, http.StatusOK, envelope{
			"source":            encodeDataSource(result.Source),
			"deleted_documents": result.DeletedDocumentCount,
		})
	}
}

func writeDataSourceError(w http.ResponseWriter, action string, err error) {
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
	writeError(w, status, fmt.Sprintf("%s: %v", action, err))
}

func listSourceViewsHandler(service app.SourceViewService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		result, err := service.List(r.Context(), app.ListSourceViewsInput{TenantID: tenantID})
		if err != nil {
			writeSourceViewError(w, "list source views", err)
			return
		}
		views := make([]sourceViewPayload, 0, len(result.Views))
		for _, view := range result.Views {
			views = append(views, encodeSourceView(view))
		}
		writeJSON(w, http.StatusOK, envelope{"views": views})
	}
}

func createSourceViewHandler(service app.SourceViewService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sourceViewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.Create(r.Context(), app.CreateSourceViewInput{
			TenantID: tenantID,
			OwnerID:  principal.UserID,
			Name:     req.Name,
			Filters:  sourceViewFiltersInput(req.Filters),
		})
		if err != nil {
			writeSourceViewError(w, "create source view", err)
			return
		}
		recordSourceViewAudit(r.Context(), audit, tenantID, principal.UserID, "source_view.created", result.View)
		writeJSON(w, http.StatusCreated, envelope{"view": encodeSourceView(result.View)})
	}
}

func updateSourceViewHandler(service app.SourceViewService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sourceViewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.Update(r.Context(), app.UpdateSourceViewInput{
			TenantID:     tenantID,
			SourceViewID: domain.SourceViewID(r.PathValue("view_id")),
			Name:         req.Name,
			Filters:      sourceViewFiltersInput(req.Filters),
		})
		if err != nil {
			writeSourceViewError(w, "update source view", err)
			return
		}
		recordSourceViewAudit(r.Context(), audit, tenantID, principal.UserID, "source_view.updated", result.View)
		writeJSON(w, http.StatusOK, envelope{"view": encodeSourceView(result.View)})
	}
}

func deleteSourceViewHandler(service app.SourceViewService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		viewID := domain.SourceViewID(r.PathValue("view_id"))
		if err := service.Delete(r.Context(), app.DeleteSourceViewInput{
			TenantID:     tenantID,
			SourceViewID: viewID,
		}); err != nil {
			writeSourceViewError(w, "delete source view", err)
			return
		}
		recordAudit(r.Context(), audit, app.RecordAuditInput{
			TenantID:     tenantID,
			ActorUserID:  principal.UserID,
			Action:       "source_view.deleted",
			ResourceType: "source_view",
			ResourceID:   string(viewID),
			Outcome:      domain.AuditOutcomeSucceeded,
			Metadata:     map[string]string{},
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func sourceViewFiltersInput(filters sourceViewFiltersRequest) app.SourceViewFiltersInput {
	return app.SourceViewFiltersInput{
		Health:   filters.Health,
		Query:    filters.Query,
		Schedule: filters.Schedule,
		Type:     filters.Type,
	}
}

func writeSourceViewError(w http.ResponseWriter, action string, err error) {
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
	writeError(w, status, fmt.Sprintf("%s: %v", action, err))
}

func recordSourceViewAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, action string, view domain.SourceView) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: "source_view",
		ResourceID:   string(view.ID),
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			"name": view.Name,
		},
	})
}

func listSourcePolicyProfilesHandler(service app.SourcePolicyProfileService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionReadDocuments); !ok {
			return
		}
		result, err := service.List(r.Context(), app.ListSourcePolicyProfilesInput{TenantID: tenantID})
		if err != nil {
			writeSourcePolicyProfileError(w, "list source policy profiles", err)
			return
		}
		profiles := make([]sourcePolicyProfilePayload, 0, len(result.Profiles))
		for _, profile := range result.Profiles {
			profiles = append(profiles, encodeSourcePolicyProfile(profile))
		}
		writeJSON(w, http.StatusOK, envelope{"profiles": profiles})
	}
}

func createSourcePolicyProfileHandler(service app.SourcePolicyProfileService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sourcePolicyProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.Create(r.Context(), app.CreateSourcePolicyProfileInput{
			TenantID:            tenantID,
			OwnerID:             principal.UserID,
			Name:                req.Name,
			Detail:              req.Detail,
			IncludePatterns:     req.IncludePatterns,
			ExcludePatterns:     req.ExcludePatterns,
			ScanIntervalMinutes: req.ScanIntervalMinutes,
		})
		if err != nil {
			writeSourcePolicyProfileError(w, "create source policy profile", err)
			return
		}
		recordSourcePolicyProfileAudit(r.Context(), audit, tenantID, principal.UserID, "source_policy_profile.created", result.Profile)
		writeJSON(w, http.StatusCreated, envelope{"profile": encodeSourcePolicyProfile(result.Profile)})
	}
}

func updateSourcePolicyProfileHandler(service app.SourcePolicyProfileService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sourcePolicyProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		tenantID := domain.TenantID(req.TenantID)
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		result, err := service.Update(r.Context(), app.UpdateSourcePolicyProfileInput{
			TenantID:              tenantID,
			SourcePolicyProfileID: domain.SourcePolicyProfileID(r.PathValue("profile_id")),
			Name:                  req.Name,
			Detail:                req.Detail,
			IncludePatterns:       req.IncludePatterns,
			ExcludePatterns:       req.ExcludePatterns,
			ScanIntervalMinutes:   req.ScanIntervalMinutes,
		})
		if err != nil {
			writeSourcePolicyProfileError(w, "update source policy profile", err)
			return
		}
		recordSourcePolicyProfileAudit(r.Context(), audit, tenantID, principal.UserID, "source_policy_profile.updated", result.Profile)
		writeJSON(w, http.StatusOK, envelope{"profile": encodeSourcePolicyProfile(result.Profile)})
	}
}

func deleteSourcePolicyProfileHandler(service app.SourcePolicyProfileService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		principal, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionUploadDocuments)
		if !ok {
			return
		}
		profileID := domain.SourcePolicyProfileID(r.PathValue("profile_id"))
		if err := service.Delete(r.Context(), app.DeleteSourcePolicyProfileInput{
			TenantID:              tenantID,
			SourcePolicyProfileID: profileID,
		}); err != nil {
			writeSourcePolicyProfileError(w, "delete source policy profile", err)
			return
		}
		recordAudit(r.Context(), audit, app.RecordAuditInput{
			TenantID:     tenantID,
			ActorUserID:  principal.UserID,
			Action:       "source_policy_profile.deleted",
			ResourceType: "source_policy_profile",
			ResourceID:   string(profileID),
			Outcome:      domain.AuditOutcomeSucceeded,
			Metadata:     map[string]string{},
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeSourcePolicyProfileError(w http.ResponseWriter, action string, err error) {
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
	writeError(w, status, fmt.Sprintf("%s: %v", action, err))
}

func recordSourcePolicyProfileAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, action string, profile domain.SourcePolicyProfile) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: "source_policy_profile",
		ResourceID:   string(profile.ID),
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			"name": profile.Name,
		},
	})
}

func recordDataSourceAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, action string, source domain.DataSource, outcome domain.AuditOutcome) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Outcome:      outcome,
		Metadata: map[string]string{
			"name":      source.Name,
			"type":      string(source.Type),
			"root_path": source.RootPath,
			"status":    string(source.Status),
		},
	})
}

func recordDataSourceDocumentsDeletedAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, source domain.DataSource, deletedDocuments int) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "data_source.documents_deleted",
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			"name":              source.Name,
			"type":              string(source.Type),
			"root_path":         source.RootPath,
			"status":            string(source.Status),
			"deleted_documents": strconv.Itoa(deletedDocuments),
		},
	})
}

func recordDataSourceReindexAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, source domain.DataSource, queuedDocuments int, skippedDocuments int) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "data_source.reindex_requested",
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			"name":              source.Name,
			"type":              string(source.Type),
			"root_path":         source.RootPath,
			"status":            string(source.Status),
			"queued_documents":  strconv.Itoa(queuedDocuments),
			"skipped_documents": strconv.Itoa(skippedDocuments),
		},
	})
}

func recordDataSourceFailedDocumentRetryAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorUserID domain.UserID, source domain.DataSource, queuedDocuments int, skippedDocuments int) {
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "data_source.failed_documents_retry_requested",
		ResourceType: "data_source",
		ResourceID:   string(source.ID),
		Outcome:      domain.AuditOutcomeSucceeded,
		Metadata: map[string]string{
			"name":              source.Name,
			"type":              string(source.Type),
			"root_path":         source.RootPath,
			"status":            string(source.Status),
			"queued_documents":  strconv.Itoa(queuedDocuments),
			"skipped_documents": strconv.Itoa(skippedDocuments),
		},
	})
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

func listAuditEventsHandler(service app.AuditService, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := domain.TenantID(r.URL.Query().Get("tenant_id"))
		if _, ok := requireTenantPermission(w, r, authorizer, tenantID, domain.PermissionManageTenant); !ok {
			return
		}
		limit, err := queryInt(r, "limit")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		from, err := queryTime(r, "from", false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from")
			return
		}
		to, err := queryTime(r, "to", true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to")
			return
		}

		result, err := service.List(r.Context(), app.ListAuditEventsInput{
			TenantID:    tenantID,
			Limit:       limit,
			Action:      r.URL.Query().Get("action"),
			Outcome:     domain.AuditOutcome(r.URL.Query().Get("outcome")),
			ActorUserID: domain.UserID(r.URL.Query().Get("actor_user_id")),
			Query:       r.URL.Query().Get("query"),
			From:        from,
			To:          to,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrInvalidEntity) {
				status = http.StatusBadRequest
			}
			writeError(w, status, fmt.Sprintf("list audit events: %v", err))
			return
		}

		events := make([]auditEventPayload, 0, len(result.Events))
		for _, event := range result.Events {
			events = append(events, encodeAuditEvent(event))
		}
		writeJSON(w, http.StatusOK, envelope{"events": events})
	}
}

func searchHandler(service app.SearchService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		principal, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionReadDocuments)
		if !ok {
			return
		}

		result, err := service.Search(r.Context(), app.SearchInput{
			TenantID:   domain.TenantID(req.TenantID),
			DocumentID: domain.DocumentID(req.DocumentID),
			Query:      req.Query,
			Limit:      req.Limit,
			Filters:    req.Filters,
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
		recordAudit(r.Context(), audit, app.RecordAuditInput{
			TenantID:     domain.TenantID(req.TenantID),
			ActorUserID:  principal.UserID,
			Action:       "search.completed",
			ResourceType: "search",
			Outcome:      domain.AuditOutcomeSucceeded,
			Metadata: map[string]string{
				"document_id": req.DocumentID,
				"hit_count":   strconv.Itoa(len(result.Hits)),
				"query_len":   strconv.Itoa(len(strings.TrimSpace(req.Query))),
			},
		})
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

func queryTime(r *http.Request, key string, endOfDay bool) (*time.Time, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return &parsed, nil
}

func scanEntryQuery(r *http.Request) (int, int, domain.DataSourceScanOutcome, error) {
	limit, err := queryInt(r, "scan_entry_limit")
	if err != nil || limit < 0 {
		return 0, 0, "", fmt.Errorf("scan_entry_limit must be a non-negative integer")
	}
	offset, err := queryInt(r, "scan_entry_offset")
	if err != nil || offset < 0 {
		return 0, 0, "", fmt.Errorf("scan_entry_offset must be a non-negative integer")
	}
	outcomeValue := strings.TrimSpace(r.URL.Query().Get("scan_entry_outcome"))
	if outcomeValue == "" {
		outcomeValue = strings.TrimSpace(r.URL.Query().Get("outcome"))
	}
	outcome := domain.DataSourceScanOutcome(outcomeValue)
	if outcome != "" && !outcome.Valid() {
		return 0, 0, "", fmt.Errorf("scan_entry_outcome must be one of imported, skipped, failed, deleted")
	}
	return limit, offset, outcome, nil
}

func askConversationHandler(service app.ConversationService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, tenantID, principal, ok := prepareAskConversation(w, r, authorizer)
		if !ok {
			return
		}
		result, err := service.Ask(r.Context(), askConversationInput(req, tenantID, principal.UserID))
		if err != nil {
			recordAskAudit(r.Context(), audit, tenantID, principal.UserID, req, domain.AuditOutcomeFailed, "", "", 0)
			writeError(w, askConversationStatus(err), fmt.Sprintf("ask conversation: %v", err))
			return
		}
		recordAskAudit(r.Context(), audit, tenantID, principal.UserID, req, domain.AuditOutcomeSucceeded, string(result.Conversation.ID), result.Completion.Model, len(result.Hits))

		writeJSON(w, http.StatusOK, encodeAskConversationResult(result))
	}
}

func askConversationStreamHandler(service app.ConversationService, authorizer internalauth.Authorizer, audit app.AuditService) http.HandlerFunc {
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
			recordAskAudit(r.Context(), audit, tenantID, principal.UserID, req, domain.AuditOutcomeFailed, "", "", 0)
			if writeErr := writeSSE(w, "error", envelope{"message": fmt.Sprintf("ask conversation: %v", err)}); writeErr != nil {
				log.Printf("write sse error: %v", writeErr)
			}
			flusher.Flush()
			return
		}
		recordAskAudit(r.Context(), audit, tenantID, principal.UserID, req, domain.AuditOutcomeSucceeded, string(result.Conversation.ID), result.Completion.Model, len(result.Hits))

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

func recordAskAudit(ctx context.Context, audit app.AuditService, tenantID domain.TenantID, actorID domain.UserID, req askConversationRequest, outcome domain.AuditOutcome, conversationID string, model string, hitCount int) {
	metadata := map[string]string{
		"document_id":   req.DocumentID,
		"model_target":  req.ModelTarget,
		"question_len":  strconv.Itoa(len(strings.TrimSpace(req.Question))),
		"retrieval_lim": strconv.Itoa(req.Limit),
		"hit_count":     strconv.Itoa(hitCount),
	}
	if model != "" {
		metadata["model"] = model
	}
	recordAudit(ctx, audit, app.RecordAuditInput{
		TenantID:     tenantID,
		ActorUserID:  actorID,
		Action:       "conversation.ask",
		ResourceType: "conversation",
		ResourceID:   conversationID,
		Outcome:      outcome,
		Metadata:     metadata,
	})
}

func recordAudit(ctx context.Context, audit app.AuditService, input app.RecordAuditInput) {
	if _, err := audit.Record(ctx, input); err != nil {
		log.Printf("record audit event failed: %v", err)
	}
}

func askConversationInput(req askConversationRequest, tenantID domain.TenantID, ownerID domain.UserID) app.AskInput {
	return app.AskInput{
		TenantID:       tenantID,
		OwnerID:        ownerID,
		ConversationID: domain.ConversationID(req.ConversationID),
		DocumentID:     domain.DocumentID(req.DocumentID),
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

func encodeDataSource(source domain.DataSource) dataSourcePayload {
	lastScanAt := ""
	if source.LastScanAt != nil {
		lastScanAt = source.LastScanAt.Format(time.RFC3339)
	}
	nextScanAt := ""
	if source.NextScanAt != nil {
		nextScanAt = source.NextScanAt.Format(time.RFC3339)
	}
	return dataSourcePayload{
		ID:                  string(source.ID),
		TenantID:            string(source.TenantID),
		OwnerID:             string(source.OwnerID),
		Type:                string(source.Type),
		Name:                source.Name,
		RootPath:            source.RootPath,
		IncludePatterns:     append([]string{}, source.IncludePatterns...),
		ExcludePatterns:     append([]string{}, source.ExcludePatterns...),
		ScanIntervalMinutes: source.ScanIntervalMinutes,
		NextScanAt:          nextScanAt,
		Status:              string(source.Status),
		LastScanAt:          lastScanAt,
		LastScanImported:    source.LastScanImported,
		LastScanSkipped:     source.LastScanSkipped,
		LastScanFailed:      source.LastScanFailed,
		CreatedAt:           source.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           source.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeSourceView(view domain.SourceView) sourceViewPayload {
	return sourceViewPayload{
		ID:       string(view.ID),
		TenantID: string(view.TenantID),
		OwnerID:  string(view.OwnerID),
		Name:     view.Name,
		Filters: sourceViewFiltersPayload{
			Health:   view.Filters.Health,
			Query:    view.Filters.Query,
			Schedule: view.Filters.Schedule,
			Type:     string(view.Filters.Type),
		},
		CreatedAt: view.CreatedAt.Format(time.RFC3339),
		UpdatedAt: view.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeSourcePolicyProfile(profile domain.SourcePolicyProfile) sourcePolicyProfilePayload {
	return sourcePolicyProfilePayload{
		ID:                  string(profile.ID),
		TenantID:            string(profile.TenantID),
		OwnerID:             string(profile.OwnerID),
		Name:                profile.Name,
		Detail:              profile.Detail,
		IncludePatterns:     profile.IncludePatterns,
		ExcludePatterns:     profile.ExcludePatterns,
		ScanIntervalMinutes: profile.ScanIntervalMinutes,
		CreatedAt:           profile.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           profile.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeDataSourceScanEntry(entry domain.DataSourceScanEntry) dataSourceScanEntryPayload {
	return dataSourceScanEntryPayload{
		TenantID:    string(entry.TenantID),
		JobID:       string(entry.JobID),
		SourceID:    string(entry.SourceID),
		Path:        entry.Path,
		Outcome:     string(entry.Outcome),
		Reason:      entry.Reason,
		Message:     entry.Message,
		DocumentID:  string(entry.DocumentID),
		SizeBytes:   entry.SizeBytes,
		ContentHash: entry.ContentHash,
		CreatedAt:   entry.CreatedAt.Format(time.RFC3339),
	}
}

func encodeDataSourceScanSummary(summary app.DataSourceScanSummary) dataSourceScanSummaryPayload {
	latestAt := ""
	if !summary.LatestAt.IsZero() {
		latestAt = summary.LatestAt.Format(time.RFC3339)
	}
	reasons := map[string]int{}
	for reason, count := range summary.Reasons {
		reasons[reason] = count
	}
	return dataSourceScanSummaryPayload{
		Total:       summary.Total,
		Imported:    summary.Imported,
		Skipped:     summary.Skipped,
		Failed:      summary.Failed,
		Deleted:     summary.Deleted,
		LatestJobID: string(summary.LatestJobID),
		LatestAt:    latestAt,
		Reasons:     reasons,
	}
}

func encodeDataSourceScanEntryPage(page app.DataSourceScanEntryPage) dataSourceScanEntryPagePayload {
	return dataSourceScanEntryPagePayload{
		Total:   page.Total,
		Limit:   page.Limit,
		Offset:  page.Offset,
		Outcome: string(page.Outcome),
		HasMore: page.Offset+page.Limit < page.Total,
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

func encodeTenantMember(member app.TenantMemberSummary) tenantMemberPayload {
	return tenantMemberPayload{
		User: encodeUser(member.User),
		Role: string(member.Membership.Role),
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
		ErrorMessage: job.ErrorMessage,
		ResultJSON:   job.ResultJSON,
		CreatedAt:    job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    job.UpdatedAt.Format(time.RFC3339),
	}
}

func encodeAuditEvent(event domain.AuditEvent) auditEventPayload {
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	return auditEventPayload{
		ID:           string(event.ID),
		TenantID:     string(event.TenantID),
		ActorUserID:  string(event.ActorUserID),
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Outcome:      string(event.Outcome),
		Metadata:     metadata,
		CreatedAt:    event.CreatedAt.Format(time.RFC3339),
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
		if allowedOrigin, ok := allowedCORSOrigin(cfg, origin); ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID, X-User-Email")
			if allowedOrigin != origin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func allowedCORSOrigin(cfg config.Config, origin string) (string, bool) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", false
	}
	for _, allowed := range strings.Split(cfg.CORSAllowedOrigin, ",") {
		allowed = strings.TrimSpace(allowed)
		switch {
		case allowed == "":
			continue
		case allowed == "*":
			return origin, true
		case allowed == origin:
			return origin, true
		}
	}
	if isLocalEnv(cfg.Env) && isLoopbackOrigin(origin) {
		return origin, true
	}
	return "", false
}

func isLocalEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "dev", "development", "local", "test":
		return true
	default:
		return false
	}
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
