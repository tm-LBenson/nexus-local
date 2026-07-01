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
	storememory "github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

const (
	defaultAnswerOwnerID = "user_eval"
	defaultAnswerTarget  = "general"
)

type AnswerSuite struct {
	Name                string              `json:"name"`
	Description         string              `json:"description,omitempty"`
	TenantID            string              `json:"tenant_id,omitempty"`
	OwnerID             string              `json:"owner_id,omitempty"`
	EmbeddingModel      string              `json:"embedding_model,omitempty"`
	EmbeddingDimensions int                 `json:"embedding_dimensions,omitempty"`
	Documents           []RetrievalDocument `json:"documents"`
	Cases               []AnswerCase        `json:"cases"`
}

type AnswerCase struct {
	ID          string            `json:"id"`
	Question    string            `json:"question"`
	DocumentID  string            `json:"document_id,omitempty"`
	ModelTarget string            `json:"model_target,omitempty"`
	Limit       int               `json:"limit,omitempty"`
	Strategy    string            `json:"strategy,omitempty"`
	History     []AnswerHistory   `json:"history,omitempty"`
	Answer      string            `json:"answer"`
	Expected    AnswerExpectation `json:"expected"`
}

type AnswerHistory struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnswerExpectation struct {
	ExpectedSources   []ExpectedHit `json:"expected_sources,omitempty"`
	MaxSourceRank     int           `json:"max_source_rank,omitempty"`
	RequireCitations  []string      `json:"require_citations,omitempty"`
	RequirePhrases    []string      `json:"require_phrases,omitempty"`
	ForbiddenPhrases  []string      `json:"forbidden_phrases,omitempty"`
	RequireRefusal    bool          `json:"require_refusal,omitempty"`
	MinSources        int           `json:"min_sources,omitempty"`
	MaxLatencyMS      int64         `json:"max_latency_ms,omitempty"`
	SkipHistoryCheck  bool          `json:"skip_history_check,omitempty"`
	SkipPromptCheck   bool          `json:"skip_prompt_check,omitempty"`
	AllowNoSourceHits bool          `json:"allow_no_source_hits,omitempty"`
}

type AnswerReport struct {
	Suite                   string             `json:"suite"`
	Description             string             `json:"description,omitempty"`
	Passed                  bool               `json:"passed"`
	CaseCount               int                `json:"case_count"`
	PassedCount             int                `json:"passed_count"`
	FailedCount             int                `json:"failed_count"`
	DurationMS              int64              `json:"duration_ms"`
	P95LatencyMS            int64              `json:"p95_latency_ms"`
	SourceRecall            float64            `json:"source_recall"`
	MatchedExpectedSources  int                `json:"matched_expected_sources"`
	TotalExpectedSources    int                `json:"total_expected_sources"`
	CitationPassedCount     int                `json:"citation_passed_count"`
	RefusalPassedCount      int                `json:"refusal_passed_count,omitempty"`
	RefusalCaseCount        int                `json:"refusal_case_count,omitempty"`
	HistoryPassedCount      int                `json:"history_passed_count"`
	PromptContextPassCount  int                `json:"prompt_context_pass_count"`
	ForbiddenPhraseHitCount int                `json:"forbidden_phrase_hit_count,omitempty"`
	Embedding               RetrievalEmbedding `json:"embedding"`
	IndexedChunks           int                `json:"indexed_chunks"`
	CaseReports             []AnswerCaseReport `json:"cases"`
}

type AnswerCaseReport struct {
	ID                    string             `json:"id"`
	Question              string             `json:"question"`
	Passed                bool               `json:"passed"`
	Failures              []string           `json:"failures,omitempty"`
	LatencyMS             int64              `json:"latency_ms"`
	Answer                string             `json:"answer"`
	SourceHitCount        int                `json:"source_hit_count"`
	ExpectedSourceMatched int                `json:"expected_source_matched"`
	ExpectedSourceTotal   int                `json:"expected_source_total"`
	ExpectedSourceRanks   []int              `json:"expected_source_ranks,omitempty"`
	CitationMatched       int                `json:"citation_matched"`
	CitationTotal         int                `json:"citation_total"`
	RequiredPhraseMatched int                `json:"required_phrase_matched"`
	RequiredPhraseTotal   int                `json:"required_phrase_total"`
	ForbiddenPhraseHits   []string           `json:"forbidden_phrase_hits,omitempty"`
	RefusalExpected       bool               `json:"refusal_expected,omitempty"`
	RefusalPassed         bool               `json:"refusal_passed,omitempty"`
	HistoryPassed         bool               `json:"history_passed"`
	PromptContextPassed   bool               `json:"prompt_context_passed"`
	ModelTarget           string             `json:"model_target"`
	TopSources            []RetrievalHitView `json:"top_sources"`
}

