package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
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
			Metadata:   map[string]string{"document_name": "Alpha Plan.md", "chunk_index": "0"},
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
	if !strings.Contains(lastPrompt, "source=Alpha Plan.md") || !strings.Contains(lastPrompt, "alpha beta launch plan") {
		t.Fatalf("prompt missing retrieved context: %q", lastPrompt)
	}
}

func TestAskCanScopeRetrievalToDocument(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	vectorIndex := vectormemory.New()
	search := NewSearchService(embedder, vectorIndex)
	gateway := &stubModelGateway{response: providers.ChatCompletion{Content: "Use the scoped notes."}}

	alphaVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"alpha beta launch plan"}})
	if err != nil {
		t.Fatalf("embed alpha text: %v", err)
	}
	omegaVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"omega archive notes"}})
	if err != nil {
		t.Fatalf("embed omega text: %v", err)
	}
	if err := vectorIndex.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_alpha"),
			ChunkID:    "chunk_1",
			Values:     alphaVector.Vectors[0],
			Text:       "alpha beta launch plan",
			Metadata:   map[string]string{"document_name": "Alpha Plan.md"},
		},
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_omega"),
			ChunkID:    "chunk_2",
			Values:     omegaVector.Vectors[0],
			Text:       "omega archive notes",
			Metadata:   map[string]string{"document_name": "Omega Notes.md"},
		},
	}); err != nil {
		t.Fatalf("upsert seed: %v", err)
	}

	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)
	result, err := service.Ask(ctx, AskInput{
		TenantID:   domain.TenantID("tenant_1"),
		OwnerID:    domain.UserID("user_1"),
		DocumentID: domain.DocumentID("doc_omega"),
		Question:   "What is the alpha beta plan?",
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(result.Hits))
	}
	if result.Hits[0].DocumentID != domain.DocumentID("doc_omega") {
		t.Fatalf("hit document = %q, want doc_omega", result.Hits[0].DocumentID)
	}
	lastPrompt := gateway.request.Messages[len(gateway.request.Messages)-1].Content
	if !strings.Contains(lastPrompt, "Omega Notes.md") || strings.Contains(lastPrompt, "Alpha Plan.md") {
		t.Fatalf("prompt did not use scoped context: %q", lastPrompt)
	}
}

func TestAskRewritesVagueFollowUpForRetrievalOnly(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 64)
	vectorIndex := vectormemory.New()
	search := NewSearchService(embedder, vectorIndex)
	gateway := &stubModelGateway{response: providers.ChatCompletion{Content: "Use the retrieved source."}}

	oidcVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"OIDC invalid redirect uri callback mismatch exact match scheme host port path trailing slash"}})
	if err != nil {
		t.Fatalf("embed oidc text: %v", err)
	}
	passwordVector, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{"Fix password reset status by verifying MFA enrollment and account recovery state"}})
	if err != nil {
		t.Fatalf("embed password text: %v", err)
	}
	if err := vectorIndex.Upsert(ctx, []providers.Vector{
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_oidc"),
			ChunkID:    "redirect_uri",
			Values:     oidcVector.Vectors[0],
			Text:       "OIDC invalid redirect uri callback mismatch exact match scheme host port path trailing slash",
			Metadata:   map[string]string{"document_name": "OIDC Troubleshooting.md"},
		},
		{
			TenantID:   domain.TenantID("tenant_1"),
			DocumentID: domain.DocumentID("doc_password"),
			ChunkID:    "password_status",
			Values:     passwordVector.Vectors[0],
			Text:       "Fix password reset status by verifying MFA enrollment and account recovery state",
			Metadata:   map[string]string{"document_name": "Password Status.md"},
		},
	}); err != nil {
		t.Fatalf("upsert seed: %v", err)
	}

	service := NewConversationService(repos, &sequenceAskIDs{}, &sequenceClock{}, search, gateway)
	first, err := service.Ask(ctx, AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Question: "Why does OIDC fail with invalid redirect uri callback mismatch?",
		Limit:    1,
		Strategy: SearchStrategyHybrid,
	})
	if err != nil {
		t.Fatalf("first ask: %v", err)
	}

	second, err := service.Ask(ctx, AskInput{
		TenantID:       domain.TenantID("tenant_1"),
		OwnerID:        domain.UserID("user_1"),
		ConversationID: first.Conversation.ID,
		Question:       "How do I fix it?",
		Limit:          1,
		Strategy:       SearchStrategyHybrid,
	})
	if err != nil {
		t.Fatalf("follow-up ask: %v", err)
	}

	if len(second.Hits) != 1 || second.Hits[0].DocumentID != domain.DocumentID("doc_oidc") {
		t.Fatalf("hits = %#v, want OIDC source from rewritten follow-up", second.Hits)
	}
	retrievalQuery := gateway.request.Metadata["retrieval_query"]
	if !strings.Contains(retrievalQuery, "OIDC fail with invalid redirect uri") || !strings.Contains(retrievalQuery, "How do I fix it?") {
		t.Fatalf("retrieval query = %q, want previous topic plus current question", retrievalQuery)
	}
	lastPrompt := gateway.request.Messages[len(gateway.request.Messages)-1].Content
	if !strings.Contains(lastPrompt, "Question:\nHow do I fix it?") || !strings.Contains(lastPrompt, "OIDC Troubleshooting.md") {
		t.Fatalf("prompt = %q, want original question with OIDC context", lastPrompt)
	}

	messages, err := repos.ListMessages(ctx, domain.TenantID("tenant_1"), first.Conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 4 || messages[2].Content != "How do I fix it?" {
		t.Fatalf("messages = %#v, want original follow-up stored as user message", messages)
	}
}

