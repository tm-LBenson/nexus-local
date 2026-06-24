package domain

import (
	"fmt"
	"strings"
	"time"
)

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

type Conversation struct {
	ID          ConversationID
	TenantID    TenantID
	OwnerID     UserID
	Title       string
	ModelTarget string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Message struct {
	ID             MessageID
	TenantID       TenantID
	ConversationID ConversationID
	Role           MessageRole
	Content        string
	CreatedAt      time.Time
}

type ConversationCreate struct {
	ID          ConversationID
	TenantID    TenantID
	OwnerID     UserID
	Title       string
	ModelTarget string
	Now         time.Time
}

type MessageCreate struct {
	ID             MessageID
	TenantID       TenantID
	ConversationID ConversationID
	Role           MessageRole
	Content        string
	Now            time.Time
}

func NewConversation(input ConversationCreate) (Conversation, error) {
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.OwnerID)) ||
		strings.TrimSpace(input.ModelTarget) == "" {
		return Conversation{}, fmt.Errorf("conversation: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Conversation{
		ID:          input.ID,
		TenantID:    input.TenantID,
		OwnerID:     input.OwnerID,
		Title:       strings.TrimSpace(input.Title),
		ModelTarget: strings.ToLower(strings.TrimSpace(input.ModelTarget)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func NewMessage(input MessageCreate) (Message, error) {
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.ConversationID)) ||
		!input.Role.Valid() ||
		strings.TrimSpace(input.Content) == "" {
		return Message{}, fmt.Errorf("message: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Message{
		ID:             input.ID,
		TenantID:       input.TenantID,
		ConversationID: input.ConversationID,
		Role:           input.Role,
		Content:        strings.TrimSpace(input.Content),
		CreatedAt:      now,
	}, nil
}

func (r MessageRole) Valid() bool {
	switch r {
	case MessageRoleSystem, MessageRoleUser, MessageRoleAssistant, MessageRoleTool:
		return true
	default:
		return false
	}
}
