package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	embeddinghash "github.com/tm-lbenson/nexus-local/services/api/internal/providers/embeddings/hash"
	vectormemory "github.com/tm-lbenson/nexus-local/services/api/internal/providers/vector/memory"
)

const (
	defaultEvalTenantID            = "tenant_eval"
	defaultEvalEmbeddingModel      = "eval-hash"
	defaultEvalEmbeddingDimensions = 256
	defaultRetrievalLimit          = 5
)

type RetrievalSuite struct {
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	TenantID            string                 `json:"tenant_id,omitempty"`
	EmbeddingModel      string                 `json:"embedding_model,omitempty"`
	EmbeddingDimensions int                    `json:"embedding_dimensions,omitempty"`
	Documents           []RetrievalDocument    `json:"documents"`
	Cases               []RetrievalCase        `json:"cases"`
	Metadata            map[string]string      `json:"metadata,omitempty"`
	Expectations        map[string]interface{} `json:"expectations,omitempty"`
}

type RetrievalDocument struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Chunks   []RetrievalChunk  `json:"chunks"`
}

type RetrievalChunk struct {
	ID       string            `json:"id"`
	Text     string            `json:"text"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type RetrievalCase struct {
	ID         string               `json:"id"`
	Query      string               `json:"query"`
	Limit      int                  `json:"limit,omitempty"`
	DocumentID string               `json:"document_id,omitempty"`
	Filters    map[string]string    `json:"filters,omitempty"`
	Strategy   string               `json:"strategy,omitempty"`
	Expected   RetrievalExpectation `json:"expected"`
}

type RetrievalExpectation struct {
	Hits       []ExpectedHit `json:"hits,omitempty"`
	MaxRank    int           `json:"max_rank,omitempty"`
	RequireAll bool          `json:"require_all,omitempty"`
	NoHits     bool          `json:"no_hits,omitempty"`
	MinScore   float32       `json:"min_score,omitempty"`
}

type ExpectedHit struct {
	DocumentID string `json:"document_id"`
	ChunkID    string `json:"chunk_id,omitempty"`
}

type RetrievalReport struct {
	Suite            string                `json:"suite"`
	Description      string                `json:"description,omitempty"`
	Passed           bool                  `json:"passed"`
	CaseCount        int                   `json:"case_count"`
	PassedCount      int                   `json:"passed_count"`
	FailedCount      int                   `json:"failed_count"`
	DurationMS       int64                 `json:"duration_ms"`
	P95LatencyMS     int64                 `json:"p95_latency_ms"`
	Recall           float64               `json:"recall"`
	MeanExpectedRank float64               `json:"mean_expected_rank,omitempty"`
	MatchedExpected  int                   `json:"matched_expected"`
	TotalExpected    int                   `json:"total_expected"`
	NoHitCaseCount   int                   `json:"no_hit_case_count,omitempty"`
	NoHitPassedCount int                   `json:"no_hit_passed_count,omitempty"`
	Embedding        RetrievalEmbedding    `json:"embedding"`
	CaseReports      []RetrievalCaseReport `json:"cases"`
	IndexedChunks    int                   `json:"indexed_chunks"`
}

type RetrievalEmbedding struct {
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

type RetrievalCaseReport struct {
	ID              string             `json:"id"`
	Query           string             `json:"query"`
	Passed          bool               `json:"passed"`
	Failures        []string           `json:"failures,omitempty"`
	Recall          float64            `json:"recall"`
	LatencyMS       int64              `json:"latency_ms"`
	ExpectedMatched int                `json:"expected_matched"`
	ExpectedTotal   int                `json:"expected_total"`
	ExpectedRanks   []int              `json:"expected_ranks,omitempty"`
	NoHitExpected   bool               `json:"no_hit_expected,omitempty"`
	HitCount        int                `json:"hit_count"`
	Limit           int                `json:"limit"`
	MaxRank         int                `json:"max_rank"`
	Strategy        string             `json:"strategy"`
	TopHits         []RetrievalHitView `json:"top_hits"`
}

type RetrievalHitView struct {
	Rank       int               `json:"rank"`
	DocumentID string            `json:"document_id"`
	ChunkID    string            `json:"chunk_id"`
	Score      float32           `json:"score"`
	Text       string            `json:"text"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func LoadRetrievalSuite(reader io.Reader) (RetrievalSuite, error) {
	var suite RetrievalSuite
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&suite); err != nil {
		return RetrievalSuite{}, fmt.Errorf("decode retrieval suite: %w", err)
	}
	if err := validateRetrievalSuite(suite); err != nil {
		return RetrievalSuite{}, err
	}
	return suite, nil
}

