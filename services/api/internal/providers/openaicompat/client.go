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
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type chatCompletionRequest struct {
	Model       string                  `json:"model"`
	Messages    []providers.ChatMessage `json:"messages"`
	Temperature float32                 `json:"temperature,omitempty"`
	Metadata    map[string]string       `json:"metadata,omitempty"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]int `json:"usage,omitempty"`
}

func New(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("base url: %w", err)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &Client{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: httpClient,
	}, nil
}

func (c *Client) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model:       input.Model,
		Messages:    input.Messages,
		Temperature: input.Temperature,
		Metadata:    input.Metadata,
	})
	if err != nil {
		return providers.ChatCompletion{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return providers.ChatCompletion{}, fmt.Errorf("model gateway status %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var parsed chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return providers.ChatCompletion{}, err
	}
	if len(parsed.Choices) == 0 {
		return providers.ChatCompletion{}, fmt.Errorf("model gateway returned no choices")
	}

	return providers.ChatCompletion{
		Model:        parsed.Model,
		Content:      parsed.Choices[0].Message.Content,
		FinishReason: parsed.Choices[0].FinishReason,
		Usage:        parsed.Usage,
		Metadata:     input.Metadata,
	}, nil
}
