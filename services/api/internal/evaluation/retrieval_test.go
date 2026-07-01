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
				ID:       "redirect",
				Query:    "OIDC invalid redirect uri",
				Limit:    2,
				Strategy: "hybrid",
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
	if report.Recall != 1 || report.MeanExpectedRank != 1 || report.TotalExpected != 1 || report.MatchedExpected != 1 {
		t.Fatalf("metrics = recall %.2f mean rank %.2f expected %d/%d, want perfect metrics", report.Recall, report.MeanExpectedRank, report.MatchedExpected, report.TotalExpected)
	}
	if report.CaseReports[0].TopHits[0].DocumentID != "doc_oidc" {
		t.Fatalf("top hit = %#v, want doc_oidc", report.CaseReports[0].TopHits[0])
	}
	if report.CaseReports[0].Recall != 1 || len(report.CaseReports[0].ExpectedRanks) != 1 || report.CaseReports[0].ExpectedRanks[0] != 1 {
		t.Fatalf("case metrics = %#v, want rank 1 recall", report.CaseReports[0])
	}
	if report.CaseReports[0].Strategy != "hybrid" {
		t.Fatalf("strategy = %q, want hybrid", report.CaseReports[0].Strategy)
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

func TestRunRetrievalSuiteReportsNoHitBehavior(t *testing.T) {
	suite := RetrievalSuite{
		Name: "test-suite",
		Documents: []RetrievalDocument{
			{
				ID:       "doc_oidc",
				Name:     "OIDC.md",
				Metadata: map[string]string{"topic": "identity"},
				Chunks: []RetrievalChunk{
					{ID: "redirect_uri", Text: "OIDC invalid redirect uri"},
				},
			},
		},
		Cases: []RetrievalCase{
			{
				ID:      "filtered-empty",
				Query:   "OIDC invalid redirect uri",
				Filters: map[string]string{"topic": "billing"},
				Expected: RetrievalExpectation{
					NoHits: true,
				},
			},
		},
	}

	report, err := RunRetrievalSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if !report.Passed || report.NoHitCaseCount != 1 || report.NoHitPassedCount != 1 {
		t.Fatalf("report = %#v, want passing no-hit metrics", report)
	}
	if !report.CaseReports[0].NoHitExpected || report.CaseReports[0].HitCount != 0 {
		t.Fatalf("case = %#v, want no-hit case with no hits", report.CaseReports[0])
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

func TestLoadRetrievalSuiteRejectsUnknownStrategy(t *testing.T) {
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
				"strategy": "keyword-only",
				"expected": {
					"hits": [{"document_id": "doc_1", "chunk_id": "chunk_1"}]
				}
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unknown strategy") {
		t.Fatalf("err = %v, want unknown strategy", err)
	}
}