func RunRetrievalSuite(ctx context.Context, suite RetrievalSuite) (RetrievalReport, error) {
	if err := validateRetrievalSuite(suite); err != nil {
		return RetrievalReport{}, err
	}
	startedAt := time.Now()
	tenantID := suite.TenantID
	if strings.TrimSpace(tenantID) == "" {
		tenantID = defaultEvalTenantID
	}
	model := suite.EmbeddingModel
	if strings.TrimSpace(model) == "" {
		model = defaultEvalEmbeddingModel
	}
	dimensions := suite.EmbeddingDimensions
	if dimensions <= 0 {
		dimensions = defaultEvalEmbeddingDimensions
	}

	embedder := embeddinghash.New(model, dimensions)
	vectorIndex := vectormemory.New()
	vectors, err := suiteVectors(ctx, suite, tenantID, embedder)
	if err != nil {
		return RetrievalReport{}, err
	}
	if err := vectorIndex.Upsert(ctx, vectors); err != nil {
		return RetrievalReport{}, err
	}

	search := app.NewSearchService(embedder, vectorIndex)
	report := RetrievalReport{
		Suite:       suite.Name,
		Description: suite.Description,
		Embedding: RetrievalEmbedding{
			Model:      model,
			Dimensions: dimensions,
		},
		CaseCount:     len(suite.Cases),
		CaseReports:   make([]RetrievalCaseReport, 0, len(suite.Cases)),
		IndexedChunks: len(vectors),
	}
	for _, testCase := range suite.Cases {
		caseReport, err := runRetrievalCase(ctx, search, tenantID, testCase)
		if err != nil {
			return RetrievalReport{}, err
		}
		report.CaseReports = append(report.CaseReports, caseReport)
		if caseReport.Passed {
			report.PassedCount++
		}
	}
	reportMetrics(&report)
	report.FailedCount = report.CaseCount - report.PassedCount
	report.Passed = report.FailedCount == 0
	report.DurationMS = time.Since(startedAt).Milliseconds()
	return report, nil
}

func suiteVectors(ctx context.Context, suite RetrievalSuite, tenantID string, embedder providers.Embedder) ([]providers.Vector, error) {
	texts := make([]string, 0)
	refs := make([]providers.Vector, 0)
	for _, document := range suite.Documents {
		for _, chunk := range document.Chunks {
			metadata := mergeMetadata(document.Metadata, chunk.Metadata)
			metadata["document_name"] = document.Name
			metadata["chunk_id"] = chunk.ID
			texts = append(texts, chunk.Text)
			refs = append(refs, providers.Vector{
				TenantID:   domain.TenantID(tenantID),
				DocumentID: domain.DocumentID(document.ID),
				ChunkID:    chunk.ID,
				Text:       chunk.Text,
				Metadata:   metadata,
			})
		}
	}
	embedded, err := embedder.Embed(ctx, providers.EmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, err
	}
	if len(embedded.Vectors) != len(refs) {
		return nil, fmt.Errorf("embedding count %d does not match chunk count %d", len(embedded.Vectors), len(refs))
	}
	for i := range refs {
		refs[i].Values = embedded.Vectors[i]
	}
	return refs, nil
}

