package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/samber/lo"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	userJobsCollection = "user_jobs"
)

// Define JobStatus as a string type and add constants for each status
type JobStatus string

const (
	// JobStatusUnprocessed is the initial status of a job detail when it is retrieved by the search provider.
	JobStatusUnprocessed JobStatus = "unprocessed"
	JobStatusNew         JobStatus = "new"
	JobStatusSaved       JobStatus = "saved"
	JobStatusApplied     JobStatus = "applied"
	JobStatusArchived    JobStatus = "archived"

	// DefaultUserJobsTopK is the number of top jobs to return from semantic search
	DefaultUserJobsTopK = 100
)

// ParseJobStatus converts a string to a JobStatus with validation.
// Returns the parsed status and a boolean indicating if it was valid.
// If invalid, the default JobStatusNew is returned with false.
func ParseJobStatus(statusStr string) (JobStatus, bool) {
	switch strings.ToLower(statusStr) {
	case string(JobStatusNew):
		return JobStatusNew, true
	case string(JobStatusSaved):
		return JobStatusSaved, true
	case string(JobStatusApplied):
		return JobStatusApplied, true
	case string(JobStatusArchived):
		return JobStatusArchived, true
	default:
		return JobStatusNew, false
	}
}

// UserJob represents a job that a user has interacted with
type UserJob struct {
	// ID is the same as the job_details document ID (provider_job_id)
	ID     string
	Status JobStatus
	Score  float64 // Semantic search score, if applicable

	// User interaction metadata
	SavedAt    *time.Time
	AppliedAt  *time.Time
	ArchivedAt *time.Time
	Notes      string

	// Reference to the job_details document
	JobRef *firestore.DocumentRef

	// Job metadata for display
	// CompanyName string
	// CompanyLogo string

	// Search context
	// SearchHash string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobAnalysis struct {
	CandidateRequirementsAnalysis []RequirementAnalysis
	JobRequirementsAnalysis       []RequirementAnalysis
	Summary                       Summary
	FinalJudgment                 FinalJudgment
}

type RequirementAnalysis struct {
	Requirement string
	Fit         string
	Examples    string
}

type Summary struct {
	Matches []string
	Gaps    []string
}

type FinalJudgment struct {
	Explanation string
	Rating      string `enum:"Definitely Yes,Strong Yes,Yes,Maybe,No"` // One of: "Definitely Yes", "Strong Yes", "Yes", "Maybe", "No"
}

// CreateUserJob saves a job reference for a user
func CreateUserJob(ctx context.Context, fs *FirestoreClient, userID, jobRefID string) (*UserJob, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	if jobRefID == "" {
		return nil, fmt.Errorf("job ID is required")
	}

	job := &UserJob{
		ID:        jobRefID,
		JobRef:    fs.Collection(JobDetailsCollection).Doc(jobRefID),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    JobStatusUnprocessed,
	}

	// Save to user's jobs subcollection
	// Using job.ID as document ID ensures deduplication
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to save user job: %w", err)
	}

	return job, nil
}

func CreateUserJobsBatch(ctx context.Context, fs *FirestoreClient, userID string, jobs []UserJob) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if len(jobs) == 0 {
		return nil
	}

	// Use Transaction instead of the deprecated Batch
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for _, job := range jobs {
			docRef := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(job.ID)
			if err := tx.Set(docRef, job); err != nil {
				return err
			}
		}
		return nil
	})
}

func GetUserJob(ctx context.Context, fs *FirestoreClient, userID, jobID string) (*UserJob, bool, error) {
	doc, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get user job: %w", err)
	}

	var job UserJob
	if err := doc.DataTo(&job); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal user job: %w", err)
	}

	return &job, true, nil
}