func LoadAnswerSuite(reader io.Reader) (AnswerSuite, error) {
	var suite AnswerSuite
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&suite); err != nil {
		return AnswerSuite{}, fmt.Errorf("decode answer suite: %w", err)
	}
	if err := validateAnswerSuite(suite); err != nil {
		return AnswerSuite{}, err
	}
	return suite, nil
}

func RunAnswerSuite(ctx context.Context, suite AnswerSuite) (AnswerReport, error) {
	if err := validateAnswerSuite(suite); err != nil {
		return AnswerReport{}, err
	}
	startedAt := time.Now()
	tenantID := suite.TenantID
	if strings.TrimSpace(tenantID) == "" {
		tenantID = defaultEvalTenantID
	}
	ownerID := suite.OwnerID
	if strings.TrimSpace(ownerID) == "" {
		ownerID = defaultAnswerOwnerID
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
	vectors, err := suiteVectors(ctx, RetrievalSuite{Documents: suite.Documents}, tenantID, embedder)
	if err != nil {
		return AnswerReport{}, err
	}
	if err := vectorIndex.Upsert(ctx, vectors); err != nil {
		return AnswerReport{}, err
	}

	repos := storememory.New()
	gateway := &scriptedAnswerGateway{}
	search := app.NewSearchService(embedder, vectorIndex)
	service := app.NewConversationService(repos, &answerEvalIDs{}, answerEvalClock{}, search, gateway)
	report := AnswerReport{
		Suite:       suite.Name,
		Description: suite.Description,
		Embedding: RetrievalEmbedding{
			Model:      model,
			Dimensions: dimensions,
		},
		CaseCount:     len(suite.Cases),
		CaseReports:   make([]AnswerCaseReport, 0, len(suite.Cases)),
		IndexedChunks: len(vectors),
	}

	for _, testCase := range suite.Cases {
		gateway.next = providers.ChatCompletion{
			Model:        "eval-answer-model",
			Content:      testCase.Answer,
			FinishReason: "stop",
		}
		caseReport, err := runAnswerCase(ctx, service, repos, gateway, domain.TenantID(tenantID), domain.UserID(ownerID), testCase)
		if err != nil {
			return AnswerReport{}, err
		}
		report.CaseReports = append(report.CaseReports, caseReport)
		if caseReport.Passed {
			report.PassedCount++
		}
	}
	answerReportMetrics(&report)
	report.FailedCount = report.CaseCount - report.PassedCount
	report.Passed = report.FailedCount == 0
	report.DurationMS = time.Since(startedAt).Milliseconds()
	return report, nil
}

func runAnswerCase(ctx context.Context, service app.ConversationService, repos answerCaseStore, gateway *scriptedAnswerGateway, tenantID domain.TenantID, ownerID domain.UserID, testCase AnswerCase) (AnswerCaseReport, error) {
	limit := testCase.Limit
	if limit <= 0 {
		limit = defaultRetrievalLimit
	}
	modelTarget := strings.TrimSpace(testCase.ModelTarget)
	if modelTarget == "" {
		modelTarget = defaultAnswerTarget
	}
	strategy, err := app.NormalizeSearchStrategy(app.SearchStrategy(testCase.Strategy))
	if err != nil {
		return AnswerCaseReport{}, fmt.Errorf("answer case %q strategy: %w", testCase.ID, err)
	}
	conversationID, err := seedAnswerHistory(ctx, repos, tenantID, ownerID, modelTarget, testCase)
	if err != nil {
		return AnswerCaseReport{}, err
	}
	startedAt := time.Now()
	result, err := service.Ask(ctx, app.AskInput{
		TenantID:       tenantID,
		OwnerID:        ownerID,
		ConversationID: conversationID,
		DocumentID:     domain.DocumentID(testCase.DocumentID),
		ModelTarget:    modelTarget,
		Question:       testCase.Question,
		Limit:          limit,
		Strategy:       strategy,
	})
	if err != nil {
		return AnswerCaseReport{}, err
	}
	latencyMS := time.Since(startedAt).Milliseconds()
	report := AnswerCaseReport{
		ID:                  testCase.ID,
		Question:            testCase.Question,
		LatencyMS:           latencyMS,
		Answer:              result.AssistantMessage.Content,
		SourceHitCount:      len(result.Hits),
		ExpectedSourceTotal: len(testCase.Expected.ExpectedSources),
		CitationTotal:       len(testCase.Expected.RequireCitations),
		RequiredPhraseTotal: len(testCase.Expected.RequirePhrases),
		RefusalExpected:     testCase.Expected.RequireRefusal,
		ModelTarget:         result.Conversation.ModelTarget,
		TopSources:          hitViews(result.Hits),
	}
	report.ExpectedSourceMatched, report.ExpectedSourceRanks = expectedSourceMetrics(result.Hits, testCase.Expected)
	report.Failures = append(report.Failures, sourceFailures(report, testCase.Expected)...)
	report.CitationMatched = countContainedPhrases(result.AssistantMessage.Content, testCase.Expected.RequireCitations)
	if report.CitationMatched < report.CitationTotal {
		report.Failures = append(report.Failures, fmt.Sprintf("citations matched %d/%d", report.CitationMatched, report.CitationTotal))
	}
	report.RequiredPhraseMatched = countContainedPhrases(result.AssistantMessage.Content, testCase.Expected.RequirePhrases)
	if report.RequiredPhraseMatched < report.RequiredPhraseTotal {
		report.Failures = append(report.Failures, fmt.Sprintf("required phrases matched %d/%d", report.RequiredPhraseMatched, report.RequiredPhraseTotal))
	}
	report.ForbiddenPhraseHits = containedPhrases(result.AssistantMessage.Content, testCase.Expected.ForbiddenPhrases)
	for _, phrase := range report.ForbiddenPhraseHits {
		report.Failures = append(report.Failures, fmt.Sprintf("forbidden phrase present: %q", phrase))
	}
	if testCase.Expected.RequireRefusal {
		report.RefusalPassed = containsRefusalLanguage(result.AssistantMessage.Content)
		if !report.RefusalPassed {
			report.Failures = append(report.Failures, "refusal language missing")
		}
	}
	if testCase.Expected.MaxLatencyMS > 0 && latencyMS > testCase.Expected.MaxLatencyMS {
		report.Failures = append(report.Failures, fmt.Sprintf("latency %dms exceeded %dms", latencyMS, testCase.Expected.MaxLatencyMS))
	}
	report.HistoryPassed = true
	if !testCase.Expected.SkipHistoryCheck {
		messages, err := repos.ListMessages(ctx, result.Conversation.TenantID, result.Conversation.ID)
		if err != nil {
			return AnswerCaseReport{}, err
		}
		report.HistoryPassed = answerHistoryPassed(messages, testCase, result.AssistantMessage.Content)
		if !report.HistoryPassed {
			report.Failures = append(report.Failures, "conversation history did not preserve seeded history plus user/assistant turn")
		}
	}
	report.PromptContextPassed = true
	if !testCase.Expected.SkipPromptCheck {
		report.PromptContextPassed = promptContainsExpectedContext(gateway.lastRequest, result.Hits)
		if !report.PromptContextPassed {
			report.Failures = append(report.Failures, "model prompt did not include expected retrieved context")
		}
	}
	report.Passed = len(report.Failures) == 0
	return report, nil
}

type answerCaseStore interface {
	SaveConversation(ctx context.Context, conversation domain.Conversation) error
	SaveMessage(ctx context.Context, message domain.Message) error
	ListMessages(ctx context.Context, tenantID domain.TenantID, conversationID domain.ConversationID) ([]domain.Message, error)
}

func seedAnswerHistory(ctx context.Context, repos answerCaseStore, tenantID domain.TenantID, ownerID domain.UserID, modelTarget string, testCase AnswerCase) (domain.ConversationID, error) {
	if len(testCase.History) == 0 {
		return "", nil
	}
	conversationID := domain.ConversationID("conv_history_" + sanitizeEvalID(testCase.ID))
	startedAt := answerEvalClock{}.Now().Add(-time.Duration(len(testCase.History)+1) * time.Second)
	conversation, err := domain.NewConversation(domain.ConversationCreate{
		ID:          conversationID,
		TenantID:    tenantID,
		OwnerID:     ownerID,
		Title:       testCase.ID,
		ModelTarget: modelTarget,
		Now:         startedAt,
	})
	if err != nil {
		return "", err
	}
	if err := repos.SaveConversation(ctx, conversation); err != nil {
		return "", err
	}
	for index, item := range testCase.History {
		message, err := domain.NewMessage(domain.MessageCreate{
			ID:             domain.MessageID(fmt.Sprintf("msg_history_%s_%03d", sanitizeEvalID(testCase.ID), index+1)),
			TenantID:       tenantID,
			ConversationID: conversationID,
			Role:           domain.MessageRole(strings.TrimSpace(item.Role)),
			Content:        item.Content,
			Now:            startedAt.Add(time.Duration(index+1) * time.Second),
		})
		if err != nil {
			return "", err
		}
		if err := repos.SaveMessage(ctx, message); err != nil {
			return "", err
		}
	}
	return conversationID, nil
}

func answerHistoryPassed(messages []domain.Message, testCase AnswerCase, answer string) bool {
	expectedLen := len(testCase.History) + 2
	if len(messages) != expectedLen {
		return false
	}
	for index, expected := range testCase.History {
		if string(messages[index].Role) != strings.TrimSpace(expected.Role) ||
			messages[index].Content != strings.TrimSpace(expected.Content) {
			return false
		}
	}
	return messages[expectedLen-2].Role == domain.MessageRoleUser &&
		messages[expectedLen-2].Content == testCase.Question &&
		messages[expectedLen-1].Role == domain.MessageRoleAssistant &&
		messages[expectedLen-1].Content == answer
}

func sanitizeEvalID(id string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(id)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "case"
	}
	return builder.String()
}

