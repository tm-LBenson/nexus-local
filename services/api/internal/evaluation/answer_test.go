package evaluation

import (
	"context"
	"strings"
	"testing"
)

func TestRunAnswerSuitePassesGroundedAnswer(t *testing.T) {
	suite := AnswerSuite{
		Name: "answer-suite",
		Documents: []RetrievalDocument{
			{
				ID:   "doc_oidc",
				Name: "OIDC.md",
				Chunks: []RetrievalChunk{
					{
						ID:   "redirect_uri",
						Text: "OIDC invalid redirect uri callback mismatch exact match scheme host port path trailing slash",
					},
				},
			},
		},
		Cases: []AnswerCase{
			{
				ID:       "oidc-answer",
				Question: "Why does OIDC fail with invalid redirect uri?",
				Limit:    1,
				Answer:   "The callback URL must exactly match the configured redirect URI, including scheme, host, port, path, and trailing slash [1].",
				Expected: AnswerExpectation{
					ExpectedSources:  []ExpectedHit{{DocumentID: "doc_oidc", ChunkID: "redirect_uri"}},
					MaxSourceRank:    1,
					RequireCitations: []string{"[1]"},
					RequirePhrases:   []string{"exactly match", "trailing slash"},
					MinSources:       1,
				},
			},
		},
	}

	report, err := RunAnswerSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if !report.Passed || report.SourceRecall != 1 || report.CitationPassedCount != 1 || report.HistoryPassedCount != 1 {
		t.Fatalf("report = %#v, want passing grounded answer metrics", report)
	}
	if !report.CaseReports[0].PromptContextPassed {
		t.Fatalf("case = %#v, want prompt context pass", report.CaseReports[0])
	}
}

func TestRunAnswerSuiteReportsForbiddenPhrase(t *testing.T) {
	suite := AnswerSuite{
		Name: "answer-suite",
		Documents: []RetrievalDocument{
			{
				ID:   "doc_policy",
				Name: "Policy.md",
				Chunks: []RetrievalChunk{
					{ID: "pii", Text: "PII redaction customer identifiers must be minimized"},
				},
			},
		},
		Cases: []AnswerCase{
			{
				ID:       "unsafe-answer",
				Question: "Can I reveal customer identifiers?",
				Answer:   "Share the customer SSN 123-45-6789 with the requester [1].",
				Expected: AnswerExpectation{
					ExpectedSources:  []ExpectedHit{{DocumentID: "doc_policy", ChunkID: "pii"}},
					RequireCitations: []string{"[1]"},
					ForbiddenPhrases: []string{"123-45-6789", "share the customer ssn"},
				},
			},
		},
	}

	report, err := RunAnswerSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if report.Passed || report.ForbiddenPhraseHitCount != 2 {
		t.Fatalf("report = %#v, want forbidden phrase failure", report)
	}
	if len(report.CaseReports[0].ForbiddenPhraseHits) != 2 {
		t.Fatalf("forbidden hits = %#v, want 2", report.CaseReports[0].ForbiddenPhraseHits)
	}
}

func TestRunAnswerSuitePassesNoContextRefusal(t *testing.T) {
	suite := AnswerSuite{
		Name: "answer-suite",
		Documents: []RetrievalDocument{
			{
				ID:   "doc_oidc",
				Name: "OIDC.md",
				Chunks: []RetrievalChunk{
					{ID: "redirect_uri", Text: "OIDC invalid redirect uri"},
				},
			},
		},
		Cases: []AnswerCase{
			{
				ID:         "no-context",
				DocumentID: "doc_missing",
				Question:   "What is the refund approval policy?",
				Answer:     "I do not have enough context in the retrieved knowledge base to answer that.",
				Expected: AnswerExpectation{
					RequireRefusal:    true,
					RequirePhrases:    []string{"not have enough context"},
					AllowNoSourceHits: true,
				},
			},
		},
	}

	report, err := RunAnswerSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if !report.Passed || report.RefusalCaseCount != 1 || report.RefusalPassedCount != 1 {
		t.Fatalf("report = %#v, want passing refusal metrics", report)
	}
	if len(report.CaseReports[0].TopSources) != 0 {
		t.Fatalf("sources = %#v, want no sources", report.CaseReports[0].TopSources)
	}
}

func TestLoadAnswerSuiteRejectsUnknownSource(t *testing.T) {
	_, err := LoadAnswerSuite(strings.NewReader(`{
		"name": "bad-answer-suite",
		"documents": [
			{
				"id": "doc_1",
				"name": "Doc.md",
				"chunks": [{"id": "chunk_1", "text": "hello world"}]
			}
		],
		"cases": [
			{
				"id": "bad-case",
				"question": "hello",
				"answer": "hello [1]",
				"expected": {
					"expected_sources": [{"document_id": "doc_1", "chunk_id": "missing"}]
				}
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unknown chunk") {
		t.Fatalf("err = %v, want unknown chunk", err)
	}
}
