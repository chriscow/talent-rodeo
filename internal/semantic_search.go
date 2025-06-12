package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
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
		DenseWeight:     0.9,
		SparseWeight:    0,
		ScoreFusion:     "linear", // Default to linear fusion
		RecencyWeight:   0.1,
		RecencyHalfLife: 30.0,
		ScoreThreshold:  0,
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
		"_id":         s.JobDetail.ID,
		"description": s.JobDetail.Description,
		"provider":    s.JobDetail.Provider,
		"providerID":  s.JobDetail.ProviderJobID,
		"title":       s.JobDetail.Title,
		"company":     s.JobDetail.Company,
		"location":    s.JobDetail.Location,
		"url":         s.JobDetail.URL,
		"postedDate":  s.JobDetail.PostedDate,
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
func SemanticSearchJobs(ctx context.Context, steeringQuery SteeringQuery, filter map[string]any, topK int, params *HybridSearchParams) ([]*ScoredJobDetail, error) {
	if params == nil {
		params = DefaultHybridSearchParams()
	} else {
		params = params.Validate()
	}

	client, err := NewPineconeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
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
	logger.Debug("dense search results", "count", len(denseResults.Matches), "top-score", denseResults.Matches[0].Score)

	sparseIndex := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
	if sparseIndex == "" {
		return nil, fmt.Errorf("PINECONE_SPARSE_INDEX_HOST environment variable not set")
	}

	query := steeringQuery.MoveToward
	sparseResults, err := client.SearchRecords(ctx, sparseIndex, JobDetailNamespace, query,
		WithTopK(topK*2), WithFilter(filter), WithFields([]string{"ID"}))
	if err != nil {
		return nil, fmt.Errorf("failed to search sparse index: %w", err)
	}
	logger.Debug("sparse search results", "count", len(sparseResults.Result.Hits), "top-score", sparseResults.Result.Hits[0].Score)

	searchResult := &SearchResult{
		DenseResults:  denseResults,
		SparseResults: sparseResults,
	}

	return processSearchResults(ctx, logger, searchResult, topK, params)
}