func runRetrievalCase(ctx context.Context, search app.SearchService, tenantID string, testCase RetrievalCase) (RetrievalCaseReport, error) {
	limit := testCase.Limit
	if limit <= 0 {
		limit = defaultRetrievalLimit
	}
	maxRank := testCase.Expected.MaxRank
	if maxRank <= 0 {
		maxRank = limit
	}
	strategy, err := app.NormalizeSearchStrategy(app.SearchStrategy(testCase.Strategy))
	if err != nil {
		return RetrievalCaseReport{}, fmt.Errorf("retrieval case %q strategy: %w", testCase.ID, err)
	}
	startedAt := time.Now()
	result, err := search.Search(ctx, app.SearchInput{
		TenantID:   domain.TenantID(tenantID),
		DocumentID: domain.DocumentID(testCase.DocumentID),
		Query:      testCase.Query,
		Limit:      limit,
		Filters:    testCase.Filters,
		Strategy:   strategy,
	})
	if err != nil {
		return RetrievalCaseReport{}, err
	}
	report := RetrievalCaseReport{
		ID:            testCase.ID,
		Query:         testCase.Query,
		Limit:         limit,
		MaxRank:       maxRank,
		HitCount:      len(result.Hits),
		ExpectedTotal: len(testCase.Expected.Hits),
		NoHitExpected: testCase.Expected.NoHits,
		LatencyMS:     time.Since(startedAt).Milliseconds(),
		Strategy:      string(strategy),
		TopHits:       hitViews(result.Hits),
	}
	evaluation := evaluateHits(result.Hits, testCase.Expected, maxRank)
	report.ExpectedMatched = evaluation.matched
	report.ExpectedRanks = evaluation.ranks
	report.Failures = evaluation.failures
	if report.ExpectedTotal > 0 {
		report.Recall = float64(report.ExpectedMatched) / float64(report.ExpectedTotal)
	}
	report.Passed = len(report.Failures) == 0
	return report, nil
}

type hitEvaluation struct {
	matched  int
	ranks    []int
	failures []string
}

func evaluateHits(hits []providers.VectorHit, expectation RetrievalExpectation, maxRank int) hitEvaluation {
	if expectation.NoHits {
		if len(hits) == 0 {
			return hitEvaluation{}
		}
		return hitEvaluation{failures: []string{fmt.Sprintf("expected no hits, got %d", len(hits))}}
	}
	if len(expectation.Hits) == 0 {
		return hitEvaluation{failures: []string{"expected at least one hit expectation"}}
	}

	matched := 0
	ranks := make([]int, 0, len(expectation.Hits))
	failures := make([]string, 0)
	for _, expected := range expectation.Hits {
		rank, ok := expectedHitRank(hits, expected, maxRank)
		if ok {
			matched++
			ranks = append(ranks, rank)
			if expectation.MinScore > 0 && hits[rank-1].Score < expectation.MinScore {
				failures = append(failures, fmt.Sprintf("%s score %.4f below %.4f", expectedLabel(expected), hits[rank-1].Score, expectation.MinScore))
			}
			continue
		}
		if expectation.RequireAll {
			failures = append(failures, fmt.Sprintf("%s not found within rank %d", expectedLabel(expected), maxRank))
		}
	}
	if !expectation.RequireAll && matched == 0 {
		failures = append(failures, fmt.Sprintf("none of %d expected hits found within rank %d", len(expectation.Hits), maxRank))
	}
	return hitEvaluation{matched: matched, ranks: ranks, failures: failures}
}

func expectedHitRank(hits []providers.VectorHit, expected ExpectedHit, maxRank int) (int, bool) {
	for i, hit := range hits {
		rank := i + 1
		if rank > maxRank {
			break
		}
		if string(hit.DocumentID) != expected.DocumentID {
			continue
		}
		if expected.ChunkID != "" && hit.ChunkID != expected.ChunkID {
			continue
		}
		return rank, true
	}
	return 0, false
}

func hitViews(hits []providers.VectorHit) []RetrievalHitView {
	views := make([]RetrievalHitView, 0, len(hits))
	for i, hit := range hits {
		views = append(views, RetrievalHitView{
			Rank:       i + 1,
			DocumentID: string(hit.DocumentID),
			ChunkID:    hit.ChunkID,
			Score:      hit.Score,
			Text:       compactText(hit.Text, 180),
			Metadata:   hit.Metadata,
		})
	}
	return views
}

