package minioobject

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestStoreIntegrationPutAndGetObject(t *testing.T) {
	endpoint := os.Getenv("TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TEST_MINIO_ENDPOINT to run MinIO integration tests")
	}

	ctx := context.Background()
	store, err := New(ctx, Config{
		Endpoint:  endpoint,
		AccessKey: envOr("TEST_MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: envOr("TEST_MINIO_SECRET_KEY", "minioadmin"),
		Bucket:    "documents-test",
	})
	if err != nil {
		t.Fatalf("new minio store: %v", err)
	}

	key := "tenants/tenant_1/documents/doc_1/Handbook.md"
	info, err := store.PutObject(ctx, providers.ObjectPut{
		TenantID:    domain.TenantID("tenant_1"),
		Key:         key,
		Body:        strings.NewReader("hello world"),
		ContentType: "text/markdown",
		SizeBytes:   11,
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if info.Key != key {
		t.Fatalf("key = %q, want %q", info.Key, key)
	}

	body, got, err := store.GetObject(ctx, domain.TenantID("tenant_1"), key)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("content = %q", content)
	}
	if got.SizeBytes != 11 {
		t.Fatalf("size = %d, want 11", got.SizeBytes)
	}

	url, err := store.PresignURL(ctx, providers.PresignRequest{
		TenantID:  domain.TenantID("tenant_1"),
		Key:       key,
		Operation: "GET",
		TTL:       time.Minute,
	})
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	if url == "" {
		t.Fatal("presign URL is empty")
	}
}

func envOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
