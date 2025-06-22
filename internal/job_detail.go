package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"talent-rodeo/assets"
	"talent-rodeo/internal/utils"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds/tools"
	"github.com/samber/lo"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	JobDetailsCollection = "job_details"
)

type SearchQueue interface {
	QueueSearch(ctx context.Context, search SearchTask) error
	GetQueueDepth(ctx context.Context) (int, error)
}

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

	CompanyLogo    string
	CompanyProfile string

	Location   string
	PostedDate time.Time
	URL        string
	ApplyURL   string // Direct application link, if different from job URL

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
	Summary             string   `json:"summary" description:"Core function and scope of the role"`
	Responsibilities    []string `json:"responsibilities" description:"Specific tasks the hire will perform"`
	Requirements        []string `json:"requirements" description:"Mandatory barriers to entry (e.g., degrees, legal requirements)"`
	Skills              []string `json:"skills" description:"Technical/hard skills (tools, languages, systems)"`
	Qualifications      []string `json:"qualifications" description:"Broader competencies (experience level, soft skills, industry knowledge)"`
	BonusQualifications []string `json:"bonusQualifications" description:"Secondary assets (e.g., familiarity with adjacent technologies)"`
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

func (j *JobDetail) GetCacheKey() string {
	return fmt.Sprintf("%s:%s", j.Provider, j.ProviderJobID)
}

func initJobDetail(_ context.Context, _ *FirestoreClient, job *JobDetail) {
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
		job.CreatedAt = time.Now().UTC()
	}

	if job.SearchJobIDs == nil {
		job.SearchJobIDs = []string{}
	}
}

func ExtractJobDetailData(ctx context.Context, job *JobDetail) error {
	pb, err := assets.EmbeddedFiles.ReadFile("prompts/job-summary.001.txt")
	if err != nil {
		return fmt.Errorf("failed to read system prompt: %w", err)
	}

	b, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job detail: %w", err)
	}

	prompt := string(pb) + "\n\n```json\n" + string(b) + "\n```\n\n"

	data, err := tools.StructuredAsk[JobDetailExtractedData](ctx, prompt,
		tools.WithModel(tools.DeepSeekChat),
		tools.WithMaxTokens(8192))
	if err != nil {
		return fmt.Errorf("failed to summarize job detail: %w", err)
	}

	job.ExtractedData = &data
	return nil
}

func SaveJobDetail(ctx context.Context, fs *FirestoreClient, job *JobDetail) error {
	if fs == nil {
		return fmt.Errorf("SaveJobDetail: fs is nil")
	}
	if job == nil {
		return fmt.Errorf("SaveJobDetail: job is nil")
	}
	initJobDetail(ctx, fs, job)

	_, err := fs.Collection(JobDetailsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return fmt.Errorf("firestore.Set: %w", err)
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)
	return nil
}

func SaveJobDetailTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, job *JobDetail) error {
	if fs == nil {
		return fmt.Errorf("SaveJobDetailTx: fs is nil")
	}
	if tx == nil {
		return fmt.Errorf("SaveJobDetailTx: tx is nil")
	}
	if job == nil {
		return fmt.Errorf("SaveJobDetailTx: job is nil")
	}
	initJobDetail(ctx, fs, job)

	if err := tx.Set(fs.Collection(JobDetailsCollection).Doc(job.ID), job); err != nil {
		return fmt.Errorf("firestore.Transaction.Set: %w", err)
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)

	return nil
}

func GetJobDetail(ctx context.Context, fs *FirestoreClient, id string) (*JobDetail, bool, error) {
	if fs == nil {
		return nil, false, fmt.Errorf("GetJobDetail: fs is nil")
	}
	if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
		if jobDetail == nil {
			return nil, false, fmt.Errorf("GetJobDetail: jobDetail in cache is nil")
		}
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

func GetJobDetailsWithNoExtractedData(ctx context.Context, fs *FirestoreClient) ([]JobDetail, error) {
	if fs == nil {
		return nil, fmt.Errorf("GetJobDetailsWithNoExtractedData: fs is nil")
	}
	iter := fs.Collection(JobDetailsCollection).Where("ExtractedData", "==", nil).Documents(ctx)

	var jobDetails []JobDetail
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue // skip nil doc
		}

		var jobDetail JobDetail
		if err := doc.DataTo(&jobDetail); err != nil {
			return nil, err
		}

		jobDetails = append(jobDetails, jobDetail)
	}

	return jobDetails, nil
}

