package app

import (
	"strings"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

func TestRewriteRetrievalQueryUsesRecentUserContextForVagueFollowUp(t *testing.T) {
	history := []domain.Message{
		{
			Role:      domain.MessageRoleUser,
			Content:   "Why does OIDC fail with invalid redirect uri callback mismatch?",
			CreatedAt: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			Role:      domain.MessageRoleAssistant,
			Content:   "The callback URL must match the configured redirect URI.",
			CreatedAt: time.Date(2026, 6, 30, 12, 0, 1, 0, time.UTC),
		},
	}

	query := rewriteRetrievalQuery("How do I fix it?", history)
	if !strings.Contains(query, "OIDC fail with invalid redirect uri") || !strings.Contains(query, "How do I fix it?") {
		t.Fatalf("query = %q, want previous topic plus current question", query)
	}
}

func TestRewriteRetrievalQueryKeepsSpecificQuestion(t *testing.T) {
	history := []domain.Message{
		{Role: domain.MessageRoleUser, Content: "Why does OIDC fail with invalid redirect uri callback mismatch?"},
	}

	question := "How do I rotate the OIDC client secret?"
	if got := rewriteRetrievalQuery(question, history); got != question {
		t.Fatalf("query = %q, want unchanged specific question", got)
	}
}

func TestRewriteRetrievalQueryWithoutHistoryKeepsFollowUp(t *testing.T) {
	question := "How do I fix it?"
	if got := rewriteRetrievalQuery(question, nil); got != question {
		t.Fatalf("query = %q, want unchanged question without history", got)
	}
}
