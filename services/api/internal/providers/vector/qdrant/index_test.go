package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

func TestIndexCreatesCollectionAndUpsertsPoints(t *testing.T) {
	var requests []string
	var upsertBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		switch r.Method + " " + r.URL.Path {
		case "GET /collections/documents":
			http.NotFound(w, r)
		case "PUT /collections/documents":
			_, _ = w.Write([]byte(`{"status":"ok","result":true}`))
		case "PUT /collections/documents/points":
			if err := json.NewDecoder(r.Body).Decode(&upsertBody); err != nil {
				t.Fatalf("decode upsert: %v", err)
			}
			_, _ = w.Write([]byte(`{"status":"ok","result":{"status":"acknowledged"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	index, err := New(context.Background(), Config{
		BaseURL:    server.URL,
		Collection: "documents",
		Dimensions: 2,
		APIKey:     "secret",
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	err = index.Upsert(context.Background(), []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_1"),
			ChunkID:    "chunk_0000",
			Values:     []float32{1, 0},
			Text:       "alpha",
			Metadata:   map[string]string{"document_name": "Handbook.md"},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %v", requests)
	}
	points, ok := upsertBody["points"].([]any)
	if !ok || len(points) != 1 {
		t.Fatalf("points = %#v", upsertBody["points"])
	}
}

func TestIndexSearchUsesQueryEndpoint(t *testing.T) {
	var searchBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /collections/documents":
			_, _ = w.Write([]byte(`{"status":"ok","result":{}}`))
		case "POST /collections/documents/points/query":
			if err := json.NewDecoder(r.Body).Decode(&searchBody); err != nil {
				t.Fatalf("decode search: %v", err)
			}
			_, _ = w.Write([]byte(`{
				"status":"ok",
				"result":{
					"points":[
						{
							"score":0.9,
							"payload":{
								"document_id":"doc_1",
								"chunk_id":"chunk_0000",
								"text":"alpha",
								"metadata":{"document_name":"Handbook.md"}
							}
						}
					]
				}
			}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	index, err := New(context.Background(), Config{
		BaseURL:    server.URL,
		Collection: "documents",
		Dimensions: 2,
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	hits, err := index.Search(context.Background(), providers.VectorSearch{
		TenantID:   domain.TenantID("tenant_1"),
		DocumentID: domain.DocumentID("doc_1"),
		Query:      []float32{1, 0},
		Limit:      3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if searchBody["limit"].(float64) != 3 {
		t.Fatalf("limit = %v", searchBody["limit"])
	}
	filter := searchBody["filter"].(map[string]any)
	must := filter["must"].([]any)
	if len(must) != 2 {
		t.Fatalf("filter must = %#v, want tenant and document filters", must)
	}
}

func TestIndexDeleteDocumentUsesFilterDelete(t *testing.T) {
	var deleteBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /collections/documents":
			_, _ = w.Write([]byte(`{"status":"ok","result":{}}`))
		case "POST /collections/documents/points/delete":
			if err := json.NewDecoder(r.Body).Decode(&deleteBody); err != nil {
				t.Fatalf("decode delete: %v", err)
			}
			_, _ = w.Write([]byte(`{"status":"ok","result":{"status":"acknowledged"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	index, err := New(context.Background(), Config{
		BaseURL:    server.URL,
		Collection: "documents",
		Dimensions: 2,
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	if err := index.DeleteDocument(context.Background(), "tenant_1", "doc_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleteBody["filter"] == nil {
		t.Fatal("delete body missing filter")
	}
}