func expectedSourceMetrics(hits []providers.VectorHit, expectation AnswerExpectation) (int, []int) {
	if len(expectation.ExpectedSources) == 0 {
		return 0, nil
	}
	maxRank := expectation.MaxSourceRank
	if maxRank <= 0 {
		maxRank = defaultRetrievalLimit
	}
	matched := 0
	ranks := make([]int, 0, len(expectation.ExpectedSources))
	for _, expected := range expectation.ExpectedSources {
		rank, ok := expectedHitRank(hits, expected, maxRank)
		if ok {
			matched++
			ranks = append(ranks, rank)
		}
	}
	return matched, ranks
}

func sourceFailures(report AnswerCaseReport, expectation AnswerExpectation) []string {
	failures := make([]string, 0)
	if expectation.MinSources > 0 && report.SourceHitCount < expectation.MinSources {
		failures = append(failures, fmt.Sprintf("source hits %d below minimum %d", report.SourceHitCount, expectation.MinSources))
	}
	if len(expectation.ExpectedSources) > 0 && report.ExpectedSourceMatched < len(expectation.ExpectedSources) {
		maxRank := expectation.MaxSourceRank
		if maxRank <= 0 {
			maxRank = defaultRetrievalLimit
		}
		failures = append(failures, fmt.Sprintf("expected sources matched %d/%d within rank %d", report.ExpectedSourceMatched, len(expectation.ExpectedSources), maxRank))
	}
	if report.SourceHitCount == 0 && !expectation.AllowNoSourceHits && !expectation.RequireRefusal {
		failures = append(failures, "no source hits returned")
	}
	return failures
}

