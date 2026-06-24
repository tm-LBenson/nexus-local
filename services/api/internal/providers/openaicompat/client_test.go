package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if captured["model"] != "test-model" {
		t.Fatalf("model = %v, want test-model", captured["model"])
	}
	if result.Content != "Hello from the GPU" {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Usage["total_tokens"] != 9 {
		t.Fatalf("total tokens = %d, want 9", result.Usage["total_tokens"])
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

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	_, err := New(ClientConfig{})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}