// RerankResults applies reranking to a list of scored job details using the Pinecone reranker.
// This is extracted to make testing easier.
func RerankResults(ctx context.Context, scoredMatches []*ScoredJobDetail, rerankQuery string, topK int, model PineconeRerankModel) ([]ScoredJobDetail, error) {
	rerankParams := map[string]any{
		"truncate": "END",
	}

	client, err := NewPineconeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
	}

	// Convert to Documents for reranking
	documents := make([]pinecone.Document, 0, len(scoredMatches))
	jobMap := make(map[string]JobDetail)
	for _, match := range scoredMatches {
		if match.JobDetail.Description == "" {
			continue
		}

		// Convert metadata to Document
		doc := match.ToPineconeDocument()

		documents = append(documents, doc)

		// Store job details in a map for easy access
		jobMap[match.JobDetail.ID] = match.JobDetail
	}

	rerankResponse, err := client.Inference.Rerank(ctx, &pinecone.RerankRequest{
		Model:      string(model),
		Query:      rerankQuery,
		Documents:  documents,
		RankFields: &[]string{"description"},
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

		id := doc["_id"].(string)
		jobDetail, exists := jobMap[id]
		if !exists {
			// If we don't have the job detail, skip this result
			continue
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
func processSearchResults(_ context.Context, logger *slog.Logger, searchResult *SearchResult, topK int, params *HybridSearchParams) ([]*ScoredJobDetail, error) {
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
			var err error
			postedDate, err = time.Parse(time.RFC3339, dateStr)
			if err != nil {
				logger.Error("failed to parse posted date", "error", err)
			}
		}

		match, exists := allMatches[id]
		if !exists {
			match = &ScoredJobDetail{
				JobDetail: JobDetail{
					ID:            id,
					Provider:      meta.Fields["provider"].GetStringValue(),
					ProviderJobID: meta.Fields["providerJobID"].GetStringValue(),
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
	// Only normalize scores for methods that need it
	if params.ScoreFusion != "reciprocal_rank_fusion" {
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
	}

	// Apply score fusion strategy and recency boosting
	for _, match := range allMatches {
		// Apply fusion based on selected strategy
		fused := fusionScore(match.DenseScore, match.SparseScore, params.ScoreFusion, params.DenseWeight, params.SparseWeight)

		// Apply recency boosting if we have a date
		if !match.JobDetail.PostedDate.IsZero() {
			daysSince := time.Since(match.JobDetail.PostedDate).Hours() / 24.0
			decayFactor := math.Exp(-daysSince / params.RecencyHalfLife)
			recencyBoostedScore := (1.0-params.RecencyWeight)*fused + params.RecencyWeight*decayFactor

			// Use structured logging for debug information
			logger.Debug("Recency boosting details",
				"ID", match.JobDetail.ID,
				"DenseScore", match.DenseScore,
				"SparseScore", match.SparseScore,
				"FusedBeforeRecency", fused,
				"PostedDate", match.JobDetail.PostedDate.Format(time.RFC3339),
				"DaysSince", daysSince,
				"DecayFactor", decayFactor,
				"RecencyWeight", params.RecencyWeight,
				"FinalScoreAfterRecency", recencyBoostedScore,
			)

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
	results := make([]*ScoredJobDetail, 0, len(scoredMatches))
	for _, match := range scoredMatches {
		// The check for match.JobDetail.Description == "" is redundant here,
		// as items with empty descriptions are filtered out before deduplication.
		results = append(results, match)
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
		if denseScore == 0 || sparseScore == 0 { // denseWeight or sparseWeight being 0 is fine
			return 0
		}
		// Avoid division by zero if weights are zero, though normalized weights should prevent this.
		// However, if one score is zero, the harmonic mean effectively becomes zero.
		// The original formula: 2 * (denseWeight * denseScore * sparseWeight * sparseScore) / (denseWeight*denseScore + sparseWeight*sparseScore)
		// If denseWeight*denseScore + sparseWeight*sparseScore is 0 (e.g. both scores are 0), this would divide by zero.
		// The check above (denseScore == 0 || sparseScore == 0) handles cases where one or both scores are zero.
		// If weights can be zero, and scores non-zero, the denominator could still be zero if one weight makes its term zero.
		// Assuming weights are normalized and > 0 if used.
		denominator := denseWeight*denseScore + sparseWeight*sparseScore
		if denominator == 0 {
			return 0
		}
		return 2 * (denseWeight * denseScore * sparseWeight * sparseScore) / denominator
	case "reciprocal_rank_fusion":
		// RRF score fusion (popular in information retrieval)
		k := 60.0 // constant
		rrfScore := 0.0
		if denseScore > 0 {
			rrfScore += 1.0 / (k + (1.0 / denseScore))
		}
		if sparseScore > 0 {
			rrfScore += 1.0 / (k + (1.0 / sparseScore))
		}
		return rrfScore
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

// recencyBoost adjusts a score based on how recent a job posting is.
// It calculates the number of days that have passed since the `postedDate`.
// An exponential decay factor is then computed using `daysSince` and `halfLifeDays`.
// This `decayFactor` ranges from 1.0 (for a job posted now) down towards 0.0 as the job gets older.
// The `halfLifeDays` parameter serves as the time constant for the decay; after `halfLifeDays` have passed
// (i.e., when `daysSince == halfLifeDays`), the `decayFactor` will be `math.Exp(-1)` (approximately 0.368).
// A smaller `halfLifeDays` value results in a faster decay.
// Finally, the original `score` is combined with this `decayFactor` using a weighted average:
// `boostedScore = (1.0 - weight) * score + weight * decayFactor`.
// The `weight` parameter (0.0-1.0) determines the influence of recency on the final score.
// A small `weight` (e.g., 0.1) means recency has a small influence (original score dominates).
// A large `weight` (e.g., 0.9) means recency has a strong influence.
// If `postedDate` is the zero value, the original score is returned unmodified.
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