// linkJobToCriteria creates a link between a job and criteria
func LinkJobDetailToCriteriaTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, criteriaID, jobID string) error {
	if fs == nil {
		return fmt.Errorf("LinkJobDetailToCriteriaTx: fs is nil")
	}
	if tx == nil {
		return fmt.Errorf("LinkJobDetailToCriteriaTx: tx is nil")
	}
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	// Create a subcollection reference under search_criteria/{criteriaID}/job_details/{linkID}
	linkRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID).Collection(JobDetailsCollection).Doc(linkID)

	// Create the link document using JobDetailCriteriaLink struct
	link := JobDetailCriteriaLink{
		CriteriaID: criteriaID,
		JobID:      jobID,
		CreatedAt:  time.Now(),
	}

	return tx.Set(linkRef, link)
}

func UnlinkJobDetailFromCriteriaTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, criteriaID, jobID string) error {
	if fs == nil {
		return fmt.Errorf("UnlinkJobDetailFromCriteriaTx: fs is nil")
	}
	if tx == nil {
		return fmt.Errorf("UnlinkJobDetailFromCriteriaTx: tx is nil")
	}
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	// Delete the link document from search_criteria/{criteriaID}/job_details/{linkID}
	linkRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID).Collection(JobDetailsCollection).Doc(linkID)
	return tx.Delete(linkRef)
}

// ListJobDetails retrieves all job details with optional limit and offset
func ListJobDetails(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]JobDetail, error) {
	if fs == nil {
		return nil, fmt.Errorf("ListJobDetails: fs is nil")
	}
	query := fs.Collection("job_details").OrderBy("CreatedAt", firestore.Desc)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	iter := query.Documents(ctx)
	var details []JobDetail

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate job details: %w", err)
		}
		if doc == nil {
			continue // skip nil doc
		}

		var jobDetail JobDetail
		if err := doc.DataTo(&jobDetail); err != nil {
			return nil, fmt.Errorf("failed to parse job detail %s: %w", doc.Ref.ID, err)
		}

		details = append(details, jobDetail)
	}

	return details, nil
}

// DeleteJobDetailsByPrefix deletes all job details with IDs starting with the given prefix
func DeleteJobDetailsByPrefix(ctx context.Context, fs *FirestoreClient, prefix string) (int, error) {
	if fs == nil {
		return 0, fmt.Errorf("DeleteJobDetailsByPrefix: fs is nil")
	}
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
		if doc == nil {
			continue // skip nil doc
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
func GetJobDetailsByIDs(ctx context.Context, fs *FirestoreClient, ids []string) ([]JobDetail, error) {
	if fs == nil {
		return nil, fmt.Errorf("GetJobDetailsByIDs: fs is nil")
	}
	if len(ids) == 0 {
		return []JobDetail{}, nil
	}

	// First check cache
	var result []JobDetail
	var missingIDs []string

	for _, id := range ids {
		if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
			if jobDetail != nil {
				result = append(result, *jobDetail)
			} // else skip nil
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
			result = append(result, jobDetail)
		}
	}

	return result, nil
}

func getSemanticJobsCacheKey(userID string, topK int, params *HybridSearchParams, steeringQuery SteeringQuery) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}
	if topK <= 0 {
		return "", fmt.Errorf("topK must be greater than 0")
	}
	paramsHash := "default"
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return "", fmt.Errorf("failed to marshal params: %w", err)
		}
		hash, err := utils.SHA256Hash(b)
		if err != nil {
			return "", fmt.Errorf("failed to hash params: %w", err)
		}
		paramsHash = hash
	}

	// Include steering query in cache key to handle custom queries
	b, err := json.Marshal(steeringQuery)
	if err != nil {
		return "", fmt.Errorf("failed to marshal steering query: %w", err)
	}
	steeringHash, err := utils.SHA256Hash(b)
	if err != nil {
		return "", fmt.Errorf("failed to hash steering query: %w", err)
	}

	combo := fmt.Sprintf("semantic_jobs:%s:%d:%s:%s", userID, topK, paramsHash, steeringHash)
	cacheKey, err := utils.SHA256Hash([]byte(combo))
	if err != nil {
		return "", fmt.Errorf("failed to hash combo: %w", err)
	}

	return cacheKey, nil
}

