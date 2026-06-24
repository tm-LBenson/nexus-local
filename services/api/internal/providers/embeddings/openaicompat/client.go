package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

type ClientConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type embeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

type embeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func New(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("embedding base url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("embedding base url: %w", err)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return &Client{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      strings.TrimSpace(cfg.Model),
		httpClient: httpClient,
	}, nil
}

func (c *Client) Embed(ctx context.Context, input providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = c.model
	}
	if model == "" {
		return providers.EmbeddingResponse{}, fmt.Errorf("embedding model is required")
	}

	body, err := json.Marshal(embeddingRequest{
		Model:          model,
		Input:          input.Texts,
		EncodingFormat: "float",
	})
	if err != nil {
		return providers.EmbeddingResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return providers.EmbeddingResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return providers.EmbeddingResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return providers.EmbeddingResponse{}, fmt.Errorf("embedding gateway status %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var parsed embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return providers.EmbeddingResponse{}, err
	}
	vectors := make([][]float32, len(input.Texts))
	seen := make([]bool, len(input.Texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(input.Texts) {
			return providers.EmbeddingResponse{}, fmt.Errorf("embedding index %d out of range", item.Index)
		}
		vectors[item.Index] = item.Embedding
		seen[item.Index] = true
	}
	for index, ok := range seen {
		if !ok {
			return providers.EmbeddingResponse{}, fmt.Errorf("embedding response missing index %d", index)
		}
	}

	responseModel := parsed.Model
	if responseModel == "" {
		responseModel = model
	}
	return providers.EmbeddingResponse{
		Model:   responseModel,
		Vectors: vectors,
	}, nil
}
