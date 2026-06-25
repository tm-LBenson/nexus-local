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

func (s ConversationService) Ask(ctx context.Context, input AskInput) (AskResult, error) {
	if err := ctx.Err(); err != nil {
		return AskResult{}, err
	}
	if s.models == nil {
		return AskResult{}, ErrConversationModelUnavailable
	}

	question := strings.TrimSpace(input.Question)
	if strings.TrimSpace(string(input.TenantID)) == "" ||
		strings.TrimSpace(string(input.OwnerID)) == "" ||
		question == "" {
		return AskResult{}, fmt.Errorf("ask: %w", domain.ErrInvalidEntity)
	}

	now := s.clock.Now()
	conversation, err := s.openConversation(ctx, input, question, now)
	if err != nil {
		return AskResult{}, err
	}

	searchResult, err := s.search.Search(ctx, SearchInput{
		TenantID: input.TenantID,
		Query:    question,
		Limit:    input.Limit,
	})
	if err != nil {
		return AskResult{}, err
	}

	history, err := s.repos.ListMessages(ctx, conversation.TenantID, conversation.ID)
	if err != nil {
		return AskResult{}, err
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
		return AskResult{}, err
	}
	if err := s.repos.SaveMessage(ctx, userMessage); err != nil {
		return AskResult{}, err
	}

	completion, err := s.models.Complete(ctx, providers.ChatCompletionRequest{
		Target:      conversation.ModelTarget,
		Model:       "",
		Messages:    promptMessages(history, question, searchResult.Hits),
		Temperature: 0.2,
		Metadata: map[string]string{
			"tenant_id":       string(conversation.TenantID),
			"conversation_id": string(conversation.ID),
		},
	})
	if err != nil {
		return AskResult{}, err
	}

	assistantNow := s.clock.Now()
	if !assistantNow.After(userMessage.CreatedAt) {
		assistantNow = userMessage.CreatedAt.Add(time.Nanosecond)
	}
	assistantMessage, err := domain.NewMessage(domain.MessageCreate{
		ID:             s.ids.NewMessageID(),
		TenantID:       conversation.TenantID,
		ConversationID: conversation.ID,
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

	conversation.UpdatedAt = assistantMessage.CreatedAt
	if err := s.repos.SaveConversation(ctx, conversation); err != nil {
		return AskResult{}, err
	}

	return AskResult{
		Conversation:     conversation,
		UserMessage:      userMessage,
		AssistantMessage: assistantMessage,
		Hits:             searchResult.Hits,
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
		builder.WriteString(fmt.Sprintf("[%d] document=%s chunk=%s score=%.3f\n%s\n\n", index+1, hit.DocumentID, hit.ChunkID, hit.Score, hit.Text))
	}
	return strings.TrimSpace(builder.String())
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
