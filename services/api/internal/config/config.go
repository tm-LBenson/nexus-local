package config

import "os"

type Config struct {
	Env                 string
	HTTPAddr            string
	Version             string
	CORSAllowedOrigin   string
	PersistenceBackend  string
	DatabaseURL         string
	ObjectStoreEndpoint string
	ObjectStoreBucket   string
	VectorBackend       string
	VectorBaseURL       string
	QueueBackend        string
	QueueURL            string
	CacheURL            string
	ModelGatewayBaseURL string
	DefaultModelTarget  string
	GeneralModelID      string
}

func Load() Config {
	return Config{
		Env:                 env("APP_ENV", "local"),
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		Version:             env("APP_VERSION", "dev"),
		CORSAllowedOrigin:   env("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
		PersistenceBackend:  env("PERSISTENCE_BACKEND", "memory"),
		DatabaseURL:         env("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable"),
		ObjectStoreEndpoint: env("OBJECT_STORAGE_ENDPOINT", "http://localhost:9000"),
		ObjectStoreBucket:   env("OBJECT_STORAGE_BUCKET", "documents"),
		VectorBackend:       env("VECTOR_BACKEND", "qdrant"),
		VectorBaseURL:       env("VECTOR_BASE_URL", "http://localhost:6333"),
		QueueBackend:        env("QUEUE_BACKEND", "nats"),
		QueueURL:            env("QUEUE_URL", "nats://localhost:4222"),
		CacheURL:            env("CACHE_URL", "redis://localhost:6379/0"),
		ModelGatewayBaseURL: env("MODEL_GATEWAY_BASE_URL", "http://localhost:8000/v1"),
		DefaultModelTarget:  env("DEFAULT_MODEL_TARGET", "general"),
		GeneralModelID:      env("GENERAL_MODEL_ID", "Qwen/Qwen2.5-7B-Instruct"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
