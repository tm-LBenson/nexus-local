package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

const (
	defaultModelTarget           = "general"
	defaultConversationListLimit = 25
	maxConversationListLimit     = 100
	maxHistoryMessages           = 12
)

var ErrConversationModelUnavailable = errors.New("conversation model gateway is not configured")

type ConversationIDs interface {
	NewConversationID() domain.ConversationID
	NewMessageID() domain.MessageID
}

type ConversationService struct {
	repos  store.RepositorySet
	ids    ConversationIDs
	clock  Clock
	search SearchService
	models providers.ModelGateway
}

type AskInput struct {
	TenantID       domain.TenantID
	OwnerID        domain.UserID
	ConversationID domain.ConversationID
	DocumentID     domain.DocumentID
	ModelTarget    string
	Question       string
	Limit          int
}

type AskResult struct {
	Conversation     domain.Conversation
	UserMessage      domain.Message
	AssistantMessage domain.Message
	Hits             []providers.VectorHit
	Completion       providers.ChatCompletion
}

type AskStreamEventType string

const (
	AskStreamStatus AskStreamEventType = "status"
	AskStreamDelta  AskStreamEventType = "delta"
)

type AskStreamEvent struct {
	Type    AskStreamEventType
	Message string
	Delta   string
}

type askPreparation struct {
	Conversation    domain.Conversation
	UserMessage     domain.Message
	Hits            []providers.VectorHit
	CompletionInput providers.ChatCompletionRequest
}

type ListConversationsInput struct {
	TenantID domain.TenantID
	Limit    int
}

type ListConversationsResult struct {
	Conversations []domain.Conversation
}

type ListMessagesInput struct {
	TenantID       domain.TenantID
	ConversationID domain.ConversationID
}

type ListMessagesResult struct {
	Messages []domain.Message
}

type DeleteConversationInput struct {
	TenantID       domain.TenantID
	ConversationID domain.ConversationID
}

type DeleteConversationResult struct {
	Conversation domain.Conversation
}

func NewConversationService(repos store.RepositorySet, ids ConversationIDs, clock Clock, search SearchService, models providers.ModelGateway) ConversationService {
	return ConversationService{
		repos:  repos,
		ids:    ids,
		clock:  clock,
		search: search,
		models: models,
	}
}

func (s ConversationService) ListConversations(ctx context.Context, input ListConversationsInput) (ListConversationsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListConversationsResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" {
		return ListConversationsResult{}, fmt.Errorf("conversations: %w", domain.ErrInvalidEntity)
	}
	conversations, err := s.repos.ListConversations(ctx, input.TenantID)
	if err != nil {
		return ListConversationsResult{}, err
	}
	limit := normalizeConversationLimit(input.Limit)
	if len(conversations) > limit {
		conversations = conversations[:limit]
	}
	return ListConversationsResult{Conversations: conversations}, nil
}

func (s ConversationService) ListMessages(ctx context.Context, input ListMessagesInput) (ListMessagesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListMessagesResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.ConversationID)) == "" {
		return ListMessagesResult{}, fmt.Errorf("messages: %w", domain.ErrInvalidEntity)
	}
	if _, err := s.repos.GetConversation(ctx, input.TenantID, input.ConversationID); err != nil {
		return ListMessagesResult{}, err
	}
	messages, err := s.repos.ListMessages(ctx, input.TenantID, input.ConversationID)
	if err != nil {
		return ListMessagesResult{}, err
	}
	return ListMessagesResult{Messages: messages}, nil
}

func (s ConversationService) DeleteConversation(ctx context.Context, input DeleteConversationInput) (DeleteConversationResult, error) {
	if err := ctx.Err(); err != nil {
		return DeleteConversationResult{}, err
	}
	if strings.TrimSpace(string(input.TenantID)) == "" || strings.TrimSpace(string(input.ConversationID)) == "" {
		return DeleteConversationResult{}, fmt.Errorf("delete conversation: %w", domain.ErrInvalidEntity)
	}
	conversation, err := s.repos.GetConversation(ctx, input.TenantID, input.ConversationID)
	if err != nil {
		return DeleteConversationResult{}, err
	}
	if err := s.repos.DeleteConversation(ctx, input.TenantID, input.ConversationID); err != nil {
		return DeleteConversationResult{}, err
	}
	return DeleteConversationResult{Conversation: conversation}, nil
}

