package internal

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"talent-rodeo/internal/utils"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	JobDetailsCollection      = "job_details"
	JobCriteriaLinkCollection = "job_criteria_links"
)

// JobDetail contains comprehensive information about a specific job.
// This is returned by GetJobDetails and includes all available
// information about the position.
type JobDetail struct {
	// Basic job information (duplicated from JobListing for clarity)
	ID            string
	Provider      string
	ProviderJobID string
	Title         string
	Company       string
	Location      string
	PostedDate    time.Time
	URL           string

	// Detailed information
	Description string // Complete job description

	Requirements   []string // List of job requirements
	Qualifications []string // Required qualifications
	Benefits       []string // List of benefits
	Skills         []string // Required skills
	SalaryInfo     *SalaryRange
	EmploymentType string // Full-time, part-time, contract, etc.
	WorkplaceType  string // Remote, on-site, hybrid
	Industry       string
	CompanyInfo    *Company

	// Additional fields from ScrapingDog
	Status           string
	SeniorityLevel   string
	JobFunction      string
	Recruiters       []Recruiter
	SimilarJobs      []RelatedJob
	PeopleAlsoViewed []RelatedJob

	ExtractedData *JobDetailExtractedData

	CreatedAt    time.Time
	UpdatedAt    *time.Time
	SearchJobIDs []string
}

type JobDetailCriteriaLink struct {
	CriteriaID string
	JobID      string
	CreatedAt  time.Time
}

type JobDetailExtractedData struct {
	Summary        string   `description:"a concise summary of the job listing"`
	Requirements   []string `description:"the essential criteria that employers consider non-negotiable for a candidate to perform the job successfully"`
	Qualifications []string `description:"the broader set of skills, experiences, and attributes that make someone a good fit for the role"`
}

// Company contains information about the company posting the job.
// This information may be limited depending on the provider.
type Company struct {
	Name        string
	Website     string
	Industry    string
	Size        string // Company size range
	Description string
	LogoURL     string
	ProfileURL  string
	// Future considerations:
	// - Company culture
	// - Tech stack
	// - Funding status
	// - Growth metrics
}

// SalaryRange represents a salary range for job listings.
// Some providers may not support all fields, so implementations
// should handle partial data appropriately.
type SalaryRange struct {
	Min      float64 `json:"min" description:"The minimum salary in the range"`
	Max      float64 `json:"max" description:"The maximum salary in the range"`
	Currency string  `json:"currency" description:"The currency of the salary range"`
	Period   string  `json:"period" description:"The period of the salary range" enum:"yearly,monthly,hourly"`
	// Future considerations:
	// - Equity compensation
	// - Bonus structure
	// - Benefits value
}

// Recruiter represents a recruiter associated with a job posting
type Recruiter struct {
	Photo      string
	Name       string
	Title      string
	ProfileURL string
}

// RelatedJob represents a related or similar job posting
type RelatedJob struct {
	Title       string
	Company     string
	Location    string
	PostingDate string
	URL         string
}

// HybridSearchParams provides configuration options for hybrid semantic search
type HybridSearchParams struct {
	DenseWeight     float64 // Weight for dense results (0.0-1.0)
	SparseWeight    float64 // Weight for sparse results (0.0-1.0)
	ScoreFusion     string  // "linear", "max", "min", "harmonic", "reciprocal_rank_fusion", "rerank"
	RecencyWeight   float64 // Weight to apply to recency boosting (0.0-1.0)
	RecencyHalfLife float64 // Half-life in days for recency decay
	EnableReranking bool    // Whether to use the reranker as a final step
	RerankModel     string  // Model to use for reranking, defaults to "bge-reranker-v2-m3"
}

// DefaultHybridSearchParams returns a set of default parameters for hybrid search
func DefaultHybridSearchParams() *HybridSearchParams {
	return &HybridSearchParams{
		DenseWeight:     0.7,
		SparseWeight:    0.3,
		ScoreFusion:     "linear",
		RecencyWeight:   0.2,
		RecencyHalfLife: 30.0,
		EnableReranking: false,
		RerankModel:     "bge-reranker-v2-m3",
	}
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
		"rerank":                 true,
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

	// Default rerank model if none specified
	if p.RerankModel == "" {
		p.RerankModel = "bge-reranker-v2-m3"
	}

	return p
}

// ScoredMatch represents a search result with scores from different sources
// type ScoredMatch struct {
// 	ID            string
// 	DenseScore    float64
// 	SparseScore   float64
// 	CombinedScore float64
// 	PostedDate    time.Time
// 	Metadata      map[string]any
// }

type ScoredJobDetail struct {
	JobDetail     JobDetail
	DenseScore    float64
	SparseScore   float64
	CombinedScore float64
}

func (j *JobDetail) GetCacheKey() string {
	return fmt.Sprintf("%s:%s", j.Provider, j.ProviderJobID)
}

