package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type envelope map[string]any

func NewRouter(cfg config.Config, modelRouter *providers.ModelRouter) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler(cfg))
	mux.HandleFunc("GET /readyz", readinessHandler(cfg))
	mux.HandleFunc("GET /v1/model-targets", modelTargetsHandler(modelRouter))
	mux.HandleFunc("POST /v1/models/route", modelRouteHandler(modelRouter))

	return loggingMiddleware(corsMiddleware(cfg, mux))
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
			"persistence_backend":     cfg.PersistenceBackend,
			"database_configured":     cfg.DatabaseURL != "",
			"object_store_configured": cfg.ObjectStoreEndpoint != "",
			"vector_backend":          cfg.VectorBackend,
			"queue_backend":           cfg.QueueBackend,
			"model_gateway":           cfg.ModelGatewayBaseURL,
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

func modelRouteHandler(modelRouter *providers.ModelRouter) http.HandlerFunc {
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
