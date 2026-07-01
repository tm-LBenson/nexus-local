package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
)

var ErrSearchUnavailable = errors.New("search pipeline is not configured")

const (
	defaultSearchLimit = 5
	maxSearchLimit     = 20
)

type SearchStrategy string

const (
	SearchStrategyVector SearchStrategy = "vector"
	SearchStrategyHybrid SearchStrategy = "hybrid"
)

type SearchService struct {
	embedder providers.Embedder
	vectors  providers.VectorIndex
}

type SearchInput struct {
	TenantID   domain.TenantID
	DocumentID domain.DocumentID
	Query      string
	Limit      int
	Filters    map[string]string
	Strategy   SearchStrategy
}

type SearchResult struct {
	Hits []providers.VectorHit
}

func NewSearchService(embedder providers.Embedder, vectors providers.VectorIndex) SearchService {
	return SearchService{
		embedder: embedder,
		vectors:  vectors,
	}
}

func (s SearchService) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if s.embedder == nil || s.vectors == nil {
		return SearchResult{}, ErrSearchUnavailable
	}

	query := strings.TrimSpace(input.Query)
	if strings.TrimSpace(string(input.TenantID)) == "" || query == "" {
		return SearchResult{}, fmt.Errorf("search: %w", domain.ErrInvalidEntity)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	strategy, err := NormalizeSearchStrategy(input.Strategy)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search: %w", err)
	}

	embedded, err := s.embedder.Embed(ctx, providers.EmbeddingRequest{Texts: []string{query}})
	if err != nil {
		return SearchResult{}, err
	}
	if len(embedded.Vectors) != 1 {
		return SearchResult{}, fmt.Errorf("embedding count %d does not match query count 1", len(embedded.Vectors))
	}

	hits, err := s.vectors.Search(ctx, providers.VectorSearch{
		TenantID:   input.TenantID,
		DocumentID: input.DocumentID,
		Query:      embedded.Vectors[0],
		Limit:      searchCandidateLimit(limit, strategy),
		Filters:    input.Filters,
	})
	if err != nil {
		return SearchResult{}, err
	}
	if strategy == SearchStrategyHybrid {
		hits = rerankHybrid(query, hits)
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return SearchResult{Hits: hits}, nil
}

func NormalizeSearchStrategy(strategy SearchStrategy) (SearchStrategy, error) {
	switch SearchStrategy(strings.ToLower(strings.TrimSpace(string(strategy)))) {
	case "":
		return SearchStrategyVector, nil
	case SearchStrategyVector:
		return SearchStrategyVector, nil
	case SearchStrategyHybrid:
		return SearchStrategyHybrid, nil
	default:
		return "", domain.ErrInvalidEntity
	}
}

func searchCandidateLimit(limit int, strategy SearchStrategy) int {
	if strategy != SearchStrategyHybrid {
		return limit
	}
	candidateLimit := limit * 4
	if candidateLimit < limit {
		candidateLimit = limit
	}
	if candidateLimit > maxSearchLimit {
		candidateLimit = maxSearchLimit
	}
	return candidateLimit
}

type hybridCandidate struct {
	hit         providers.VectorHit
	vectorScore float32
	lexical     float32
	score       float32
	index       int
}

func rerankHybrid(query string, hits []providers.VectorHit) []providers.VectorHit {
	if len(hits) <= 1 {
		return markHybridHits(query, hits)
	}
	candidates := make([]hybridCandidate, 0, len(hits))
	for index, hit := range hits {
		lexical := lexicalScore(query, hit)
		vectorScore := normalizeVectorScore(hit.Score)
		score := (0.55 * lexical) + (0.45 * vectorScore)
		hit.Metadata = cloneMetadata(hit.Metadata)
		hit.Metadata["retrieval_strategy"] = string(SearchStrategyHybrid)
		hit.Metadata["vector_score"] = formatScore(hit.Score)
		hit.Metadata["lexical_score"] = formatScore(lexical)
		hit.Score = score
		candidates = append(candidates, hybridCandidate{
			hit:         hit,
			vectorScore: vectorScore,
			lexical:     lexical,
			score:       score,
			index:       index,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if left.lexical != right.lexical {
			return left.lexical > right.lexical
		}
		if left.vectorScore != right.vectorScore {
			return left.vectorScore > right.vectorScore
		}
		return left.index < right.index
	})
	reranked := make([]providers.VectorHit, 0, len(candidates))
	for _, candidate := range candidates {
		reranked = append(reranked, candidate.hit)
	}
	return reranked
}

func markHybridHits(query string, hits []providers.VectorHit) []providers.VectorHit {
	marked := make([]providers.VectorHit, 0, len(hits))
	for _, hit := range hits {
		lexical := lexicalScore(query, hit)
		vectorScore := normalizeVectorScore(hit.Score)
		hit.Metadata = cloneMetadata(hit.Metadata)
		hit.Metadata["retrieval_strategy"] = string(SearchStrategyHybrid)
		hit.Metadata["vector_score"] = formatScore(hit.Score)
		hit.Metadata["lexical_score"] = formatScore(lexical)
		hit.Score = (0.55 * lexical) + (0.45 * vectorScore)
		marked = append(marked, hit)
	}
	return marked
}

func lexicalScore(query string, hit providers.VectorHit) float32 {
	queryTerms := termCounts(query)
	if len(queryTerms) == 0 {
		return 0
	}
	text := hit.Text
	for _, value := range hit.Metadata {
		text += " " + value
	}
	textTerms := termCounts(text)
	matched := 0
	total := 0
	for term, count := range queryTerms {
		total += count
		if textTerms[term] == 0 {
			continue
		}
		if textTerms[term] >= count {
			matched += count
		} else {
			matched += textTerms[term]
		}
	}
	if total == 0 {
		return 0
	}
	score := float32(matched) / float32(total)
	normalizedQuery := strings.Join(tokenize(query), " ")
	normalizedText := strings.Join(tokenize(text), " ")
	if normalizedQuery != "" && strings.Contains(normalizedText, normalizedQuery) {
		score += 0.15
	}
	if score > 1 {
		return 1
	}
	return score
}

func termCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, token := range tokenize(text) {
		counts[token]++
	}
	return counts
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || searchStopWords[field] {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

var searchStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "how": true, "i": true,
	"in": true, "into": true, "is": true, "it": true, "of": true, "on": true,
	"or": true, "that": true, "the": true, "this": true, "to": true, "what": true,
	"when": true, "where": true, "with": true,
}

func normalizeVectorScore(score float32) float32 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := map[string]string{}
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func formatScore(score float32) string {
	return strconv.FormatFloat(float64(score), 'f', 4, 32)
}
