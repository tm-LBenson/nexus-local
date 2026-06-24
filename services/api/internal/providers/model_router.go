package providers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrUnknownTarget = errors.New("unknown model target")
	ErrEmptyTarget   = errors.New("model target name is required")
)

type TargetConfig struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

type ModelRequest struct {
	Target   string            `json:"target"`
	TenantID string            `json:"tenant_id,omitempty"`
	Input    string            `json:"input,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ModelRoute struct {
	Target   string `json:"target"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

type ModelRouter struct {
	targets       map[string]TargetConfig
	defaultTarget string
}

func NewModelRouter(targets []TargetConfig, defaultTarget string) (*ModelRouter, error) {
	byName := make(map[string]TargetConfig, len(targets))
	for _, target := range targets {
		name := normalizeTarget(target.Name)
		if name == "" {
			return nil, ErrEmptyTarget
		}
		target.Name = name
		byName[name] = target
	}

	defaultTarget = normalizeTarget(defaultTarget)
	if defaultTarget == "" {
		return nil, fmt.Errorf("default target: %w", ErrEmptyTarget)
	}
	if _, ok := byName[defaultTarget]; !ok {
		return nil, fmt.Errorf("default target %q: %w", defaultTarget, ErrUnknownTarget)
	}

	return &ModelRouter{
		targets:       byName,
		defaultTarget: defaultTarget,
	}, nil
}

func (r *ModelRouter) Route(ctx context.Context, req ModelRequest) (ModelRoute, error) {
	if err := ctx.Err(); err != nil {
		return ModelRoute{}, err
	}

	name := normalizeTarget(req.Target)
	if name == "" {
		name = r.defaultTarget
	}

	target, ok := r.targets[name]
	if !ok {
		return ModelRoute{}, fmt.Errorf("%q: %w", name, ErrUnknownTarget)
	}

	return ModelRoute{
		Target:   target.Name,
		Provider: target.Provider,
		BaseURL:  target.BaseURL,
		Model:    target.Model,
	}, nil
}

func (r *ModelRouter) Targets() []TargetConfig {
	targets := make([]TargetConfig, 0, len(r.targets))
	for _, target := range r.targets {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})
	return targets
}

func normalizeTarget(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
