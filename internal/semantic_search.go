package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/lmittmann/tint"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
	"google.golang.org/protobuf/types/known/structpb"
)

// HybridSearchParams provides configuration options for hybrid semantic search
// The ScoreFusion parameter allows users to choose how dense and sparse scores are combined.
//
// The following fusion methods are supported:
//
// Linear fusion (method="linear"):
//   - Simple weighted average of dense and sparse scores
//   - Best when both signals are reliable and complementary
//   - Allows fine-tuning through weight adjustment
//   - Most commonly used as a baseline approach
//
// Maximum fusion (method="max"):
//   - Takes the highest score between dense and sparse
//   - Useful when one signal is expected to be more reliable but we don't know which
//   - Good for queries that might be either semantic or keyword-focused
//   - Can help reduce false negatives but may increase false positives
//
// Minimum fusion (method="min"):
//   - Takes the lowest score between dense and sparse
//   - Conservative approach requiring agreement between both signals
//   - Useful for high-precision requirements where false positives are costly
//   - Can help reduce false positives but may increase false negatives
//
// Harmonic mean fusion (method="harmonic"):
//   - Weighted harmonic mean of dense and sparse scores
//   - Penalizes large differences between scores
//   - Best when both signals should agree for high relevance
//   - More conservative than linear fusion
//   - Good for technical job matching where both semantic and keyword match matter
//
// Reciprocal Rank Fusion (method="reciprocal_rank_fusion"):
//   - Based on rank position rather than raw scores
//   - Robust to score scaling differences between methods
//   - Handles outliers well
//   - Good for merging results from very different scoring approaches
//   - Uses constant k=60 to prevent high ranks from dominating
type HybridSearchParams struct {
	DenseWeight     float64 // Weight for dense results (0.0-1.0)
	SparseWeight    float64 // Weight for sparse results (0.0-1.0)
	ScoreFusion     string  // "linear", "max", "min", "harmonic", "reciprocal_rank_fusion"
	RecencyWeight   float64 // Weight to apply to recency boosting (0.0-1.0)
	RecencyHalfLife float64 // Half-life in days for recency decay
	ScoreThreshold  float64 // Minimum score to consider a result relevant (0.0-1.0)
}

// DefaultHybridSearchParams returns a set of default parameters for hybrid search
func DefaultHybridSearchParams() *HybridSearchParams {
	return &HybridSearchParams{
		DenseWeight:     0.7,
		SparseWeight:    0.3,
		ScoreFusion:     "linear",
		RecencyWeight:   0.2,
		RecencyHalfLife: 30.0,
		ScoreThreshold:  0.5,
	}
}

type ScoredJobDetail struct {
	JobDetail   JobDetail
	DenseScore  float64
	SparseScore float64
	Score       float64
}

func (s *ScoredJobDetail) ToPineconeDocument() pinecone.Document {
	// pinecone.Document is a map[string]any
	return pinecone.Document{
		"_id":           s.JobDetail.ID,
		"description":   s.JobDetail.Description,
		"provider":      s.JobDetail.Provider,
		"providerID":    s.JobDetail.ProviderJobID,
		"title":         s.JobDetail.Title,
		"company":       s.JobDetail.Company,
		"location":      s.JobDetail.Location,
		"url":           s.JobDetail.URL,
		"postedDate":    s.JobDetail.PostedDate,
		"denseScore":    s.DenseScore,
		"sparseScore":   s.SparseScore,
		"combinedScore": s.Score,
	}
}

// SearchResult holds raw search results from Pinecone
type SearchResult struct {
	DenseResults  *pinecone.QueryVectorsResponse
	SparseResults *pinecone.SearchRecordsResponse
}