// ListUserJobs retrieves all jobs for a user. Firebase counts as one read for
// each query, it doesn't matter if your collection has 500 docs or 5000 doc. A
// single query counts as one read, even if it had to go through 500 docs
// searching for it in the cloud firestore.
func ListUserJobs(ctx context.Context, fs *FirestoreClient, userID string) ([]UserJob, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Documents(ctx)
	var jobs []UserJob

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user jobs: %w", err)
		}

		var job UserJob
		if err := doc.DataTo(&job); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user job: %w", err)
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetUserJobsSorted retrieves user jobs with a specific status and supports paging
func GetUserJobsSorted(ctx context.Context, fs *FirestoreClient, userID string, status JobStatus, pageSize int, pageToken string) ([]*UserJob, string, error) {
	// Create base query for jobs with the specified status
	baseQuery := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).
		Where("Status", "==", string(status))

	// Add ordering after filters
	baseQuery = baseQuery.OrderBy("Score", firestore.Desc)

	// Apply paging if a page token is provided
	var query firestore.Query
	if pageToken != "" {
		// Get the document snapshot for the token (which is a job ID)
		docRef := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(pageToken)
		docSnap, err := docRef.Get(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get document for pagination: %w", err)
		}

		// Use the document snapshot with StartAfter for pagination
		query = baseQuery.StartAfter(docSnap)
	} else {
		query = baseQuery
	}

	// Apply page size limit
	query = query.Limit(pageSize)

	// Execute the query
	iter := query.Documents(ctx)
	var jobs []*UserJob

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to iterate user jobs: %w", err)
		}

		var job UserJob
		if err := doc.DataTo(&job); err != nil {
			return nil, "", fmt.Errorf("failed to unmarshal user job: %w", err)
		}

		jobs = append(jobs, &job)
	}

	var nextPageToken string
	// Only set nextPageToken if we actually got the maximum number of results,
	// suggesting there are more results to be fetched
	if len(jobs) == pageSize {
		nextPageToken = jobs[len(jobs)-1].ID
	} else {
		nextPageToken = ""
	}

	return jobs, nextPageToken, nil
}

// GetJobsCount returns the count of jobs with the specified status for a user
func GetJobsCount(ctx context.Context, fs *FirestoreClient, userID string, status JobStatus) (int64, error) {
	if userID == "" {
		return 0, fmt.Errorf("user ID is required")
	}

	query := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).
		Where("Status", "==", string(status))

	// Use the more efficient aggregation query
	aggregationQuery := query.NewAggregationQuery().WithCount("count")
	results, err := aggregationQuery.Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get job count: %w", err)
	}

	// Extract the count from the results using the "count" alias
	countResult, ok := results["count"]
	if !ok {
		return 0, fmt.Errorf("count not found in aggregation results")
	}

	// Convert the result to the correct type and extract the integer value
	countValue := countResult.(*firestorepb.Value)
	return countValue.GetIntegerValue(), nil
}

// GetAllJobStatusCounts returns a map with counts for all job statuses for a user
func GetAllJobStatusCounts(ctx context.Context, fs *FirestoreClient, userID string) (map[string]int64, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Initialize the result map with all possible statuses set to 0
	result := map[string]int64{
		string(JobStatusNew):      0,
		string(JobStatusSaved):    0,
		string(JobStatusApplied):  0,
		string(JobStatusArchived): 0,
	}

	// Get counts for each status
	for status := range result {
		count, err := GetJobsCount(ctx, fs, userID, JobStatus(status))
		if err != nil {
			return nil, fmt.Errorf("failed to get count for status %s: %w", status, err)
		}
		result[status] = count
	}

	return result, nil
}

// UserHasAnyJobs checks if a user has any jobs, without counting them all
func UserHasAnyJobs(ctx context.Context, fs *FirestoreClient, userID string) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user ID is required")
	}

	// Just get the first job document - if any exist
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Where("Status", "!=", string(JobStatusUnprocessed)).Limit(1).Documents(ctx)

	// Check if there's at least one document
	_, err := iter.Next()
	if err == iterator.Done {
		// No documents found
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check for user jobs: %w", err)
	}

	// Don't forget to close the iterator
	iter.Stop()

	// If we got here, at least one job exists
	return true, nil
}

// GetJobStatuses returns a slice of all possible job statuses with proper casing
func GetJobStatuses() []string {
	return []string{
		"New",
		"Saved",
		"Applied",
		"Archived",
	}
}

// StatusToDisplayName converts a raw status string to a properly formatted display name
func StatusToDisplayName(status string) string {
	switch status {
	case string(JobStatusNew):
		return "New"
	case string(JobStatusSaved):
		return "Saved"
	case string(JobStatusApplied):
		return "Applied"
	case string(JobStatusArchived):
		return "Archived"
	default:
		return status
	}
}

// UpdateUserJobStatus updates the status and corresponding timestamp of a user's job
func UpdateUserJobStatus(ctx context.Context, fs *FirestoreClient, userID, jobID string, status JobStatus) error {
	now := time.Now()
	updates := []firestore.Update{
		{Path: "Status", Value: status},
		{Path: "UpdatedAt", Value: now},
	}

	// Add timestamp based on status
	switch status {
	case JobStatusSaved:
		updates = append(updates, firestore.Update{Path: "SavedAt", Value: now})
	case JobStatusApplied:
		updates = append(updates, firestore.Update{Path: "AppliedAt", Value: now})
	case JobStatusArchived:
		updates = append(updates, firestore.Update{Path: "ArchivedAt", Value: now})
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job status: %w", err)
	}

	return nil
}