func reportMetrics(report *RetrievalReport) {
	latencies := make([]int64, 0, len(report.CaseReports))
	rankTotal := 0
	rankCount := 0
	for _, testCase := range report.CaseReports {
		latencies = append(latencies, testCase.LatencyMS)
		report.MatchedExpected += testCase.ExpectedMatched
		report.TotalExpected += testCase.ExpectedTotal
		for _, rank := range testCase.ExpectedRanks {
			if rank <= 0 {
				continue
			}
			rankTotal += rank
			rankCount++
		}
		if testCase.NoHitExpected {
			report.NoHitCaseCount++
			if testCase.Passed {
				report.NoHitPassedCount++
			}
		}
	}
	if report.TotalExpected > 0 {
		report.Recall = float64(report.MatchedExpected) / float64(report.TotalExpected)
	}
	if rankCount > 0 {
		report.MeanExpectedRank = float64(rankTotal) / float64(rankCount)
	}
	report.P95LatencyMS = percentileLatency(latencies, 95)
}

func percentileLatency(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 100 {
		return sorted[len(sorted)-1]
	}
	index := ((percentile * len(sorted)) + 99) / 100
	if index <= 0 {
		index = 1
	}
	return sorted[index-1]
}

func validateRetrievalSuite(suite RetrievalSuite) error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("retrieval suite name is required")
	}
	if len(suite.Documents) == 0 {
		return fmt.Errorf("retrieval suite needs at least one document")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("retrieval suite needs at least one case")
	}
	documentIDs := map[string]bool{}
	chunkIDs := map[string]map[string]bool{}
	for _, document := range suite.Documents {
		if strings.TrimSpace(document.ID) == "" || strings.TrimSpace(document.Name) == "" {
			return fmt.Errorf("retrieval document id and name are required")
		}
		if documentIDs[document.ID] {
			return fmt.Errorf("duplicate document id %q", document.ID)
		}
		documentIDs[document.ID] = true
		chunkIDs[document.ID] = map[string]bool{}
		if len(document.Chunks) == 0 {
			return fmt.Errorf("retrieval document %q needs at least one chunk", document.ID)
		}
		for _, chunk := range document.Chunks {
			if strings.TrimSpace(chunk.ID) == "" || strings.TrimSpace(chunk.Text) == "" {
				return fmt.Errorf("retrieval document %q has a chunk without id or text", document.ID)
			}
			if chunkIDs[document.ID][chunk.ID] {
				return fmt.Errorf("duplicate chunk id %q in document %q", chunk.ID, document.ID)
			}
			chunkIDs[document.ID][chunk.ID] = true
		}
	}
	for _, testCase := range suite.Cases {
		if strings.TrimSpace(testCase.ID) == "" || strings.TrimSpace(testCase.Query) == "" {
			return fmt.Errorf("retrieval case id and query are required")
		}
		if _, err := app.NormalizeSearchStrategy(app.SearchStrategy(testCase.Strategy)); err != nil {
			return fmt.Errorf("retrieval case %q has unknown strategy %q", testCase.ID, testCase.Strategy)
		}
		if testCase.DocumentID != "" && !documentIDs[testCase.DocumentID] {
			return fmt.Errorf("retrieval case %q scopes to unknown document %q", testCase.ID, testCase.DocumentID)
		}
		if testCase.Expected.NoHits {
			continue
		}
		if len(testCase.Expected.Hits) == 0 {
			return fmt.Errorf("retrieval case %q needs expected hits or no_hits", testCase.ID)
		}
		for _, expected := range testCase.Expected.Hits {
			if !documentIDs[expected.DocumentID] {
				return fmt.Errorf("retrieval case %q expects unknown document %q", testCase.ID, expected.DocumentID)
			}
			if expected.ChunkID != "" && !chunkIDs[expected.DocumentID][expected.ChunkID] {
				return fmt.Errorf("retrieval case %q expects unknown chunk %q in document %q", testCase.ID, expected.ChunkID, expected.DocumentID)
			}
		}
	}
	return nil
}

func mergeMetadata(left map[string]string, right map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		merged[key] = value
	}
	return merged
}

func expectedLabel(expected ExpectedHit) string {
	if expected.ChunkID == "" {
		return expected.DocumentID
	}
	return fmt.Sprintf("%s#%s", expected.DocumentID, expected.ChunkID)
}

func compactText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "..."
}

func SortCaseReports(reports []RetrievalCaseReport) {
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ID < reports[j].ID
	})
}