func initJobDetail(ctx context.Context, fs *FirestoreClient, job *JobDetail) {
	if job == nil {
		return
	}

	if job.ID == "" {
		job.ID = job.GetCacheKey()
	}

	if job.UpdatedAt == nil && !job.CreatedAt.IsZero() {
		nowUTC := time.Now().UTC()
		job.UpdatedAt = &nowUTC
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	if job.SearchJobIDs == nil {
		job.SearchJobIDs = []string{}
	}

	job.CreatedAt = time.Now().UTC()
}

func SaveJobDetail(ctx context.Context, fs *FirestoreClient, job *JobDetail) error {
	initJobDetail(ctx, fs, job)

	_, err := fs.Collection(JobDetailsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return err
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)
	return nil
}

func SaveJobDetailTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, job *JobDetail) error {
	initJobDetail(ctx, fs, job)

	if err := tx.Set(fs.Collection(JobDetailsCollection).Doc(job.ID), job); err != nil {
		return err
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)

	return nil
}

func GetJobDetail(ctx context.Context, fs *FirestoreClient, id string) (*JobDetail, bool, error) {
	if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
		return jobDetail, true, nil
	}

	doc, err := fs.Collection(JobDetailsCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, err
	}

	var jobDetail JobDetail
	if err := doc.DataTo(&jobDetail); err != nil {
		return nil, false, err
	}

	return &jobDetail, false, nil
}

func LinkJobDetailToCriteria(ctx context.Context, fs *FirestoreClient, criteriaID, jobID string) error {
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	linkRef := fs.Collection(JobCriteriaLinkCollection).Doc(linkID)

	// Check if the link already exists
	doc, err := linkRef.Get(ctx)
	if err == nil && doc.Exists() {
		// Link already exists, skip creation
		return nil
	}

	// Create the link
	link := JobDetailCriteriaLink{
		CriteriaID: criteriaID,
		JobID:      jobID,
		CreatedAt:  time.Now(),
	}

	_, err = linkRef.Set(ctx, link)
	return err
}

// linkJobToCriteria creates a link between a job and criteria
func LinkJobDetailToCriteriaTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, criteriaID, jobID string) error {
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	linkRef := fs.Collection(JobCriteriaLinkCollection).Doc(linkID)

	// Check if the link already exists
	doc, err := tx.Get(linkRef)
	if err == nil && doc.Exists() {
		// Link already exists, skip creation
		return nil
	}

	// Create the link
	link := JobDetailCriteriaLink{
		CriteriaID: criteriaID,
		JobID:      jobID,
		CreatedAt:  time.Now(),
	}

	return tx.Create(linkRef, &link)
}

// ListJobDetails retrieves all job details with optional limit and offset
func ListJobDetails(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]map[string]any, error) {
	query := fs.Collection("job_details").OrderBy("CreatedAt", firestore.Desc)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	iter := query.Documents(ctx)
	var details []map[string]any

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate job details: %w", err)
		}

		details = append(details, doc.Data())
	}

	return details, nil
}

// DeleteJobDetailsByPrefix deletes all job details with IDs starting with the given prefix
func DeleteJobDetailsByPrefix(ctx context.Context, fs *FirestoreClient, prefix string) (int, error) {
	if prefix == "" {
		return 0, fmt.Errorf("prefix is required")
	}

	// Query for documents with IDs starting with the prefix
	iter := fs.Collection("job_details").Documents(ctx)
	deleted := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return deleted, fmt.Errorf("failed to iterate job details: %w", err)
		}

		// Check if document ID starts with prefix
		if len(doc.Ref.ID) >= len(prefix) && doc.Ref.ID[:len(prefix)] == prefix {
			if _, err := doc.Ref.Delete(ctx); err != nil {
				return deleted, fmt.Errorf("failed to delete job detail %s: %w", doc.Ref.ID, err)
			}
			deleted++
		}
	}

	return deleted, nil
}