func (s ConversationService) Ask(ctx context.Context, input AskInput) (AskResult, error) {
	prepared, err := s.prepareAsk(ctx, input, nil)
	if err != nil {
		return AskResult{}, err
	}

	completion, err := s.models.Complete(ctx, prepared.CompletionInput)
	if err != nil {
		return AskResult{}, err
	}

	return s.finishAsk(ctx, prepared, completion)
}

func (s ConversationService) AskStream(ctx context.Context, input AskInput, emit func(AskStreamEvent) error) (AskResult, error) {
	if emit == nil {
		emit = func(AskStreamEvent) error { return nil }
	}

	prepared, err := s.prepareAsk(ctx, input, emit)
	if err != nil {
		return AskResult{}, err
	}
	if err := emit(AskStreamEvent{Type: AskStreamStatus, Message: "Generating"}); err != nil {
		return AskResult{}, err
	}

	streamer, ok := s.models.(providers.StreamingModelGateway)
	if !ok {
		completion, err := s.models.Complete(ctx, prepared.CompletionInput)
		if err != nil {
			return AskResult{}, err
		}
		if strings.TrimSpace(completion.Content) != "" {
			if err := emit(AskStreamEvent{Type: AskStreamDelta, Delta: completion.Content}); err != nil {
				return AskResult{}, err
			}
		}
		return s.finishAsk(ctx, prepared, completion)
	}

	sawDelta := false
	completion, err := streamer.StreamComplete(ctx, prepared.CompletionInput, func(chunk providers.ChatCompletionChunk) error {
		if chunk.Content == "" {
			return nil
		}
		sawDelta = true
		return emit(AskStreamEvent{Type: AskStreamDelta, Delta: chunk.Content})
	})
	if err != nil {
		return AskResult{}, err
	}
	if !sawDelta && strings.TrimSpace(completion.Content) != "" {
		if err := emit(AskStreamEvent{Type: AskStreamDelta, Delta: completion.Content}); err != nil {
			return AskResult{}, err
		}
	}

	return s.finishAsk(ctx, prepared, completion)
}

func (s ConversationService) prepareAsk(ctx context.Context, input AskInput, emit func(AskStreamEvent) error) (askPreparation, error) {
	if err := ctx.Err(); err != nil {
		return askPreparation{}, err
	}
	if s.models == nil {
		return askPreparation{}, ErrConversationModelUnavailable
	}

	question := strings.TrimSpace(input.Question)
	if strings.TrimSpace(string(input.TenantID)) == "" ||
		strings.TrimSpace(string(input.OwnerID)) == "" ||
		question == "" {
		return askPreparation{}, fmt.Errorf("ask: %w", domain.ErrInvalidEntity)
	}

	now := s.clock.Now()
	conversation, err := s.openConversation(ctx, input, question, now)
	if err != nil {
		return askPreparation{}, err
	}

	if emit != nil {
		if err := emit(AskStreamEvent{Type: AskStreamStatus, Message: "Retrieving"}); err != nil {
			return askPreparation{}, err
		}
	}

	searchResult, err := s.search.Search(ctx, SearchInput{
		TenantID:   input.TenantID,
		DocumentID: input.DocumentID,
		Query:      question,
		Limit:      input.Limit,
	})
	if err != nil {
		return askPreparation{}, err
	}

	history, err := s.repos.ListMessages(ctx, conversation.TenantID, conversation.ID)
	if err != nil {
		return askPreparation{}, err
	}

	userMessage, err := domain.NewMessage(domain.MessageCreate{
		ID:             s.ids.NewMessageID(),
		TenantID:       conversation.TenantID,
		ConversationID: conversation.ID,
		Role:           domain.MessageRoleUser,
		Content:        question,
		Now:            now,
	})
	if err != nil {
		return askPreparation{}, err
	}
	if err := s.repos.SaveMessage(ctx, userMessage); err != nil {
		return askPreparation{}, err
	}

	return askPreparation{
		Conversation: conversation,
		UserMessage:  userMessage,
		Hits:         searchResult.Hits,
		CompletionInput: providers.ChatCompletionRequest{
			Target:      conversation.ModelTarget,
			Model:       "",
			Messages:    promptMessages(history, question, searchResult.Hits),
			Temperature: 0.2,
			Metadata: map[string]string{
				"tenant_id":       string(conversation.TenantID),
				"conversation_id": string(conversation.ID),
			},
		},
	}, nil
}

