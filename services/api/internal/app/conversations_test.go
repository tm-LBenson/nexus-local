package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestAskCreatesConversationRetrievesContextAndStoresMessages(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	vectorIndex := vectormemory.New()
	search := NewSearchService(embedder, vectorIndex)
	gateway := &stubModelGateway{
		response: providers.ChatCompletion{
			Model:        "general-model",
			Content:      "Use the launch plan.",
			FinishReason: "stop",
		},
	}

	seed, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed seed: %v", err)
	}
	if err := vectorIndex.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_1"),
			ChunkID:    "chunk_1",
			Values:     seed.Vectors[0],
			Text:       "alpha beta launch plan",
		},
	}); err != nil {
		t.Fatalf("upsert seed: %v", err)
	}

	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)
	result, err := service.Ask(ctx, AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Question: "What is the alpha beta plan?",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	if result.Conversation.ID != domain.ConversationID("conv_fixed") {
		t.Fatalf("conversation id = %q, want conv_fixed", result.Conversation.ID)
	}
	if result.UserMessage.Role != domain.MessageRoleUser || result.AssistantMessage.Role != domain.MessageRoleAssistant {
		t.Fatalf("roles = %s/%s, want user/assistant", result.UserMessage.Role, result.AssistantMessage.Role)
	}
	messages, err := repos.ListMessages(ctx, domain.TenantID("tenant_1"), result.Conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if gateway.request.Target != "general" {
		t.Fatalf("target = %q, want general", gateway.request.Target)
	}
	lastPrompt := gateway.request.Messages[len(gateway.request.Messages)-1].Content
	if !strings.Contains(lastPrompt, "document=doc_1") || !strings.Contains(lastPrompt, "alpha beta launch plan") {
		t.Fatalf("prompt missing retrieved context: %q", lastPrompt)
	}
}

func TestAskStreamEmitsDeltasAndStoresFinalMessage(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	search := NewSearchService(embedder, vectormemory.New())
	gateway := &streamingStubModelGateway{
		chunks: []string{"Use ", "the launch plan."},
		response: providers.ChatCompletion{
			Model:        "general-model",
			Content:      "Use the launch plan.",
			FinishReason: "stop",
		},
	}

	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)
	var events []AskStreamEvent
	result, err := service.AskStream(ctx, AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Question: "What is the launch plan?",
		Limit:    1,
	}, func(event AskStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ask stream: %v", err)
	}

	var statuses []string
	var deltas []string
	for _, event := range events {
		switch event.Type {
		case AskStreamStatus:
			statuses = append(statuses, event.Message)
		case AskStreamDelta:
			deltas = append(deltas, event.Delta)
		}
	}
	if strings.Join(statuses, ",") != "Retrieving,Generating" {
		t.Fatalf("statuses = %q, want retrieving/generating", statuses)
	}
	if strings.Join(deltas, "") != "Use the launch plan." {
		t.Fatalf("deltas = %q", deltas)
	}
	if result.AssistantMessage.Content != "Use the launch plan." {
		t.Fatalf("assistant content = %q", result.AssistantMessage.Content)
	}
	messages, err := repos.ListMessages(ctx, domain.TenantID("tenant_1"), result.Conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if gateway.request.Target != "general" {
		t.Fatalf("target = %q, want general", gateway.request.Target)
	}
}

func TestAskUsesExistingConversationTarget(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	search := NewSearchService(embedder, vectormemory.New())
	gateway := &stubModelGateway{response: providers.ChatCompletion{Content: "done"}}

	conversation, err := domain.NewConversation(domain.ConversationCreate{
		ID:          domain.ConversationID("conv_existing"),
		TenantID:    domain.TenantID("tenant_1"),
		OwnerID:     domain.UserID("user_1"),
		ModelTarget: "email-revision",
		Now:         fixedClock{}.Now(),
	})
	if err != nil {
		t.Fatalf("new conversation: %v", err)
	}
	if err := repos.SaveConversation(ctx, conversation); err != nil {
		t.Fatalf("save conversation: %v", err)
	}

	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)
	_, err = service.Ask(ctx, AskInput{
		TenantID:       domain.TenantID("tenant_1"),
		OwnerID:        domain.UserID("user_1"),
		ConversationID: domain.ConversationID("conv_existing"),
		ModelTarget:    "general",
		Question:       "Revise this",
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if gateway.request.Target != "email-revision" {
		t.Fatalf("target = %q, want email-revision", gateway.request.Target)
	}
}

func TestListConversationsAndMessages(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	search := NewSearchService(embedder, vectormemory.New())
	gateway := &stubModelGateway{response: providers.ChatCompletion{Content: "done"}}
	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)

	askResult, err := service.Ask(ctx, AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Question: "What is saved?",
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	conversations, err := service.ListConversations(ctx, ListConversationsInput{
		TenantID: domain.TenantID("tenant_1"),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations.Conversations) != 1 {
		t.Fatalf("conversations len = %d, want 1", len(conversations.Conversations))
	}
	if conversations.Conversations[0].ID != askResult.Conversation.ID {
		t.Fatalf("conversation id = %q, want %q", conversations.Conversations[0].ID, askResult.Conversation.ID)
	}

	messages, err := service.ListMessages(ctx, ListMessagesInput{
		TenantID:       domain.TenantID("tenant_1"),
		ConversationID: askResult.Conversation.ID,
	})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages.Messages))
	}
	if messages.Messages[0].Role != domain.MessageRoleUser || messages.Messages[1].Role != domain.MessageRoleAssistant {
		t.Fatalf("roles = %s/%s, want user/assistant", messages.Messages[0].Role, messages.Messages[1].Role)
	}
}

