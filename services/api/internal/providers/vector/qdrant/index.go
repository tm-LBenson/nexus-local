package qdrant

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type Config struct {
	BaseURL    string
	APIKey     string
	Collection string
	Dimensions int
	HTTPClient *http.Client
}

type Index struct {
	baseURL    string
	apiKey     string
	collection string
	dimensions int
	httpClient *http.Client
}

type qdrantResponse[T any] struct {
	Status string `json:"status"`
	Result T      `json:"result"`
}

func New(ctx context.Context, cfg Config) (*Index, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("qdrant base url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("qdrant base url: %w", err)
	}
	if strings.TrimSpace(cfg.Collection) == "" {
		return nil, fmt.Errorf("qdrant collection is required")
	}
	if cfg.Dimensions <= 0 {
		return nil, fmt.Errorf("qdrant dimensions must be positive")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	index := &Index{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		collection: cfg.Collection,
		dimensions: cfg.Dimensions,
		httpClient: httpClient,
	}
	if err := index.EnsureCollection(ctx); err != nil {
		return nil, err
	}
	return index, nil
}

func (i *Index) EnsureCollection(ctx context.Context) error {
	resp, err := i.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(i.collection), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		_ = resp.Body.Close()
		return nil
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("qdrant collection check status %d", resp.StatusCode)
	}

	body := map[string]any{
		"vectors": map[string]any{
			"size":     i.dimensions,
			"distance": "Cosine",
		},
	}
	return i.expectOK(ctx, http.MethodPut, "/collections/"+url.PathEscape(i.collection), body)
}

func (i *Index) Upsert(ctx context.Context, vectors []providers.Vector) error {
	points := make([]map[string]any, 0, len(vectors))
	for _, vector := range vectors {
		points = append(points, map[string]any{
			"id":     pointID(vector),
			"vector": vector.Values,
			"payload": map[string]any{
				"tenant_id":   string(vector.TenantID),
				"document_id": string(vector.DocumentID),
				"chunk_id":    vector.ChunkID,
				"text":        vector.Text,
				"metadata":    vector.Metadata,
			},
		})
	}
	if len(points) == 0 {
		return nil
	}
	return i.expectOK(ctx, http.MethodPut, "/collections/"+url.PathEscape(i.collection)+"/points?wait=true", map[string]any{"points": points})
}

func (i *Index) Search(ctx context.Context, input providers.VectorSearch) ([]providers.VectorHit, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	body := map[string]any{
		"query":        input.Query,
		"limit":        limit,
		"with_payload": true,
		"filter":       filterFrom(input.TenantID, input.DocumentID, input.Filters),
	}

	var response qdrantResponse[struct {
		Points []struct {
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"points"`
	}]
	if err := i.requestJSON(ctx, http.MethodPost, "/collections/"+url.PathEscape(i.collection)+"/points/query", body, &response); err != nil {
		return nil, err
	}

	hits := make([]providers.VectorHit, 0, len(response.Result.Points))
	for _, point := range response.Result.Points {
		payload := point.Payload
		hits = append(hits, providers.VectorHit{
			DocumentID: domain.DocumentID(stringValue(payload["document_id"])),
			ChunkID:    stringValue(payload["chunk_id"]),
			Text:       stringValue(payload["text"]),
			Score:      point.Score,
			Metadata:   metadataValue(payload["metadata"]),
		})
	}
	return hits, nil
}

func (i *Index) DeleteDocument(ctx context.Context, tenantID domain.TenantID, documentID domain.DocumentID) error {
	body := map[string]any{
		"filter": filterFrom(tenantID, documentID, nil),
	}
	return i.expectOK(ctx, http.MethodPost, "/collections/"+url.PathEscape(i.collection)+"/points/delete?wait=true", body)
}

func (i *Index) expectOK(ctx context.Context, method string, path string, body any) error {
	resp, err := i.doJSON(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("qdrant status %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}
	return nil
}

func (i *Index) requestJSON(ctx context.Context, method string, path string, body any, out any) error {
	resp, err := i.doJSON(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("qdrant status %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (i *Index) doJSON(ctx context.Context, method string, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	return i.do(ctx, method, path, reader)
}

func (i *Index) do(ctx context.Context, method string, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, i.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if i.apiKey != "" {
		req.Header.Set("api-key", i.apiKey)
	}
	return i.httpClient.Do(req)
}

func filterFrom(tenantID domain.TenantID, documentID domain.DocumentID, metadata map[string]string) map[string]any {
	must := []map[string]any{
		matchCondition("tenant_id", string(tenantID)),
	}
	if documentID != "" {
		must = append(must, matchCondition("document_id", string(documentID)))
	}
	for key, value := range metadata {
		must = append(must, matchCondition("metadata."+key, value))
	}
	return map[string]any{"must": must}
}

func matchCondition(key string, value string) map[string]any {
	return map[string]any{
		"key": key,
		"match": map[string]string{
			"value": value,
		},
	}
}

func pointID(vector providers.Vector) string {
	raw := fmt.Sprintf("%s/%s/%s", vector.TenantID, vector.DocumentID, vector.ChunkID)
	sum := sha1.Sum([]byte(raw))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func metadataValue(value any) map[string]string {
	output := map[string]string{}
	if value == nil {
		return output
	}
	if metadata, ok := value.(map[string]any); ok {
		for key, value := range metadata {
			output[key] = stringValue(value)
		}
	}
	return output
}
