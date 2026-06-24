package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestClientEmbedPostsEmbeddingRequest(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %q, want /embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "embedding-model",
			"data": [
				{"index": 1, "embedding": [0.3, 0.4]},
				{"index": 0, "embedding": [0.1, 0.2]}
			]
		}`))
	}))
	defer server.Close()

	client, err := New(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "embedding-model",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.Embed(context.Background(), providers.EmbeddingRequest{
		Texts: []string{"alpha", "beta"},
	})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	if captured["model"] != "embedding-model" {
		t.Fatalf("model = %v, want embedding-model", captured["model"])
	}
	if captured["encoding_format"] != "float" {
		t.Fatalf("encoding format = %v, want float", captured["encoding_format"])
	}
	inputs, ok := captured["input"].([]any)
	if !ok || len(inputs) != 2 || inputs[0] != "alpha" || inputs[1] != "beta" {
		t.Fatalf("input = %#v", captured["input"])
	}
	if result.Model != "embedding-model" {
		t.Fatalf("model = %q, want embedding-model", result.Model)
	}
	if len(result.Vectors) != 2 || result.Vectors[0][0] != 0.1 || result.Vectors[1][0] != 0.3 {
		t.Fatalf("vectors = %#v", result.Vectors)
	}
}

func TestClientEmbedHandlesNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "embedding unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(ClientConfig{BaseURL: server.URL, Model: "embedding-model"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Embed(context.Background(), providers.EmbeddingRequest{
		Texts: []string{"alpha"},
	})
	if err == nil {
		t.Fatal("err = nil, want non-success error")
	}
}

func TestClientEmbedRejectsMissingResponseIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1]}]}`))
	}))
	defer server.Close()

	client, err := New(ClientConfig{BaseURL: server.URL, Model: "embedding-model"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Embed(context.Background(), providers.EmbeddingRequest{
		Texts: []string{"alpha", "beta"},
	})
	if err == nil {
		t.Fatal("err = nil, want missing index error")
	}
}

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	_, err := New(ClientConfig{})
	if err == nil {
		t.Fatal("err = nil, want validation error")
	}
}