func TestListConversationMessagesRequiresExistingConversation(t *testing.T) {
	service := NewConversationService(memory.New(), &askIDs{}, fixedClock{}, SearchService{}, &stubModelGateway{})

	_, err := service.ListMessages(context.Background(), ListMessagesInput{
		TenantID:       domain.TenantID("tenant_1"),
		ConversationID: domain.ConversationID("missing"),
	})
	if err == nil {
		t.Fatal("err = nil, want not found")
	}
}

func TestNormalizeConversationLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default", limit: 0, want: defaultConversationListLimit},
		{name: "negative", limit: -1, want: defaultConversationListLimit},
		{name: "custom", limit: 3, want: 3},
		{name: "max", limit: maxConversationListLimit + 1, want: maxConversationListLimit},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeConversationLimit(tc.limit); got != tc.want {
				t.Fatalf("limit = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAskRejectsInvalidInput(t *testing.T) {
	service := NewConversationService(memory.New(), &askIDs{}, fixedClock{}, SearchService{}, &stubModelGateway{})

	_, err := service.Ask(context.Background(), AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
	})
	if !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}

type stubModelGateway struct {
	request  providers.ChatCompletionRequest
	response providers.ChatCompletion
}

func (g *stubModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	g.request = input
	return g.response, nil
}

type streamingStubModelGateway struct {
	request  providers.ChatCompletionRequest
	chunks   []string
	response providers.ChatCompletion
}

func (g *streamingStubModelGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	g.request = input
	return g.response, nil
}

func (g *streamingStubModelGateway) StreamComplete(ctx context.Context, input providers.ChatCompletionRequest, emit func(providers.ChatCompletionChunk) error) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	g.request = input
	for _, chunk := range g.chunks {
		if err := emit(providers.ChatCompletionChunk{
			Model:   g.response.Model,
			Content: chunk,
		}); err != nil {
			return providers.ChatCompletion{}, err
		}
	}
	return g.response, nil
}

type askIDs struct {
	message int
}

func (g *askIDs) NewConversationID() domain.ConversationID {
	return domain.ConversationID("conv_fixed")
}

func (g *askIDs) NewMessageID() domain.MessageID {
	g.message++
	if g.message == 1 {
		return domain.MessageID("msg_user")
	}
	return domain.MessageID("msg_assistant")
}
