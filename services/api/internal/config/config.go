package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env                  string
	HTTPAddr             string
	Version              string
	CORSAllowedOrigin    string
	DeploymentProfile    string
	AuthMode             string
	DevUserID            string
	DevUserEmail         string
	TrustedUserIDHeader  string
	TrustedEmailHeader   string
	PersistenceBackend   string
	RunMigrations        bool
	DatabaseURL          string
	ObjectStoreBackend   string
	ObjectStoreEndpoint  string
	ObjectStoreAccessKey string
	ObjectStoreSecretKey string
	ObjectStoreBucket    string
	EmbeddingBackend     string
	EmbeddingBaseURL     string
	EmbeddingAPIKey      string
	EmbeddingModel       string
	EmbeddingDimensions  int
	VectorBackend        string
	VectorBaseURL        string
	VectorCollection     string
	VectorAPIKey         string
	QueueBackend         string
	QueueURL             string
	CacheURL             string
	ProviderPreset       string
	ModelGatewayBaseURL  string
	ModelGatewayAPIKey   string
	DefaultModelTarget   string
	GeneralModelID       string
	WorkerPollInterval   time.Duration
}

func Load() Config {
	return Config{
		Env:                  env("APP_ENV", "local"),
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		Version:              env("APP_VERSION", "dev"),
		CORSAllowedOrigin:    env("CORS_ALLOWED_ORIGIN", "http://localhost:5173,http://127.0.0.1:5173"),
		DeploymentProfile:    env("DEPLOYMENT_PROFILE", "cpu-lite"),
		AuthMode:             env("AUTH_MODE", "dev"),
		DevUserID:            env("DEV_USER_ID", "user_1"),
		DevUserEmail:         env("DEV_USER_EMAIL", "dev@example.local"),
		TrustedUserIDHeader:  env("TRUSTED_USER_ID_HEADER", "X-User-ID"),
		TrustedEmailHeader:   env("TRUSTED_EMAIL_HEADER", "X-User-Email"),
		PersistenceBackend:   env("PERSISTENCE_BACKEND", "memory"),
		RunMigrations:        envBool("RUN_MIGRATIONS", false),
		DatabaseURL:          env("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable"),
		ObjectStoreBackend:   env("OBJECT_STORAGE_BACKEND", "memory"),
		ObjectStoreEndpoint:  env("OBJECT_STORAGE_ENDPOINT", "http://localhost:9000"),
		ObjectStoreAccessKey: env("OBJECT_STORAGE_ACCESS_KEY", "minioadmin"),
		ObjectStoreSecretKey: env("OBJECT_STORAGE_SECRET_KEY", "minioadmin"),
		ObjectStoreBucket:    env("OBJECT_STORAGE_BUCKET", "documents"),
		EmbeddingBackend:     env("EMBEDDING_BACKEND", "hash"),
		EmbeddingBaseURL:     env("EMBEDDING_BASE_URL", "http://localhost:8082/v1"),
		EmbeddingAPIKey:      env("EMBEDDING_API_KEY", ""),
		EmbeddingModel:       env("EMBEDDING_MODEL", "hash-embedding"),
		EmbeddingDimensions:  envInt("EMBEDDING_DIMENSIONS", 384),
		VectorBackend:        env("VECTOR_BACKEND", "memory"),
		VectorBaseURL:        env("VECTOR_BASE_URL", "http://localhost:6333"),
		VectorCollection:     env("VECTOR_COLLECTION", "documents"),
		VectorAPIKey:         env("VECTOR_API_KEY", ""),
		QueueBackend:         env("QUEUE_BACKEND", "nats"),
		QueueURL:             env("QUEUE_URL", "nats://localhost:4222"),
		CacheURL:             env("CACHE_URL", "redis://localhost:6379/0"),
		ProviderPreset:       env("PROVIDER_PRESET", "starter"),
		ModelGatewayBaseURL:  env("MODEL_GATEWAY_BASE_URL", "http://localhost:8000/v1"),
		ModelGatewayAPIKey:   env("MODEL_GATEWAY_API_KEY", ""),
		DefaultModelTarget:   env("DEFAULT_MODEL_TARGET", "general"),
		GeneralModelID:       env("GENERAL_MODEL_ID", "Qwen/Qwen2.5-7B-Instruct"),
		WorkerPollInterval:   envDuration("WORKER_POLL_INTERVAL", 2*time.Second),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
