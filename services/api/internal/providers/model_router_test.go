package providers

import (
	"context"
	"errors"
	"testing"
)

func TestModelRouterUsesDefaultTarget(t *testing.T) {
	router := newTestRouter(t)

	route, err := router.Route(context.Background(), ModelRequest{})
	if err != nil {
		t.Fatalf("route: %v", err)
	}

	if route.Target != "general" {
		t.Fatalf("target = %q, want general", route.Target)
	}
	if route.BaseURL != "http://gpu.local:8000/v1" {
		t.Fatalf("base url = %q", route.BaseURL)
	}
}

func TestModelRouterRoutesExplicitTarget(t *testing.T) {
	router := newTestRouter(t)

	route, err := router.Route(context.Background(), ModelRequest{Target: " EMAIL-REVISION "})
	if err != nil {
		t.Fatalf("route: %v", err)
	}

	if route.Target != "email-revision" {
		t.Fatalf("target = %q, want email-revision", route.Target)
	}
	if route.Model != "email-model" {
		t.Fatalf("model = %q, want email-model", route.Model)
	}
}

func TestModelRouterRejectsUnknownTarget(t *testing.T) {
	router := newTestRouter(t)

	_, err := router.Route(context.Background(), ModelRequest{Target: "not-real"})
	if !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("err = %v, want ErrUnknownTarget", err)
	}
}

func TestNewModelRouterRejectsMissingDefault(t *testing.T) {
	_, err := NewModelRouter([]TargetConfig{
		{Name: "general", Provider: "openai-compatible", BaseURL: "http://gpu.local:8000/v1", Model: "test-model"},
	}, "missing")
	if !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("err = %v, want ErrUnknownTarget", err)
	}
}

func newTestRouter(t *testing.T) *ModelRouter {
	t.Helper()

	router, err := NewModelRouter([]TargetConfig{
		{Name: "general", Provider: "openai-compatible", BaseURL: "http://gpu.local:8000/v1", Model: "general-model"},
		{Name: "email-revision", Provider: "openai-compatible", BaseURL: "http://gpu.local:8000/v1", Model: "email-model"},
	}, "general")
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}
