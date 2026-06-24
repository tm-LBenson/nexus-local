package minioobject

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type Store struct {
	client *minio.Client
	bucket string
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	endpoint, secure, err := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("object storage bucket is required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}

	store := &Store{
		client: client,
		bucket: cfg.Bucket,
	}
	if err := store.EnsureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *Store) PutObject(ctx context.Context, input providers.ObjectPut) (providers.ObjectInfo, error) {
	if err := validateTenantKey(input.TenantID, input.Key); err != nil {
		return providers.ObjectInfo{}, err
	}
	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	info, err := s.client.PutObject(ctx, s.bucket, input.Key, input.Body, input.SizeBytes, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"tenant-id": string(input.TenantID),
		},
	})
	if err != nil {
		return providers.ObjectInfo{}, err
	}

	return providers.ObjectInfo{
		Key:         info.Key,
		ContentType: contentType,
		SizeBytes:   info.Size,
		UpdatedAt:   time.Now().UTC(),
	}, nil
}

func (s *Store) GetObject(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, providers.ObjectInfo, error) {
	if err := validateTenantKey(tenantID, key); err != nil {
		return nil, providers.ObjectInfo{}, err
	}
	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, providers.ObjectInfo{}, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, providers.ObjectInfo{}, err
	}
	return object, providers.ObjectInfo{
		Key:         key,
		ContentType: stat.ContentType,
		SizeBytes:   stat.Size,
		UpdatedAt:   stat.LastModified,
	}, nil
}

func (s *Store) DeleteObject(ctx context.Context, tenantID domain.TenantID, key string) error {
	if err := validateTenantKey(tenantID, key); err != nil {
		return err
	}
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *Store) PresignURL(ctx context.Context, input providers.PresignRequest) (string, error) {
	if err := validateTenantKey(input.TenantID, input.Key); err != nil {
		return "", err
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	switch strings.ToUpper(input.Operation) {
	case "", http.MethodGet:
		u, err := s.client.PresignedGetObject(ctx, s.bucket, input.Key, ttl, nil)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	case http.MethodPut:
		u, err := s.client.PresignedPutObject(ctx, s.bucket, input.Key, ttl)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	default:
		return "", fmt.Errorf("unsupported presign operation %q", input.Operation)
	}
}

func normalizeEndpoint(raw string, useSSL bool) (string, bool, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", false, fmt.Errorf("object storage endpoint is required")
	}

	parsed, err := url.Parse(value)
	if err == nil && parsed.Host != "" {
		switch parsed.Scheme {
		case "http":
			return parsed.Host, false, nil
		case "https":
			return parsed.Host, true, nil
		default:
			return "", false, fmt.Errorf("unsupported object storage endpoint scheme %q", parsed.Scheme)
		}
	}

	return value, useSSL, nil
}

func validateTenantKey(tenantID domain.TenantID, key string) error {
	prefix := "tenants/" + string(tenantID) + "/"
	if tenantID == "" || key == "" || !strings.HasPrefix(key, prefix) {
		return fmt.Errorf("object key must be scoped under %q", prefix)
	}
	return nil
}
