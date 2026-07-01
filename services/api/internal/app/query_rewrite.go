package app

import (
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

const (
	maxRewriteHistoryMessages = 2
	maxRetrievalQueryLength   = 360
)

var followUpPhrases = []string{
	"what about",
	"how about",
	"what should i do",
	"why is it",
	"why does it",
	"why did it",
	"what does that mean",
	"what next",
	"next step",
	"next steps",
	"fix it",
	"solve it",
	"troubleshoot it",
}

var ambiguousFollowUpWords = map[string]bool{
	"it": true, "that": true, "this": true, "they": true, "them": true,
	"these": true, "those": true, "issue": true, "problem": true, "thing": true,
}

var genericFollowUpTerms = map[string]bool{
	"about": true, "can": true, "check": true, "do": true, "explain": true,
	"fix": true, "help": true, "mean": true, "means": true, "next": true,
	"step": true, "steps": true, "tell": true, "work": true, "working": true,
	"you": true,
}

func rewriteRetrievalQuery(question string, history []domain.Message) string {
	question = strings.TrimSpace(question)
	if question == "" || !shouldRewriteRetrievalQuery(question) {
		return question
	}

	context := recentUserContext(history)
	if context == "" {
		return question
	}
	return compactRetrievalQuery(context + " " + question)
}

func shouldRewriteRetrievalQuery(question string) bool {
	lower := strings.ToLower(strings.TrimSpace(question))
	if lower == "" {
		return false
	}
	for _, phrase := range followUpPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	words := rawWords(lower)
	hasAmbiguousWord := false
	for _, word := range words {
		if ambiguousFollowUpWords[word] {
			hasAmbiguousWord = true
			break
		}
	}
	if !hasAmbiguousWord {
		return false
	}
	return len(topicalQueryTerms(question)) <= 4
}

func topicalQueryTerms(question string) []string {
	terms := make([]string, 0)
	for _, token := range tokenize(question) {
		if genericFollowUpTerms[token] {
			continue
		}
		terms = append(terms, token)
	}
	return terms
}

func recentUserContext(history []domain.Message) string {
	selected := make([]string, 0, maxRewriteHistoryMessages)
	for index := len(history) - 1; index >= 0 && len(selected) < maxRewriteHistoryMessages; index-- {
		message := history[index]
		if message.Role != domain.MessageRoleUser {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		selected = append(selected, content)
	}
	if len(selected) == 0 {
		return ""
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return compactRetrievalQuery(strings.Join(selected, " "))
}

func rawWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

func compactRetrievalQuery(query string) string {
	query = strings.Join(strings.Fields(query), " ")
	if len(query) <= maxRetrievalQueryLength {
		return query
	}
	return strings.TrimSpace(query[:maxRetrievalQueryLength])
}