func TestPromptMessagesRequireGroundingCitationsAndMissingContext(t *testing.T) {
	messages := promptMessages(nil, "What is the refund approval policy?", nil)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}

	system := strings.ToLower(messages[0].Content)
	requiredSystemPhrases := []string{
		"enterprise knowledge-base assistant",
		"retrieved context",
		"bracketed citations",
		"does not have enough context",
		"do not invent",
		"pii",
		"general guidance",
	}
	for _, phrase := range requiredSystemPhrases {
		if !strings.Contains(system, phrase) {
			t.Fatalf("system prompt missing %q: %q", phrase, messages[0].Content)
		}
	}

	userPrompt := messages[1].Content
	requiredUserPhrases := []string{
		"Question:\nWhat is the refund approval policy?",
		"Retrieved context:",
		"No relevant document chunks were retrieved.",
		"knowledge base does not contain enough context",
	}
	for _, phrase := range requiredUserPhrases {
		if !strings.Contains(userPrompt, phrase) {
			t.Fatalf("user prompt missing %q: %q", phrase, userPrompt)
		}
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

func TestDeleteConversationRemovesHistory(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	embedder := embeddinghash.New("test", 16)
	search := NewSearchService(embedder, vectormemory.New())
	gateway := &stubModelGateway{response: providers.ChatCompletion{Content: "done"}}
	service := NewConversationService(repos, &askIDs{}, fixedClock{}, search, gateway)

	askResult, err := service.Ask(ctx, AskInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Question: "What should be deleted?",
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	deleted, err := service.DeleteConversation(ctx, DeleteConversationInput{
		TenantID:       domain.TenantID("tenant_1"),
		ConversationID: askResult.Conversation.ID,
	})
	if err != nil {
		t.Fatalf("delete conversation: %v", err)
	}
	if deleted.Conversation.ID != askResult.Conversation.ID {
		t.Fatalf("deleted id = %q, want %q", deleted.Conversation.ID, askResult.Conversation.ID)
	}

	conversations, err := service.ListConversations(ctx, ListConversationsInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations.Conversations) != 0 {
		t.Fatalf("conversations len = %d, want 0", len(conversations.Conversations))
	}
	_, err = service.ListMessages(ctx, ListMessagesInput{
		TenantID:       domain.TenantID("tenant_1"),
		ConversationID: askResult.Conversation.ID,
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
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

type sequenceAskIDs struct {
	conversation int
	message      int
}

func (g *sequenceAskIDs) NewConversationID() domain.ConversationID {
	g.conversation++
	return domain.ConversationID(fmt.Sprintf("conv_sequence_%d", g.conversation))
}

func (g *sequenceAskIDs) NewMessageID() domain.MessageID {
	g.message++
	return domain.MessageID(fmt.Sprintf("msg_sequence_%d", g.message))
}

type sequenceClock struct {
	tick int
}

func (c *sequenceClock) Now() time.Time {
	c.tick++
	return time.Date(2026, 6, 23, 12, 0, c.tick, 0, time.UTC)
}
