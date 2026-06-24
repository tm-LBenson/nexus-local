package minioobject

import "testing"

func TestNormalizeEndpointWithHTTPURL(t *testing.T) {
	endpoint, secure, err := normalizeEndpoint("http://minio:9000", true)
	if err != nil {
		t.Fatalf("normalize endpoint: %v", err)
	}
	if endpoint != "minio:9000" {
		t.Fatalf("endpoint = %q, want minio:9000", endpoint)
	}
	if secure {
		t.Fatal("secure = true, want false")
	}
}

func TestNormalizeEndpointWithHTTPSURL(t *testing.T) {
	endpoint, secure, err := normalizeEndpoint("https://objects.example.com", false)
	if err != nil {
		t.Fatalf("normalize endpoint: %v", err)
	}
	if endpoint != "objects.example.com" {
		t.Fatalf("endpoint = %q, want objects.example.com", endpoint)
	}
	if !secure {
		t.Fatal("secure = false, want true")
	}
}

func TestNormalizeEndpointWithoutScheme(t *testing.T) {
	endpoint, secure, err := normalizeEndpoint("minio:9000", true)
	if err != nil {
		t.Fatalf("normalize endpoint: %v", err)
	}
	if endpoint != "minio:9000" {
		t.Fatalf("endpoint = %q, want minio:9000", endpoint)
	}
	if !secure {
		t.Fatal("secure = false, want true")
	}
}
