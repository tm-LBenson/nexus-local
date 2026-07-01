package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers/openaicompat"
)

type RoutedModelGateway struct {
	router  *providers.ModelRouter
	apiKey  string
	timeout time.Duration
	mu      sync.Mutex
	clients map[string]providers.ModelGateway
}

func NewRoutedModelGateway(router *providers.ModelRouter, cfg config.Config) *RoutedModelGateway {
	return &RoutedModelGateway{
		router:  router,
		apiKey:  cfg.ModelGatewayAPIKey,
		timeout: cfg.ModelGatewayTimeout,
		clients: map[string]providers.ModelGateway{},
	}
}

func (g *RoutedModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	client, input, err := g.routedClient(ctx, input)
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	return client.Complete(ctx, input)
}

func (g *RoutedModelGateway) StreamComplete(ctx context.Context, input providers.ChatCompletionRequest, emit func(providers.ChatCompletionChunk) error) (providers.ChatCompletion, error) {
	client, input, err := g.routedClient(ctx, input)
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	streamer, ok := client.(providers.StreamingModelGateway)
	if ok {
		return streamer.StreamComplete(ctx, input, emit)
	}

	completion, err := client.Complete(ctx, input)
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	if strings.TrimSpace(completion.Content) != "" && emit != nil {
		if err := emit(providers.ChatCompletionChunk{
			Model:        completion.Model,
			Content:      completion.Content,
			FinishReason: completion.FinishReason,
			Usage:        completion.Usage,
			Metadata:     completion.Metadata,
		}); err != nil {
			return providers.ChatCompletion{}, err
		}
	}
	return completion, nil
}

func (g *RoutedModelGateway) routedClient(ctx context.Context, input providers.ChatCompletionRequest) (providers.ModelGateway, providers.ChatCompletionRequest, error) {
	if g.router == nil {
		return nil, providers.ChatCompletionRequest{}, fmt.Errorf("model router is not configured")
	}
	route, err := g.router.Route(ctx, providers.ModelRequest{Target: input.Target})
	if err != nil {
		return nil, providers.ChatCompletionRequest{}, err
	}

	client, err := g.clientFor(route)
	if err != nil {
		return nil, providers.ChatCompletionRequest{}, err
	}
	if strings.TrimSpace(input.Model) == "" {
		input.Model = route.Model
	}
	return client, input, nil
}

func (g *RoutedModelGateway) clientFor(route providers.ModelRoute) (providers.ModelGateway, error) {
	provider := strings.ToLower(strings.TrimSpace(route.Provider))
	key := provider + "|" + strings.TrimSpace(route.BaseURL)

	g.mu.Lock()
	defer g.mu.Unlock()
	if client, ok := g.clients[key]; ok {
		return client, nil
	}

	var client providers.ModelGateway
	switch provider {
	case "openai", "openai-compatible", "openaicompat":
		created, err := openaicompat.New(openaicompat.ClientConfig{
			BaseURL: route.BaseURL,
			APIKey:  g.apiKey,
			Timeout: g.timeout,
		})
		if err != nil {
			return nil, err
		}
		client = created
	default:
		return nil, fmt.Errorf("unsupported model provider %q", route.Provider)
	}
	g.clients[key] = client
	return client, nil
}
