package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestClientCompletePostsChatCompletionRequest(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "test-model",
			"choices": [
				{
					"message": {"role": "assistant", "content": "Hello from the GPU"},
					"finish_reason": "stop"
				}
			],
			"usage": {"prompt_tokens": 5, "completion_tokens": 4, "total_tokens": 9}
		}`))
	}))
	defer server.Close()

	client, err := New(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.Complete(context.Background(), providers.ChatCompletionRequest{
		Model: "test-model",
		Messages: []providers.ChatMessage{
			{Role: "user", Content: "Say hi"},
		},
		Temperature: 0.2,
		MaxTokens:   12,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if captured["model"] != "test-model" {
		t.Fatalf("model = %v, want test-model", captured["model"])
	}
	if captured["max_tokens"] != float64(12) {
		t.Fatalf("max_tokens = %v, want 12", captured["max_tokens"])
	}
	if result.Content != "Hello from the GPU" {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Usage["total_tokens"] != 9 {
		t.Fatalf("total tokens = %d, want 9", result.Usage["total_tokens"])
	}
}

func TestClientStreamCompleteParsesChatCompletionChunks(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"delta\":{\"content\":\"from stream\"},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := New(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var chunks []string
	result, err := client.StreamComplete(context.Background(), providers.ChatCompletionRequest{
		Model: "test-model",
		Messages: []providers.ChatMessage{
			{Role: "user", Content: "Say hi"},
		},
		Temperature: 0.2,
		MaxTokens:   10,
		Metadata:    map[string]string{"tenant_id": "tenant_1"},
	}, func(chunk providers.ChatCompletionChunk) error {
		chunks = append(chunks, chunk.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}

	if captured["stream"] != true {
		t.Fatalf("stream = %v, want true", captured["stream"])
	}
	if captured["max_tokens"] != float64(10) {
		t.Fatalf("max_tokens = %v, want 10", captured["max_tokens"])
	}
	if strings.Join(chunks, "") != "Hello from stream" {
		t.Fatalf("chunks = %q", chunks)
	}
	if result.Content != "Hello from stream" {
		t.Fatalf("content = %q", result.Content)
	}
	if result.FinishReason != "stop" {
		t.Fatalf("finish reason = %q, want stop", result.FinishReason)
	}
	if result.Usage["total_tokens"] != 7 {
		t.Fatalf("total tokens = %d, want 7", result.Usage["total_tokens"])
	}
	if result.Metadata["tenant_id"] != "tenant_1" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestClientCompleteHandlesNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Complete(context.Background(), providers.ChatCompletionRequest{
		Model: "test-model",
		Messages: []providers.ChatMessage{
			{Role: "user", Content: "Say hi"},
		},
	})
	if err == nil {
		t.Fatal("err = nil, want non-success error")
	}
}

func TestNewUsesConfiguredTimeout(t *testing.T) {
	client, err := New(ClientConfig{
		BaseURL: "http://gateway.local/v1",
		Timeout: 4 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if client.httpClient.Timeout != 4*time.Minute {
		t.Fatalf("timeout = %s, want 4m", client.httpClient.Timeout)
	}
}

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	_, err := New(ClientConfig{})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}