func answerReportMetrics(report *AnswerReport) {
	latencies := make([]int64, 0, len(report.CaseReports))
	for _, testCase := range report.CaseReports {
		latencies = append(latencies, testCase.LatencyMS)
		report.MatchedExpectedSources += testCase.ExpectedSourceMatched
		report.TotalExpectedSources += testCase.ExpectedSourceTotal
		if testCase.CitationTotal == 0 || testCase.CitationMatched == testCase.CitationTotal {
			report.CitationPassedCount++
		}
		if testCase.RefusalExpected {
			report.RefusalCaseCount++
			if testCase.RefusalPassed {
				report.RefusalPassedCount++
			}
		}
		if testCase.HistoryPassed {
			report.HistoryPassedCount++
		}
		if testCase.PromptContextPassed {
			report.PromptContextPassCount++
		}
		report.ForbiddenPhraseHitCount += len(testCase.ForbiddenPhraseHits)
	}
	if report.TotalExpectedSources > 0 {
		report.SourceRecall = float64(report.MatchedExpectedSources) / float64(report.TotalExpectedSources)
	}
	report.P95LatencyMS = percentileLatency(latencies, 95)
}

func countContainedPhrases(text string, phrases []string) int {
	return len(containedPhrases(text, phrases))
}

func containedPhrases(text string, phrases []string) []string {
	normalized := strings.ToLower(text)
	matches := make([]string, 0)
	for _, phrase := range phrases {
		trimmed := strings.TrimSpace(phrase)
		if trimmed == "" {
			continue
		}
		if strings.Contains(normalized, strings.ToLower(trimmed)) {
			matches = append(matches, phrase)
		}
	}
	return matches
}

