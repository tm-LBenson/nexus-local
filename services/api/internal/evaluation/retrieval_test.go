package evaluation

import (
	"context"
	"strings"
	"testing"
)

func TestRunRetrievalSuitePassesExpectedHits(t *testing.T) {
	suite := RetrievalSuite{
		Name: "test-suite",
		Documents: []RetrievalDocument{
			{
				ID:   "doc_oidc",
				Name: "OIDC.md",
				Chunks: []RetrievalChunk{
					{
						ID:   "redirect_uri",
						Text: "OIDC invalid redirect uri callback mismatch redirect uri exact match",
					},
				},
			},
			{
				ID:   "doc_billing",
				Name: "Billing.md",
				Chunks: []RetrievalChunk{
					{
						ID:   "invoice",
						Text: "invoice receipt payroll reimbursement vendor",
					},
				},
			},
		},
		Cases: []RetrievalCase{
			{
				ID:    "redirect",
				Query: "OIDC invalid redirect uri",
				Limit: 2,
				Expected: RetrievalExpectation{
					Hits:    []ExpectedHit{{DocumentID: "doc_oidc", ChunkID: "redirect_uri"}},
					MaxRank: 1,
				},
			},
		},
	}

	report, err := RunRetrievalSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if !report.Passed || report.PassedCount != 1 || report.FailedCount != 0 {
		t.Fatalf("report = %#v, want pass", report)
	}
	if report.CaseReports[0].TopHits[0].DocumentID != "doc_oidc" {
		t.Fatalf("top hit = %#v, want doc_oidc", report.CaseReports[0].TopHits[0])
	}
}

func TestRunRetrievalSuiteReportsMissingHit(t *testing.T) {
	suite := RetrievalSuite{
		Name: "test-suite",
		Documents: []RetrievalDocument{
			{
				ID:   "doc_oidc",
				Name: "OIDC.md",
				Chunks: []RetrievalChunk{
					{ID: "redirect_uri", Text: "OIDC invalid redirect uri"},
				},
			},
			{
				ID:   "doc_billing",
				Name: "Billing.md",
				Chunks: []RetrievalChunk{
					{ID: "invoice", Text: "invoice receipt payroll"},
				},
			},
		},
		Cases: []RetrievalCase{
			{
				ID:    "wrong-expectation",
				Query: "OIDC invalid redirect uri",
				Expected: RetrievalExpectation{
					Hits:    []ExpectedHit{{DocumentID: "doc_billing", ChunkID: "invoice"}},
					MaxRank: 1,
				},
			},
		},
	}

	report, err := RunRetrievalSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if report.Passed || report.FailedCount != 1 {
		t.Fatalf("report = %#v, want failure", report)
	}
	if len(report.CaseReports[0].Failures) == 0 {
		t.Fatalf("failures = %#v, want explanation", report.CaseReports[0].Failures)
	}
}

func TestLoadRetrievalSuiteRejectsUnknownChunk(t *testing.T) {
	_, err := LoadRetrievalSuite(strings.NewReader(`{
		"name": "bad-suite",
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
				"query": "hello",
				"expected": {
					"hits": [{"document_id": "doc_1", "chunk_id": "missing"}]
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
