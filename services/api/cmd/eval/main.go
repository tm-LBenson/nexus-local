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
	fmt.Println("")

	for _, testCase := range report.CaseReports {
		caseStatus := "PASS"
		if !testCase.Passed {
			caseStatus = "FAIL"
		}
		fmt.Printf("[%s] %s\n", caseStatus, testCase.ID)
		fmt.Printf("  Query: %s\n", testCase.Query)
		fmt.Printf("  Expected: %d/%d matched within rank %d\n", testCase.ExpectedMatched, testCase.ExpectedTotal, testCase.MaxRank)
		for _, failure := range testCase.Failures {
			fmt.Printf("  - %s\n", failure)
		}
		for _, hit := range testCase.TopHits {
			fmt.Printf("  #%d %.4f %s#%s\n", hit.Rank, hit.Score, hit.DocumentID, hit.ChunkID)
		}
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
