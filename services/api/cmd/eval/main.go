package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tm-lbenson/nexus-local/services/api/internal/evaluation"
)

func main() {
	kind := flag.String("kind", "retrieval", "eval kind: retrieval or answer")
	fixturePath := flag.String("fixture", "", "fixture JSON path")
	jsonOutput := flag.Bool("json", false, "write JSON report")
	flag.Parse()

	resolvedKind := strings.ToLower(strings.TrimSpace(*kind))
	resolvedFixture, err := resolveFixturePath(resolvedKind, *fixturePath)
	if err != nil {
		fail("%v", err)
	}
	file, err := os.Open(resolvedFixture)
	if err != nil {
		fail("open fixture: %v", err)
	}
	defer file.Close()

	switch resolvedKind {
	case "retrieval":
		runRetrieval(file, *jsonOutput)
	case "answer":
		runAnswer(file, *jsonOutput)
	default:
		fail("unknown eval kind %q; use retrieval or answer", *kind)
	}
}

func runRetrieval(file *os.File, jsonOutput bool) {
	suite, err := evaluation.LoadRetrievalSuite(file)
	if err != nil {
		fail("%v", err)
	}
	report, err := evaluation.RunRetrievalSuite(context.Background(), suite)
	if err != nil {
		fail("run retrieval eval: %v", err)
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail("write JSON report: %v", err)
		}
	} else {
		writeTextReport(report)
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func runAnswer(file *os.File, jsonOutput bool) {
	suite, err := evaluation.LoadAnswerSuite(file)
	if err != nil {
		fail("%v", err)
	}
	report, err := evaluation.RunAnswerSuite(context.Background(), suite)
	if err != nil {
		fail("run answer eval: %v", err)
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail("write JSON report: %v", err)
		}
	} else {
		writeAnswerReport(report)
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func resolveFixturePath(kind string, path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return path, nil
	}
	name := "retrieval-baseline.json"
	if kind == "answer" {
		name = "answer-baseline.json"
	}
	candidates := []string{
		filepath.Join("fixtures", "eval", name),
		filepath.Join("..", "..", "fixtures", "eval", name),
		filepath.Join("..", "..", "..", "fixtures", "eval", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s fixture not found; pass -fixture", kind)
}

func writeTextReport(report evaluation.RetrievalReport) {
	status := "PASS"
	if !report.Passed {
		status = "FAIL"
	}
	fmt.Printf("Nexus Local retrieval eval: %s\n", status)
	fmt.Printf("Suite: %s\n", report.Suite)
	if report.Description != "" {
		fmt.Printf("Description: %s\n", report.Description)
	}
	fmt.Printf("Cases: %d passed / %d failed / %d total\n", report.PassedCount, report.FailedCount, report.CaseCount)
	fmt.Printf("Indexed chunks: %d\n", report.IndexedChunks)
	fmt.Printf("Embedding: %s / %d dims\n", report.Embedding.Model, report.Embedding.Dimensions)
	fmt.Printf("Recall: %.2f (%d/%d expected hits)\n", report.Recall, report.MatchedExpected, report.TotalExpected)
	if report.MeanExpectedRank > 0 {
		fmt.Printf("Mean expected rank: %.2f\n", report.MeanExpectedRank)
	}
	fmt.Printf("P95 case latency: %dms\n", report.P95LatencyMS)
	if report.NoHitCaseCount > 0 {
		fmt.Printf("No-hit cases: %d/%d passed\n", report.NoHitPassedCount, report.NoHitCaseCount)
	}
	fmt.Println("")

	for _, testCase := range report.CaseReports {
		caseStatus := "PASS"
		if !testCase.Passed {
			caseStatus = "FAIL"
		}
		fmt.Printf("[%s] %s\n", caseStatus, testCase.ID)
		fmt.Printf("  Query: %s\n", testCase.Query)
		fmt.Printf("  Strategy: %s\n", testCase.Strategy)
		fmt.Printf("  Expected: %d/%d matched within rank %d\n", testCase.ExpectedMatched, testCase.ExpectedTotal, testCase.MaxRank)
		fmt.Printf("  Metrics: recall %.2f, latency %dms", testCase.Recall, testCase.LatencyMS)
		if len(testCase.ExpectedRanks) > 0 {
			fmt.Printf(", ranks %s", formatRanks(testCase.ExpectedRanks))
		}
		if testCase.NoHitExpected {
			fmt.Print(", no-hit expected")
		}
		fmt.Println("")
		for _, failure := range testCase.Failures {
			fmt.Printf("  - %s\n", failure)
		}
		for _, hit := range testCase.TopHits {
			fmt.Printf("  #%d %.4f %s#%s\n", hit.Rank, hit.Score, hit.DocumentID, hit.ChunkID)
		}
	}
}

func writeAnswerReport(report evaluation.AnswerReport) {
	status := "PASS"
	if !report.Passed {
		status = "FAIL"
	}
	fmt.Printf("Nexus Local answer eval: %s\n", status)
	fmt.Printf("Suite: %s\n", report.Suite)
	if report.Description != "" {
		fmt.Printf("Description: %s\n", report.Description)
	}
	fmt.Printf("Cases: %d passed / %d failed / %d total\n", report.PassedCount, report.FailedCount, report.CaseCount)
	fmt.Printf("Indexed chunks: %d\n", report.IndexedChunks)
	fmt.Printf("Embedding: %s / %d dims\n", report.Embedding.Model, report.Embedding.Dimensions)
	fmt.Printf("Source recall: %.2f (%d/%d expected sources)\n", report.SourceRecall, report.MatchedExpectedSources, report.TotalExpectedSources)
	fmt.Printf("Citation checks: %d/%d passed\n", report.CitationPassedCount, report.CaseCount)
	if report.RefusalCaseCount > 0 {
		fmt.Printf("Refusal checks: %d/%d passed\n", report.RefusalPassedCount, report.RefusalCaseCount)
	}
	fmt.Printf("History checks: %d/%d passed\n", report.HistoryPassedCount, report.CaseCount)
	fmt.Printf("Prompt context checks: %d/%d passed\n", report.PromptContextPassCount, report.CaseCount)
	fmt.Printf("P95 case latency: %dms\n", report.P95LatencyMS)
	if report.ForbiddenPhraseHitCount > 0 {
		fmt.Printf("Forbidden phrase hits: %d\n", report.ForbiddenPhraseHitCount)
	}
	fmt.Println("")

	for _, testCase := range report.CaseReports {
		caseStatus := "PASS"
		if !testCase.Passed {
			caseStatus = "FAIL"
		}
		fmt.Printf("[%s] %s\n", caseStatus, testCase.ID)
		fmt.Printf("  Question: %s\n", testCase.Question)
		fmt.Printf("  Sources: %d/%d expected matched", testCase.ExpectedSourceMatched, testCase.ExpectedSourceTotal)
		if len(testCase.ExpectedSourceRanks) > 0 {
			fmt.Printf(", ranks %s", formatRanks(testCase.ExpectedSourceRanks))
		}
		fmt.Println("")
		fmt.Printf("  Citations: %d/%d, required phrases: %d/%d, latency: %dms\n", testCase.CitationMatched, testCase.CitationTotal, testCase.RequiredPhraseMatched, testCase.RequiredPhraseTotal, testCase.LatencyMS)
		if testCase.RefusalExpected {
			fmt.Printf("  Refusal: %t\n", testCase.RefusalPassed)
		}
		for _, failure := range testCase.Failures {
			fmt.Printf("  - %s\n", failure)
		}
		for _, hit := range testCase.TopSources {
			fmt.Printf("  #%d %.4f %s#%s\n", hit.Rank, hit.Score, hit.DocumentID, hit.ChunkID)
		}
	}
}

func formatRanks(ranks []int) string {
	if len(ranks) == 0 {
		return "[]"
	}
	formatted := "["
	for i, rank := range ranks {
		if i > 0 {
			formatted += ", "
		}
		formatted += fmt.Sprintf("%d", rank)
	}
	return formatted + "]"
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