// GetJobDetailsByIDs retrieves multiple job details by their IDs
func GetJobDetailsByIDs(ctx context.Context, fs *FirestoreClient, ids []string) ([]*JobDetail, error) {
	if len(ids) == 0 {
		return []*JobDetail{}, nil
	}

	// First check cache
	var result []*JobDetail
	var missingIDs []string

	for _, id := range ids {
		if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
			result = append(result, jobDetail)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If all IDs were found in cache, return early
	if len(missingIDs) == 0 {
		return result, nil
	}

	// Create a batch of document references to get
	refs := make([]*firestore.DocumentRef, len(missingIDs))
	for i, id := range missingIDs {
		refs[i] = fs.Collection(JobDetailsCollection).Doc(id)
	}

	// Execute the batch get
	docs, err := fs.GetAll(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details: %w", err)
	}

	// Process results
	for i, doc := range docs {
		if doc.Exists() {
			var jobDetail JobDetail
			if err := doc.DataTo(&jobDetail); err != nil {
				return nil, fmt.Errorf("failed to parse job detail %s: %w", missingIDs[i], err)
			}

			// Add to cache
			fs.jobDetailCache.SetWithTTL(jobDetail.ID, &jobDetail, 1, 1*time.Hour)

			// Add to result
			result = append(result, &jobDetail)
		}
	}

	return result, nil
}

// fusionScore combines dense and sparse scores using the specified method
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

// extractMetadata pulls relevant fields from the metadata into a map
func extractMetadata(meta *pinecone.Metadata) map[string]any {
	if meta == nil {
		return nil
	}

	result := make(map[string]any)

	// Extract common fields if they exist
	fields := []string{"text", "title", "company", "location", "url", "postedDate"}

	for _, field := range fields {
		if val, ok := meta.Fields[field]; ok {
			// Extract the value based on the actual implementation of Pinecone metadata
			// This assumes the Value type from protobuf
			if str := val.GetStringValue(); str != "" {
				result[field] = str
			} else if num := val.GetNumberValue(); num != 0 {
				result[field] = num
			} else if b := val.GetBoolValue(); b {
				result[field] = b
			}
		}
	}

	return result
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
//  6. Optionally applies reranking for final ordering
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

	denseVector, err := steeringQuery.GetDenseVector(ctx, client, PineconeEmbedDense)
	if err != nil {
		return nil, fmt.Errorf("failed to get dense steering vector: %w", err)
	}

	// Fixed: Changed from GetDenseVector to GetSparseVector
	sparseVector, err := steeringQuery.GetSparseVector(ctx, client, PineconeEmbedSparse)
	if err != nil {
		return nil, fmt.Errorf("failed to get sparse steering vector: %w", err)
	}

	denseIndex := os.Getenv("PINECONE_DENSE_INDEX_HOST")
	if denseIndex == "" {
		return nil, fmt.Errorf("PINECONE_DENSE_INDEX_HOST environment variable not set")
	}

	denseResults, err := client.QueryByVector(ctx, denseIndex, JobDetailNamespace, denseVector,
		WithTopK(topK*2), WithFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("failed to search dense index: %w", err)
	}

	sparseIndex := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
	if sparseIndex == "" {
		return nil, fmt.Errorf("PINECONE_SPARSE_INDEX_HOST environment variable not set")
	}
	sparseResults, err := client.QueryByVector(ctx, sparseIndex, JobDetailNamespace, sparseVector,
		WithTopK(topK*2), WithFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("failed to search sparse index: %w", err)
	}

	// Create maps to track scores from each source
	denseScores := make(map[string]float32)
	sparseScores := make(map[string]float32)
	allMatches := make(map[string]*ScoredJobDetail)

	// Process dense results
	for _, hit := range denseResults.Matches {
		id := hit.Vector.Id
		meta := hit.Vector.Metadata
		if meta == nil {
			continue // skip if no metadata
		}

		denseScores[id] = hit.Score

		// metadata := extractMetadata(meta)

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
					ID:         id,
					PostedDate: postedDate,
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
	for _, hit := range sparseResults.Matches {
		id := hit.Vector.Id
		meta := hit.Vector.Metadata
		if meta == nil {
			continue // skip if no metadata
		}

		sparseScores[id] = hit.Score

		// Parse posted date if available
		var postedDate time.Time
		if dateStr := meta.Fields["postedDate"].GetStringValue(); dateStr != "" {
			postedDate, _ = time.Parse(time.RFC3339, dateStr)
		}

		match, exists := allMatches[id]
		if !exists {
			match = &ScoredJobDetail{
				JobDetail: JobDetail{
					ID:         id,
					PostedDate: postedDate,
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
			fused = recencyBoost(fused, match.JobDetail.PostedDate, params.RecencyWeight, params.RecencyHalfLife)
		}

		match.CombinedScore = fused
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
		if existing, found := deduplicated[hash]; !found || match.CombinedScore > existing.CombinedScore {
			deduplicated[hash] = match
		}
	}

	// Create a sorted list of matches by combined score
	scoredMatches := make([]*ScoredJobDetail, 0, len(deduplicated))
	for _, match := range deduplicated {
		scoredMatches = append(scoredMatches, match)
	}

	// Sort by combined score in descending order
	sort.Slice(scoredMatches, func(i, j int) bool {
		return scoredMatches[i].CombinedScore > scoredMatches[j].CombinedScore
	})

	// Limit to topK
	if len(scoredMatches) > topK {
		scoredMatches = scoredMatches[:topK]
	}

	// Optional: use reranker for final ordering if specified
	rerankParams := map[string]any{
		"truncate": "END",
	}

	// If reranking is specifically requested
	if params.ScoreFusion == "rerank" {
		// Convert to Documents for reranking
		documents := make([]pinecone.Document, 0, len(scoredMatches))
		for _, match := range scoredMatches {
			if match.JobDetail.Description == "" {
				continue
			}

			// Convert metadata to Document
			doc := pinecone.Document{}
			doc["ID"] = match.JobDetail.ID
			doc["Score"] = match.CombinedScore

			documents = append(documents, doc)
		}

		rerankResponse, err := client.Inference.Rerank(ctx, &pinecone.RerankRequest{
			Model:      params.RerankModel,
			Query:      steeringQuery.RerankQuery,
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
				JobDetail:     jobDetail,
				CombinedScore: float64(result.Score),
			})
		}

		return results, nil
	}

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
