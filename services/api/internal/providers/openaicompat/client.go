package openaicompat

import (
	"bufio"
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
	MaxTokens   int                     `json:"max_tokens,omitempty"`
	Metadata    map[string]string       `json:"metadata,omitempty"`
	Stream      bool                    `json:"stream,omitempty"`
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

type chatCompletionStreamResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
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
		MaxTokens:   input.MaxTokens,
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

func (c *Client) StreamComplete(ctx context.Context, input providers.ChatCompletionRequest, emit func(providers.ChatCompletionChunk) error) (providers.ChatCompletion, error) {
	if emit == nil {
		emit = func(providers.ChatCompletionChunk) error { return nil }
	}

	body, err := json.Marshal(chatCompletionRequest{
		Model:       input.Model,
		Messages:    input.Messages,
		Temperature: input.Temperature,
		MaxTokens:   input.MaxTokens,
		Metadata:    input.Metadata,
		Stream:      true,
	})
	if err != nil {
		return providers.ChatCompletion{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
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

	var content strings.Builder
	model := input.Model
	finishReason := ""
	var usage map[string]int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var parsed chatCompletionStreamResponse
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			return providers.ChatCompletion{}, fmt.Errorf("decode stream chunk: %w", err)
		}
		if parsed.Model != "" {
			model = parsed.Model
		}
		if parsed.Usage != nil {
			usage = parsed.Usage
		}
		for _, choice := range parsed.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			if choice.Delta.Content == "" {
				continue
			}
			content.WriteString(choice.Delta.Content)
			if err := emit(providers.ChatCompletionChunk{
				Model:    model,
				Content:  choice.Delta.Content,
				Metadata: input.Metadata,
			}); err != nil {
				return providers.ChatCompletion{}, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}

	return providers.ChatCompletion{
		Model:        model,
		Content:      content.String(),
		FinishReason: finishReason,
		Usage:        usage,
		Metadata:     input.Metadata,
	}, nil
}