func (s ConversationService) finishAsk(ctx context.Context, prepared askPreparation, completion providers.ChatCompletion) (AskResult, error) {
	if err := ctx.Err(); err != nil {
		return AskResult{}, err
	}

	assistantNow := s.clock.Now()
	if !assistantNow.After(prepared.UserMessage.CreatedAt) {
		assistantNow = prepared.UserMessage.CreatedAt.Add(time.Nanosecond)
	}
	assistantMessage, err := domain.NewMessage(domain.MessageCreate{
		ID:             s.ids.NewMessageID(),
		TenantID:       prepared.Conversation.TenantID,
		ConversationID: prepared.Conversation.ID,
		Role:           domain.MessageRoleAssistant,
		Content:        completion.Content,
		Now:            assistantNow,
	})
	if err != nil {
		return AskResult{}, err
	}
	if err := s.repos.SaveMessage(ctx, assistantMessage); err != nil {
		return AskResult{}, err
	}

	conversation := prepared.Conversation
	conversation.UpdatedAt = assistantMessage.CreatedAt
	if err := s.repos.SaveConversation(ctx, conversation); err != nil {
		return AskResult{}, err
	}

	return AskResult{
		Conversation:     conversation,
		UserMessage:      prepared.UserMessage,
		AssistantMessage: assistantMessage,
		Hits:             prepared.Hits,
		Completion:       completion,
	}, nil
}

func (s ConversationService) openConversation(ctx context.Context, input AskInput, question string, now time.Time) (domain.Conversation, error) {
	if input.ConversationID != "" {
		conversation, err := s.repos.GetConversation(ctx, input.TenantID, input.ConversationID)
		if err != nil {
			return domain.Conversation{}, err
		}
		return conversation, nil
	}

	modelTarget := strings.TrimSpace(input.ModelTarget)
	if modelTarget == "" {
		modelTarget = defaultModelTarget
	}
	conversation, err := domain.NewConversation(domain.ConversationCreate{
		ID:          s.ids.NewConversationID(),
		TenantID:    input.TenantID,
		OwnerID:     input.OwnerID,
		Title:       titleFromQuestion(question),
		ModelTarget: modelTarget,
		Now:         now,
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	if err := s.repos.SaveConversation(ctx, conversation); err != nil {
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func promptMessages(history []domain.Message, question string, hits []providers.VectorHit) []providers.ChatMessage {
	messages := []providers.ChatMessage{
		{
			Role:    "system",
			Content: "You answer from the retrieved context when it is relevant. If the context does not contain the answer, say what is missing and answer cautiously from general knowledge only when useful.",
		},
	}

	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	for _, message := range history {
		if message.Role != domain.MessageRoleUser && message.Role != domain.MessageRoleAssistant {
			continue
		}
		messages = append(messages, providers.ChatMessage{
			Role:    string(message.Role),
			Content: message.Content,
		})
	}

	messages = append(messages, providers.ChatMessage{
		Role:    "user",
		Content: questionWithContext(question, hits),
	})
	return messages
}

func questionWithContext(question string, hits []providers.VectorHit) string {
	var builder strings.Builder
	builder.WriteString("Question:\n")
	builder.WriteString(question)
	builder.WriteString("\n\nRetrieved context:\n")
	if len(hits) == 0 {
		builder.WriteString("No relevant document chunks were retrieved.")
		return builder.String()
	}
	for index, hit := range hits {
		builder.WriteString(fmt.Sprintf("[%d] source=%s document_id=%s chunk=%s score=%.3f\n%s\n\n", index+1, sourceName(hit), hit.DocumentID, sourceChunk(hit), hit.Score, hit.Text))
	}
	return strings.TrimSpace(builder.String())
}

func sourceName(hit providers.VectorHit) string {
	name := strings.TrimSpace(hit.Metadata["document_name"])
	if name == "" {
		return string(hit.DocumentID)
	}
	return name
}

func sourceChunk(hit providers.VectorHit) string {
	index := strings.TrimSpace(hit.Metadata["chunk_index"])
	if index == "" {
		return hit.ChunkID
	}
	return index
}

func titleFromQuestion(question string) string {
	words := strings.Fields(question)
	title := strings.Join(words, " ")
	if len(title) <= 80 {
		return title
	}
	return strings.TrimSpace(title[:80])
}

func normalizeConversationLimit(limit int) int {
	if limit <= 0 {
		return defaultConversationListLimit
	}
	if limit > maxConversationListLimit {
		return maxConversationListLimit
	}
	return limit
}
