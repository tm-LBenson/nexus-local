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
	PersistenceBackend   string
	RunMigrations        bool
	DatabaseURL          string
	ObjectStoreBackend   string
	ObjectStoreEndpoint  string
	ObjectStoreAccessKey string
	ObjectStoreSecretKey string
	ObjectStoreBucket    string
	VectorBackend        string
	VectorBaseURL        string
	QueueBackend         string
	QueueURL             string
	CacheURL             string
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
		CORSAllowedOrigin:    env("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
		PersistenceBackend:   env("PERSISTENCE_BACKEND", "memory"),
		RunMigrations:        envBool("RUN_MIGRATIONS", false),
		DatabaseURL:          env("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable"),
		ObjectStoreBackend:   env("OBJECT_STORAGE_BACKEND", "memory"),
		ObjectStoreEndpoint:  env("OBJECT_STORAGE_ENDPOINT", "http://localhost:9000"),
		ObjectStoreAccessKey: env("OBJECT_STORAGE_ACCESS_KEY", "minioadmin"),
		ObjectStoreSecretKey: env("OBJECT_STORAGE_SECRET_KEY", "minioadmin"),
		ObjectStoreBucket:    env("OBJECT_STORAGE_BUCKET", "documents"),
		VectorBackend:        env("VECTOR_BACKEND", "qdrant"),
		VectorBaseURL:        env("VECTOR_BASE_URL", "http://localhost:6333"),
		QueueBackend:         env("QUEUE_BACKEND", "nats"),
		QueueURL:             env("QUEUE_URL", "nats://localhost:4222"),
		CacheURL:             env("CACHE_URL", "redis://localhost:6379/0"),
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
