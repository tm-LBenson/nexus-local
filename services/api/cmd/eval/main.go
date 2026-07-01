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
	fixturePath := flag.String("fixture", "", "retrieval fixture JSON path")
	jsonOutput := flag.Bool("json", false, "write JSON report")
	flag.Parse()

	resolvedFixture, err := resolveFixturePath(*fixturePath)
	if err != nil {
		fail("%v", err)
	}
	file, err := os.Open(resolvedFixture)
	if err != nil {
		fail("open fixture: %v", err)
	}
	defer file.Close()

	suite, err := evaluation.LoadRetrievalSuite(file)
	if err != nil {
		fail("%v", err)
	}
	report, err := evaluation.RunRetrievalSuite(context.Background(), suite)
	if err != nil {
		fail("run retrieval eval: %v", err)
	}

	if *jsonOutput {
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

func resolveFixturePath(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return path, nil
	}
	candidates := []string{
		filepath.Join("fixtures", "eval", "retrieval-baseline.json"),
		filepath.Join("..", "..", "fixtures", "eval", "retrieval-baseline.json"),
		filepath.Join("..", "..", "..", "fixtures", "eval", "retrieval-baseline.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("retrieval fixture not found; pass -fixture")
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
