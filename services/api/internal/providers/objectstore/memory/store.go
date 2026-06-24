package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type Store struct {
	mu      sync.RWMutex
	objects map[string]object
}

type object struct {
	content     []byte
	contentType string
	sizeBytes   int64
	updatedAt   time.Time
}

func New() *Store {
	return &Store{objects: map[string]object{}}
}

func (s *Store) PutObject(ctx context.Context, input providers.ObjectPut) (providers.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return providers.ObjectInfo{}, err
	}
	if err := validateTenantKey(input.TenantID, input.Key); err != nil {
		return providers.ObjectInfo{}, err
	}

	content, err := io.ReadAll(input.Body)
	if err != nil {
		return providers.ObjectInfo{}, err
	}
	now := time.Now().UTC()
	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[input.Key] = object{
		content:     content,
		contentType: contentType,
		sizeBytes:   int64(len(content)),
		updatedAt:   now,
	}
	return providers.ObjectInfo{
		Key:         input.Key,
		ContentType: contentType,
		SizeBytes:   int64(len(content)),
		UpdatedAt:   now,
	}, nil
}

func (s *Store) GetObject(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, providers.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, providers.ObjectInfo{}, err
	}
	if err := validateTenantKey(tenantID, key); err != nil {
		return nil, providers.ObjectInfo{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.objects[key]
	if !ok {
		return nil, providers.ObjectInfo{}, fmt.Errorf("object %q not found", key)
	}
	return io.NopCloser(bytes.NewReader(object.content)), providers.ObjectInfo{
		Key:         key,
		ContentType: object.contentType,
		SizeBytes:   object.sizeBytes,
		UpdatedAt:   object.updatedAt,
	}, nil
}

func (s *Store) DeleteObject(ctx context.Context, tenantID domain.TenantID, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateTenantKey(tenantID, key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *Store) PresignURL(ctx context.Context, input providers.PresignRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateTenantKey(input.TenantID, input.Key); err != nil {
		return "", err
	}
	return "memory://" + input.Key, nil
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateTenantKey(tenantID domain.TenantID, key string) error {
	prefix := "tenants/" + string(tenantID) + "/"
	if tenantID == "" || key == "" || !strings.HasPrefix(key, prefix) {
		return fmt.Errorf("object key must be scoped under %q", prefix)
	}
	return nil
}
