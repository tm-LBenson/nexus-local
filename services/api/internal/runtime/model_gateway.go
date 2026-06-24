package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers/openaicompat"
)

type RoutedModelGateway struct {
	router  *providers.ModelRouter
	apiKey  string
	mu      sync.Mutex
	clients map[string]providers.ModelGateway
}

func NewRoutedModelGateway(router *providers.ModelRouter, cfg config.Config) *RoutedModelGateway {
	return &RoutedModelGateway{
		router:  router,
		apiKey:  cfg.ModelGatewayAPIKey,
		clients: map[string]providers.ModelGateway{},
	}
}

func (g *RoutedModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if g.router == nil {
		return providers.ChatCompletion{}, fmt.Errorf("model router is not configured")
	}
	route, err := g.router.Route(ctx, providers.ModelRequest{Target: input.Target})
	if err != nil {
		return providers.ChatCompletion{}, err
	}

	client, err := g.clientFor(route)
	if err != nil {
		return providers.ChatCompletion{}, err
	}
	if strings.TrimSpace(input.Model) == "" {
		input.Model = route.Model
	}
	return client.Complete(ctx, input)
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