// Validate ensures the parameters are valid and adjusts if necessary
func (p *HybridSearchParams) Validate() *HybridSearchParams {
	// Default to linear fusion if not specified or invalid
	validFusions := map[string]bool{
		"linear":                 true,
		"max":                    true,
		"min":                    true,
		"harmonic":               true,
		"reciprocal_rank_fusion": true,
	}

	if !validFusions[p.ScoreFusion] {
		p.ScoreFusion = "linear"
	}

	// Ensure weights are between 0 and 1
	if p.DenseWeight < 0 {
		p.DenseWeight = 0
	}
	if p.DenseWeight > 1 {
		p.DenseWeight = 1
	}

	if p.SparseWeight < 0 {
		p.SparseWeight = 0
	}
	if p.SparseWeight > 1 {
		p.SparseWeight = 1
	}

	// Normalize weights to sum to 1.0
	sum := p.DenseWeight + p.SparseWeight
	if sum > 0 {
		p.DenseWeight /= sum
		p.SparseWeight /= sum
	} else {
		// If both weights are 0, default to equal weights
		p.DenseWeight = 0.5
		p.SparseWeight = 0.5
	}

	// Ensure recency weight is between 0 and 1
	if p.RecencyWeight < 0 {
		p.RecencyWeight = 0
	}
	if p.RecencyWeight > 1 {
		p.RecencyWeight = 1
	}

	// Ensure half-life is positive
	if p.RecencyHalfLife <= 0 {
		p.RecencyHalfLife = 30.0
	}

	return p
}

// SemanticSearchJobs performs a hybrid search combining dense and sparse vectors
// to find the most relevant job listings based on the query.
//
// Parameters:
//   - ctx: The context for the operation
//   - steeringQuery: The query to steer the search
//   - criteriaID: Optional criteria ID to filter results
//   - topK: Maximum number of results to return
//   - params: Optional search parameters to customize the search behavior
//
// The function performs the following steps:
//  1. Retrieves both dense and sparse vectors for the query
//  2. Searches both dense and sparse indices
//  3. Combines and normalizes scores using the specified fusion strategy
//  4. Applies recency boosting if dates are available
//  5. Deduplicates and sorts results
//
// If params is nil, default parameters will be used.
func SemanticSearchJobs(ctx context.Context, steeringQuery *SteeringQuery, criteriaID string, topK int, params *HybridSearchParams) ([]ScoredJobDetail, error) {
	if params == nil {
		params = DefaultHybridSearchParams()
	} else {
		params = params.Validate()
	}

	client, err := NewPineconeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
	}

	// Build filter map
	filter := map[string]any{}
	if criteriaID != "" {
		filter["criteriaID"] = map[string]any{
			"$eq": criteriaID,
		}
	}

	metaFilter, err := structpb.NewStruct(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to create filter struct: %w", err)
	}

	denseVector, err := steeringQuery.GetDenseVector(ctx, client, PineconeEmbedDense)
	if err != nil {
		return nil, fmt.Errorf("failed to get dense steering vector: %w", err)
	}

	denseIndex := os.Getenv("PINECONE_DENSE_INDEX_HOST")
	if denseIndex == "" {
		return nil, fmt.Errorf("PINECONE_DENSE_INDEX_HOST environment variable not set")
	}

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))
	logger = logger.With("pkg", "internal.SemanticSearchJobs")

	denseResults, err := client.QueryByVector(ctx, denseIndex, JobDetailNamespace, denseVector,
		WithTopK(topK*2), WithMetadataFilter(metaFilter))
	if err != nil {
		return nil, fmt.Errorf("failed to search dense index: %w", err)
	}
	logger.Debug("dense search results", "count", len(denseResults.Matches))

	sparseIndex := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
	if sparseIndex == "" {
		return nil, fmt.Errorf("PINECONE_SPARSE_INDEX_HOST environment variable not set")
	}

	query := steeringQuery.Query
	if steeringQuery.MoveToward != nil {
		query += " " + *steeringQuery.MoveToward
	}
	sparseResults, err := client.SearchRecords(ctx, sparseIndex, JobDetailNamespace, query,
		WithTopK(topK*2), WithFilter(filter), WithFields([]string{"ID"}))
	if err != nil {
		return nil, fmt.Errorf("failed to search sparse index: %w", err)
	}
	logger.Debug("sparse search results", "count", len(sparseResults.Result.Hits))

	searchResult := &SearchResult{
		DenseResults:  denseResults,
		SparseResults: sparseResults,
	}

	return processSearchResults(ctx, logger, searchResult, topK, params)
}