func containsRefusalLanguage(text string) bool {
	return countContainedPhrases(text, []string{
		"not enough context",
		"do not have enough context",
		"don't have enough context",
		"missing context",
		"cannot determine",
		"can't determine",
	}) > 0
}

func promptContainsExpectedContext(request providers.ChatCompletionRequest, hits []providers.VectorHit) bool {
	if len(request.Messages) == 0 {
		return false
	}
	last := request.Messages[len(request.Messages)-1].Content
	if !strings.Contains(last, "Retrieved context:") {
		return false
	}
	if len(hits) == 0 {
		return strings.Contains(last, "No relevant document chunks were retrieved.")
	}
	for _, hit := range hits {
		if !strings.Contains(last, string(hit.DocumentID)) || !strings.Contains(last, hit.ChunkID) {
			return false
		}
	}
	return true
}

func validateAnswerSuite(suite AnswerSuite) error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("answer suite name is required")
	}
	if len(suite.Documents) == 0 {
		return fmt.Errorf("answer suite needs at least one document")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("answer suite needs at least one case")
	}
	documentIDs := map[string]bool{}
	chunkIDs := map[string]map[string]bool{}
	for _, document := range suite.Documents {
		if strings.TrimSpace(document.ID) == "" || strings.TrimSpace(document.Name) == "" {
			return fmt.Errorf("answer document id and name are required")
		}
		if documentIDs[document.ID] {
			return fmt.Errorf("duplicate document id %q", document.ID)
		}
		documentIDs[document.ID] = true
		chunkIDs[document.ID] = map[string]bool{}
		for _, chunk := range document.Chunks {
			if strings.TrimSpace(chunk.ID) == "" || strings.TrimSpace(chunk.Text) == "" {
				return fmt.Errorf("answer document %q has a chunk without id or text", document.ID)
			}
			if chunkIDs[document.ID][chunk.ID] {
				return fmt.Errorf("duplicate chunk id %q in document %q", chunk.ID, document.ID)
			}
			chunkIDs[document.ID][chunk.ID] = true
		}
	}
	for _, testCase := range suite.Cases {
		if strings.TrimSpace(testCase.ID) == "" || strings.TrimSpace(testCase.Question) == "" {
			return fmt.Errorf("answer case id and question are required")
		}
		if strings.TrimSpace(testCase.Answer) == "" {
			return fmt.Errorf("answer case %q needs a scripted answer", testCase.ID)
		}
		if _, err := app.NormalizeSearchStrategy(app.SearchStrategy(testCase.Strategy)); err != nil {
			return fmt.Errorf("answer case %q has unknown strategy %q", testCase.ID, testCase.Strategy)
		}
		for index, item := range testCase.History {
			role := domain.MessageRole(strings.TrimSpace(item.Role))
			if !role.Valid() || strings.TrimSpace(item.Content) == "" {
				return fmt.Errorf("answer case %q has invalid history item %d", testCase.ID, index+1)
			}
		}
		for _, expected := range testCase.Expected.ExpectedSources {
			if !documentIDs[expected.DocumentID] {
				return fmt.Errorf("answer case %q expects unknown document %q", testCase.ID, expected.DocumentID)
			}
			if expected.ChunkID != "" && !chunkIDs[expected.DocumentID][expected.ChunkID] {
				return fmt.Errorf("answer case %q expects unknown chunk %q in document %q", testCase.ID, expected.ChunkID, expected.DocumentID)
			}
		}
	}
	return nil
}

type scriptedAnswerGateway struct {
	next        providers.ChatCompletion
	lastRequest providers.ChatCompletionRequest
}

func (g *scriptedAnswerGateway) Complete(ctx context.Context, input providers.ChatCompletionRequest) (providers.ChatCompletion, error) {
	if err := ctx.Err(); err != nil {
		return providers.ChatCompletion{}, err
	}
	g.lastRequest = input
	return g.next, nil
}

type answerEvalIDs struct {
	conversation int
	message      int
}

func (i *answerEvalIDs) NewConversationID() domain.ConversationID {
	i.conversation++
	return domain.ConversationID(fmt.Sprintf("conv_eval_%03d", i.conversation))
}

func (i *answerEvalIDs) NewMessageID() domain.MessageID {
	i.message++
	return domain.MessageID(fmt.Sprintf("msg_eval_%03d", i.message))
}

type answerEvalClock struct{}

func (answerEvalClock) Now() time.Time {
	return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
}

func SortAnswerCaseReports(reports []AnswerCaseReport) {
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ID < reports[j].ID
	})
}