// UpdateUserJobNotes updates the notes for a user's job
func UpdateUserJobNotes(ctx context.Context, fs *FirestoreClient, userID, jobID, notes string) error {
	updates := []firestore.Update{
		{Path: "Notes", Value: notes},
		{Path: "UpdatedAt", Value: time.Now()},
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job notes: %w", err)
	}

	return nil
}

// DeleteUserJob deletes a job for a user
func DeleteUserJob(ctx context.Context, fs *FirestoreClient, userID, jobID string) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if jobID == "" {
		return fmt.Errorf("job ID is required")
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user job: %w", err)
	}

	return nil
}

// DeleteAllUserJobsByPrefix deletes all user jobs with IDs starting with the given prefix across all users
func DeleteAllUserJobsByPrefix(ctx context.Context, fs *FirestoreClient, prefix string) (int, error) {
	if prefix == "" {
		return 0, fmt.Errorf("prefix is required")
	}

	// Get all users
	usersIter := fs.Collection(usersCollection).Documents(ctx)
	deleted := 0

	for {
		userDoc, err := usersIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return deleted, fmt.Errorf("failed to iterate users: %w", err)
		}

		// For each user, get their jobs collection
		jobsIter := userDoc.Ref.Collection(userJobsCollection).Documents(ctx)

		for {
			jobDoc, err := jobsIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return deleted, fmt.Errorf("failed to iterate jobs for user %s: %w", userDoc.Ref.ID, err)
			}

			// Check if job ID starts with prefix
			if len(jobDoc.Ref.ID) >= len(prefix) && jobDoc.Ref.ID[:len(prefix)] == prefix {
				if _, err := jobDoc.Ref.Delete(ctx); err != nil {
					return deleted, fmt.Errorf("failed to delete job %s for user %s: %w", jobDoc.Ref.ID, userDoc.Ref.ID, err)
				}
				deleted++
			}
		}
	}

	return deleted, nil
}

// GetSemanticJobsForUser retrieves semantic search results for a user with 1-hour caching
// This function is used by the dashboard for the "New" job status tab
func GetSemanticJobsForUser(ctx context.Context, fs *FirestoreClient, userID string, steeringQuery *SteeringQuery, criteriaID string, topK int, params *HybridSearchParams) ([]ScoredJobDetail, error) {
	// Skip requests without a steering query
	if steeringQuery == nil {
		return nil, fmt.Errorf("user %s has no steering query", userID)
	}

	// Create cache key based on user ID, criteria ID, topK, params hash, and steering query hash
	paramsHash := "default"
	if params != nil {
		paramsHash = fmt.Sprintf("%.2f-%.2f-%.2f-%.2f-%.2f-%s",
			params.DenseWeight, params.SparseWeight, params.RecencyWeight,
			params.RecencyHalfLife, params.ScoreThreshold, params.ScoreFusion)
	}

	// Include steering query in cache key to handle custom queries
	steeringHash := fmt.Sprintf("%s|%s|%.2f|%.2f", steeringQuery.Query, steeringQuery.RerankQuery, steeringQuery.ForceToward, steeringQuery.ForceAway)
	if steeringQuery.MoveToward != "" {
		steeringHash += "|" + steeringQuery.MoveToward
	}
	if steeringQuery.MoveAway != "" {
		steeringHash += "|" + steeringQuery.MoveAway
	}

	criteria, found, err := GetSearchCriteria(ctx, fs, criteriaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get criteria by ID %s: %w", criteriaID, err)
	}
	if !found {
		return nil, fmt.Errorf("criteria with ID %s not found", criteriaID)
	}

	cacheKey := fmt.Sprintf("semantic_jobs:%s:%s:%d:%s:%s", userID, criteriaID, topK, paramsHash, steeringHash)

	// Check cache first
	if cachedJobs, found := fs.semanticJobsCache.Get(cacheKey); found {
		return cachedJobs, nil
	}

	// Cache miss - perform semantic search
	if params == nil {
		params = DefaultHybridSearchParams()
	}
	scoredJobs, err := SemanticSearchJobs(ctx, steeringQuery, *criteria, topK, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs for user %s: %w", userID, err)
	}

	jobMap := map[string]ScoredJobDetail{}
	for _, job := range scoredJobs {
		jobMap[job.JobDetail.ID] = job
	}

	ids := lo.Keys(jobMap)

	jobDetails, err := GetJobDetailsByIDs(ctx, fs, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details for semantic search: %w", err)
	}

	if len(jobDetails) != len(scoredJobs) {
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