// RerankResults applies reranking to a list of scored job details using the Pinecone reranker.
// This is extracted to make testing easier.
func RerankResults(ctx context.Context, client *Pinecone, scoredMatches []*ScoredJobDetail, rerankQuery string, topK int, model PineconeRerankModel) ([]ScoredJobDetail, error) {
	rerankParams := map[string]any{
		"truncate": "END",
	}

	// Convert to Documents for reranking
	documents := make([]pinecone.Document, 0, len(scoredMatches))
	for _, match := range scoredMatches {
		if match.JobDetail.Description == "" {
			continue
		}

		// Convert metadata to Document
		doc := match.ToPineconeDocument()

		documents = append(documents, doc)
	}

	rerankResponse, err := client.Inference.Rerank(ctx, &pinecone.RerankRequest{
		Model:      string(model),
		Query:      rerankQuery,
		Documents:  documents,
		RankFields: &[]string{"text"},
		TopN:       &topK,
		Parameters: &rerankParams,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rerank jobs: %w", err)
	}

	// Create final results from reranker output
	results := make([]ScoredJobDetail, 0, len(rerankResponse.Data))
	for _, result := range rerankResponse.Data {
		doc := *result.Document // Document is a map[string]any

		// Parse posted date if available
		var postedDate time.Time
		if dateStr, ok := doc["postedDate"].(string); ok {
			postedDate, _ = time.Parse(time.RFC3339, dateStr)
		}

		// Create job detail from document
		jobDetail := JobDetail{
			ID:          doc["ID"].(string),
			Title:       safeGetString(doc, "title"),
			Company:     safeGetString(doc, "company"),
			Location:    safeGetString(doc, "location"),
			Description: safeGetString(doc, "text"),
			URL:         safeGetString(doc, "url"),
			PostedDate:  postedDate,
		}

		results = append(results, ScoredJobDetail{
			JobDetail: jobDetail,
			Score:     float64(result.Score),
		})
	}

	return results, nil
}

// processSearchResults processes raw search results from Pinecone and applies fusion scoring,
// normalization, recency boosting, deduplication, and sorting.
// The client parameter is optional and only needed when reranking is specified.
func processSearchResults(_ context.Context, logger *slog.Logger, searchResult *SearchResult, topK int, params *HybridSearchParams) ([]ScoredJobDetail, error) {
	// Create maps to track scores from each source
	denseScores := make(map[string]float32)
	sparseScores := make(map[string]float32)
	allMatches := make(map[string]*ScoredJobDetail)

	// Process dense results
	for _, hit := range searchResult.DenseResults.Matches {
		id := hit.Vector.Id
		meta := hit.Vector.Metadata
		if meta == nil {
			continue // skip if no metadata
		}

		denseScores[id] = hit.Score

		// Parse posted date if available
		var postedDate time.Time
		if dateStr := meta.Fields["postedDate"].GetStringValue(); dateStr != "" {
			// the postedDate is of the format postedDate: "2025-05-11T23:52:36.312383Z"
			postedDate, _ = time.Parse(time.RFC3339, dateStr)
		}

		match, exists := allMatches[id]
		if !exists {
			match = &ScoredJobDetail{
				JobDetail: JobDetail{
					ID:            id,
					Provider:      meta.Fields["provider"].GetStringValue(),
					ProviderJobID: meta.Fields["providerID"].GetStringValue(),
					Title:         meta.Fields["title"].GetStringValue(),
					Company:       meta.Fields["company"].GetStringValue(),
					Location:      meta.Fields["location"].GetStringValue(),
					URL:           meta.Fields["url"].GetStringValue(),
					PostedDate:    postedDate,
					Description:   meta.Fields["text"].GetStringValue(),
				},
				DenseScore:  float64(hit.Score),
				SparseScore: 0,
			}
			allMatches[id] = match
		} else {
			match.DenseScore = float64(hit.Score)
			// Keep existing metadata
		}
	}

	// Process sparse results
	for _, hit := range searchResult.SparseResults.Result.Hits {
		id := hit.Id

		sparseScores[id] = hit.Score

		// Parse posted date if available
		var postedDate time.Time
		if dateStr, ok := hit.Fields["postedDate"]; ok && dateStr != nil {
			if dateString, ok := dateStr.(string); ok {
				postedDate, _ = time.Parse(time.RFC3339, dateString)
			}
		}

		match, exists := allMatches[id]
		if !exists {
			match = &ScoredJobDetail{
				JobDetail: JobDetail{
					ID:            id,
					Provider:      safeGetString(hit.Fields, "provider"),
					ProviderJobID: safeGetString(hit.Fields, "providerJobID"),
					Title:         safeGetString(hit.Fields, "title"),
					Company:       safeGetString(hit.Fields, "company"),
					Location:      safeGetString(hit.Fields, "location"),
					URL:           safeGetString(hit.Fields, "url"),
					PostedDate:    postedDate,
					Description:   safeGetString(hit.Fields, "text"),
				},
				DenseScore:  0,
				SparseScore: float64(hit.Score),
			}
			allMatches[id] = match
		} else {
			match.SparseScore = float64(hit.Score)
		}
	}

	// Normalize scores if there are more than 1 item
	if len(denseScores) > 1 {
		normalizedDense := normalizeScores(denseScores)
		for id, score := range normalizedDense {
			if match, ok := allMatches[id]; ok {
				match.DenseScore = float64(score)
			}
		}
	}

	if len(sparseScores) > 1 {
		normalizedSparse := normalizeScores(sparseScores)
		for id, score := range normalizedSparse {
			if match, ok := allMatches[id]; ok {
				match.SparseScore = float64(score)
			}
		}
	}

	// Apply score fusion strategy and recency boosting
	for _, match := range allMatches {
		// Apply fusion based on selected strategy
		fused := fusionScore(match.DenseScore, match.SparseScore, params.ScoreFusion, params.DenseWeight, params.SparseWeight)

		// Apply recency boosting if we have a date
		if !match.JobDetail.PostedDate.IsZero() {
			// Log details about the posted date and recency calculation
			daysSince := time.Since(match.JobDetail.PostedDate).Hours() / 24.0
			decayFactor := math.Exp(-daysSince / params.RecencyHalfLife)

			// Calculate the boosted score separately for logging
			recencyBoosted := (1.0-params.RecencyWeight)*fused + params.RecencyWeight*decayFactor

			// Use a logger that doesn't affect production but works in tests
			// We'll use fmt.Printf in a way that only outputs during tests
			if _, file, _, ok := runtime.Caller(1); ok && strings.Contains(file, "_test.go") {
				fmt.Printf("ID=%s, Dense=%.4f, Sparse=%.4f, Fused=%.4f, PostedDate=%v, DaysSince=%.2f, DecayFactor=%.4f, Recency=%.4f, Final=%.4f\n",
					match.JobDetail.ID, match.DenseScore, match.SparseScore, fused,
					match.JobDetail.PostedDate.Format(time.RFC3339), daysSince,
					decayFactor, params.RecencyWeight, recencyBoosted)
			}

			fused = recencyBoost(fused, match.JobDetail.PostedDate, params.RecencyWeight, params.RecencyHalfLife)
		}

		match.Score = fused
	}

	// deduplicate documents by hash
	deduplicated := make(map[string]*ScoredJobDetail)
	for _, match := range allMatches {
		if match.JobDetail.Description == "" {
			continue
		}

		description := match.JobDetail.Description

		hash, err := utils.SHA256Hash([]byte(description))
		if err != nil {
			return nil, fmt.Errorf("failed to hash job detail: %w", err)
		}

		// Keep the match with the highest score if duplicates exist
		if existing, found := deduplicated[hash]; !found || match.Score > existing.Score {
			deduplicated[hash] = match
		}
	}

	logger.Debug("deduplicated matches", "from", len(allMatches), "to", len(deduplicated))

	// Create a sorted list of matches by combined score
	scoredMatches := make([]*ScoredJobDetail, 0, len(deduplicated))
	for _, match := range deduplicated {
		if match.Score < params.ScoreThreshold {
			continue // Skip matches below the score threshold
		}
		scoredMatches = append(scoredMatches, match)
	}

	logger.Debug("filtered matches", "from", len(deduplicated), "to", len(scoredMatches), "threshold", params.ScoreThreshold)

	// Sort by combined score in descending order
	sort.Slice(scoredMatches, func(i, j int) bool {
		return scoredMatches[i].Score > scoredMatches[j].Score
	})

	// Limit to topK
	if len(scoredMatches) > topK {
		scoredMatches = scoredMatches[:topK]
	}

	logger.Debug("final matches", "count", len(scoredMatches), "topK", topK)

	// Create final results from sorted matches
	results := make([]ScoredJobDetail, 0, len(scoredMatches))
	for _, match := range scoredMatches {
		if match.JobDetail.Description == "" {
			continue
		}

		results = append(results, *match)
	}

	return results, nil
}

// fusionScore combines dense and sparse scores using the specified method.
//
// The function supports several score fusion strategies, each with different characteristics:
//
// Linear fusion (method="linear"):
//   - Simple weighted average of dense and sparse scores
//   - Best when both signals are reliable and complementary
//   - Allows fine-tuning through weight adjustment
//   - Most commonly used as a baseline approach
//
// Maximum fusion (method="max"):
//   - Takes the highest score between dense and sparse
//   - Useful when one signal is expected to be more reliable but we don't know which
//   - Good for queries that might be either semantic or keyword-focused
//   - Can help reduce false negatives but may increase false positives
//
// Minimum fusion (method="min"):
//   - Takes the lowest score between dense and sparse
//   - Conservative approach requiring agreement between both signals
//   - Useful for high-precision requirements where false positives are costly
//   - Can help reduce false positives but may increase false negatives
//
// Harmonic mean fusion (method="harmonic"):
//   - Weighted harmonic mean of dense and sparse scores
//   - Penalizes large differences between scores
//   - Best when both signals should agree for high relevance
//   - More conservative than linear fusion
//   - Good for technical job matching where both semantic and keyword match matter
//
// Reciprocal Rank Fusion (method="reciprocal_rank_fusion"):
//   - Based on rank position rather than raw scores
//   - Robust to score scaling differences between methods
//   - Handles outliers well
//   - Good for merging results from very different scoring approaches
//   - Uses constant k=60 to prevent high ranks from dominating
//
// Parameters:
//   - denseScore: score from dense vector similarity (0.0-1.0)
//   - sparseScore: score from sparse vector similarity (0.0-1.0)
//   - method: fusion method to use (linear, max, min, harmonic, reciprocal_rank_fusion)
//   - denseWeight: weight for dense score (0.0-1.0)
//   - sparseWeight: weight for sparse score (0.0-1.0)
//
// Returns:
//
//	Combined score (0.0-1.0). If method is invalid, defaults to linear fusion.
func fusionScore(denseScore, sparseScore float64, method string, denseWeight, sparseWeight float64) float64 {
	switch method {
	case "linear":
		return (denseWeight * denseScore) + (sparseWeight * sparseScore)
	case "max":
		return math.Max(denseScore, sparseScore)
	case "min":
		return math.Min(denseScore, sparseScore)
	case "harmonic":
		// Harmonic mean weighted by the params
		if denseScore == 0 || sparseScore == 0 {
			return 0
		}
		return 2 * (denseWeight * denseScore * sparseWeight * sparseScore) /
			(denseWeight*denseScore + sparseWeight*sparseScore)
	case "reciprocal_rank_fusion":
		// RRF score fusion (popular in information retrieval)
		k := 60.0 // constant to prevent division by zero issues
		return 1.0/(k+(1.0/denseScore)) + 1.0/(k+(1.0/sparseScore))
	default:
		return (denseWeight * denseScore) + (sparseWeight * sparseScore)
	}
}

// normalizeScores applies min-max normalization to scale scores between 0 and 1
func normalizeScores(scores map[string]float32) map[string]float32 {
	if len(scores) == 0 {
		return scores
	}

	// Find min and max
	var min, max float32
	first := true
	for _, score := range scores {
		if first {
			min = score
			max = score
			first = false
			continue
		}

		if score < min {
			min = score
		}
		if score > max {
			max = score
		}
	}

	// Apply min-max normalization to scale between 0 and 1
	normalized := make(map[string]float32, len(scores))
	if max == min {
		// All scores are the same
		for id := range scores {
			normalized[id] = 1.0
		}
	} else {
		for id, score := range scores {
			normalized[id] = (score - min) / (max - min)
		}
	}

	return normalized
}

// recencyBoost adjusts a score based on how recent a job posting is
func recencyBoost(score float64, postedDate time.Time, weight float64, halfLifeDays float64) float64 {
	if postedDate.IsZero() {
		return score
	}

	// Calculate days since posting
	daysSince := time.Since(postedDate).Hours() / 24.0

	// Decay factor: 1.0 for new jobs, decreasing over time
	decayFactor := math.Exp(-daysSince / halfLifeDays)

	// Apply decay to score with controllable weight
	return (1.0-weight)*score + weight*decayFactor
}

// safeGetString safely gets a string value from a map
func safeGetString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}

	if val, ok := m[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}

	return ""
}