func getSemanticJobsFilter(ctx context.Context, fs *FirestoreClient, userID, criteriaID string, steeringQuery SteeringQuery) (map[string]any, error) {
	if fs == nil {
		return nil, fmt.Errorf("getSemanticJobsFilter: fs is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if err := steeringQuery.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate steering query: %w", err)
	}

	filter := map[string]any{
		"criteriaID": map[string]any{
			"$eq": criteriaID,
		},
		// "workType": map[string]any{
		// 	"$eq": string(steeringQuery.WorkType),
		// },
		// "experienceLevel": map[string]any{
		// 	"$eq": string(steeringQuery.ExperienceLevel),
		// },
	}

	// Get all IDs for UserJobs for this user
	userJobs, err := ListUserJobs(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user jobs: %w", err)
	}

	if len(userJobs) > 0 {
		jobIDs := []any{} // for protobuf, we need to create []interface{} to use $nin
		for _, job := range userJobs {
			jobIDs = append(jobIDs, job.ID)
		}

		filter["ID"] = map[string]any{
			"$nin": jobIDs, // Exclude jobs already in user's job list
		}
	}

	return filter, nil
}

// SearchJobDetails retrieves semantic search results for a user with 1-hour caching
// This function is used by the dashboard for the "New" job status tab
func SearchJobDetails(ctx context.Context, fs *FirestoreClient, userID, criteriaID string, steeringQuery SteeringQuery, topK int, params *HybridSearchParams) ([]ScoredJobDetail, error) {
	if fs == nil {
		return nil, fmt.Errorf("SearchJobDetails: fs is nil")
	}
	cacheKey, err := getSemanticJobsCacheKey(userID, topK, params, steeringQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get semantic jobs cache key: %w", err)
	}

	// Check cache first
	if cachedJobs, found := fs.semanticJobsCache.Get(cacheKey); found {
		return cachedJobs, nil
	}

	filter, err := getSemanticJobsFilter(ctx, fs, userID, criteriaID, steeringQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get semantic jobs filter: %w", err)
	}

	// Cache miss - perform semantic search
	if params == nil {
		params = DefaultHybridSearchParams()
	}
	scoredJobs, err := SemanticSearchJobs(ctx, steeringQuery, filter, topK*2, params)
	if err != nil {
		return nil, fmt.Errorf("SemanticSearchJobs failed: %w", err)
	}

	rerankedJobs, err := RerankResults(ctx, scoredJobs, steeringQuery.RerankQuery, topK, PineconeRerankBGEv2M3)
	if err != nil {
		return nil, fmt.Errorf("RerankResults failed: %w", err)
	}

	//
	// Now get all of the actual JobDetails from Firestore. We get all the IDs,
	// then fetch the details in a single batch to minimize Firestore reads.
	// This is important because the reranked jobs do not have all details.
	//

	jobMap := map[string]ScoredJobDetail{}
	for _, job := range rerankedJobs {
		jobMap[job.JobDetail.ID] = job
	}

	ids := lo.Keys(jobMap)

	jobDetails, err := GetJobDetailsByIDs(ctx, fs, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details for semantic search: %w", err)
	}

	if len(jobDetails) != len(rerankedJobs) {
		// HMMM: job detail might still be processing in the background?
		// return nil, fmt.Errorf("mismatch in job details count: expected scoredJobs:%d, got jobDetails:%d", len(scoredJobs), len(jobDetails))
	}

	// Update the job details in the scored jobs
	for _, job := range jobDetails {
		if scoredJob, exists := jobMap[job.ID]; exists {
			// Update the job detail with the full job data
			scoredJob.JobDetail = job
			jobMap[job.ID] = scoredJob
		}
	}

	// Convert the updated map back to a slice and sort by Score descending
	updatedScoredJobs := make([]ScoredJobDetail, 0, len(jobMap))
	for _, scoredJob := range jobMap {
		updatedScoredJobs = append(updatedScoredJobs, scoredJob)
	}

	// Sort by Score descending to maintain the original semantic search ranking
	sort.Slice(updatedScoredJobs, func(i, j int) bool {
		return updatedScoredJobs[i].Score > updatedScoredJobs[j].Score
	})

	// Cache the results with 1-hour TTL
	fs.semanticJobsCache.SetWithTTL(cacheKey, updatedScoredJobs, 1, 1*time.Hour)

	return updatedScoredJobs, nil
}

// InvalidateSemanticJobsCache invalidates the semantic jobs cache for a user
// This should be called when a user interacts with jobs (changes status, etc.)
func InvalidateSemanticJobsCache(fs *FirestoreClient, userID string) {
	// TODO: We need to invalidate all cache entries for this user
	// Since Ristretto doesn't support prefix-based deletion, we'll use a simple approach
	// and let the cache entries expire naturally. For a more aggressive approach,
	// we could track cache keys per user, but for now this is sufficient.

	// Note: In a production system, you might want to maintain a separate
	// mapping of user IDs to cache keys for more efficient invalidation

}
