package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	userJobsCollection = "user_jobs"
)

/*
You are a helpful assistant that summarizes job listings into concise, easy-to-read summaries.
Create a summary that highlights the most important aspects of the job, including key responsibilities,
required qualifications, and any notable benefits or company information.
Keep the summary brief but informative, focusing on what would matter most to job seekers.
*/
type UserJobSummary struct {
	Summary        string   `json:"summary"`
	Qualifications []string `json:"qualifications"`
}

// Define JobStatus as a string type and add constants for each status
type JobStatus string

const (
	JobStatusNew      JobStatus = "new"
	JobStatusViewed   JobStatus = "viewed"
	JobStatusSaved    JobStatus = "saved"
	JobStatusApplied  JobStatus = "applied"
	JobStatusRejected JobStatus = "rejected"
)

// ParseJobStatus converts a string to a JobStatus with validation.
// Returns the parsed status and a boolean indicating if it was valid.
// If invalid, the default JobStatusNew is returned with false.
func ParseJobStatus(statusStr string) (JobStatus, bool) {
	switch statusStr {
	case string(JobStatusNew):
		return JobStatusNew, true
	case string(JobStatusViewed):
		return JobStatusViewed, true
	case string(JobStatusSaved):
		return JobStatusSaved, true
	case string(JobStatusApplied):
		return JobStatusApplied, true
	case string(JobStatusRejected):
		return JobStatusRejected, true
	default:
		return JobStatusNew, false
	}
}

// UserJob represents a job that a user has interacted with
type UserJob struct {
	// ID is the same as the job_details document ID (provider_job_id)
	ID string `firestore:"id"`

	// Reference to the job_details document
	JobRef *firestore.DocumentRef `firestore:"job_ref"`

	// Analysis of the job match
	Analysis *JobAnalysis `firestore:"analysis"`

	// Summary of the job
	CompanyName    string   `firestore:"company_name"`
	CompanyLogo    string   `firestore:"company_logo"`
	Summary        string   `firestore:"summary"`
	Qualifications []string `firestore:"qualifications"`

	// User interaction metadata
	Status     JobStatus  `firestore:"status"` // "new", "viewed", "saved", "applied", "rejected"
	ViewedAt   *time.Time `firestore:"viewed_at,omitempty"`
	SavedAt    *time.Time `firestore:"saved_at,omitempty"`
	AppliedAt  *time.Time `firestore:"applied_at,omitempty"`
	RejectedAt *time.Time `firestore:"rejected_at,omitempty"`
	Notes      string     `firestore:"notes,omitempty"`

	// Search context
	SearchHash string    `firestore:"search_hash"`
	CreatedAt  time.Time `firestore:"created_at"`
	UpdatedAt  time.Time `firestore:"updated_at"`
}

type JobAnalysis struct {
	CandidateRequirementsAnalysis []RequirementAnalysis `json:"candidate_requirements_analysis"`
	JobRequirementsAnalysis       []RequirementAnalysis `json:"job_requirements_analysis"`
	Summary                       Summary               `json:"summary"`
	FinalJudgment                 FinalJudgment         `json:"final_judgment"`
}

type RequirementAnalysis struct {
	Requirement string `json:"requirement"`
	Fit         string `json:"fit"`
	Examples    string `json:"examples"`
}

type Summary struct {
	Matches []string `json:"matches"`
	Gaps    []string `json:"gaps"`
}

type FinalJudgment struct {
	Explanation string `json:"explanation"`
	Rating      string `json:"rating"` // One of: "Definitely Yes", "Strong Yes", "Yes", "Maybe", "No"
}

// SaveUserJob saves a job reference for a user
func SaveUserJob(ctx context.Context, fs *firestore.Client, userID string, job *UserJob) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}

	// Set timestamps
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Set default status if not provided
	if job.Status == "" {
		job.Status = JobStatusNew
	}

	// Create reference to job_details document if not set
	if job.JobRef == nil {
		job.JobRef = fs.Collection("job_details").Doc(job.ID)
	}

	// Save to user's jobs subcollection
	// Using job.ID as document ID ensures deduplication
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to save user job: %w", err)
	}

	return nil
}

func GetUserJob(ctx context.Context, fs *firestore.Client, userID, jobID string) (*UserJob, bool, error) {
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

// GetUserJobs retrieves all jobs for a user
func GetUserJobs(ctx context.Context, fs *firestore.Client, userID string) ([]*UserJob, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Documents(ctx)
	var jobs []*UserJob

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
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// UpdateUserJobStatus updates the status and corresponding timestamp of a user's job
func UpdateUserJobStatus(ctx context.Context, fs *firestore.Client, userID, jobID, status string) error {
	now := time.Now()
	updates := []firestore.Update{
		{Path: "status", Value: status},
		{Path: "updated_at", Value: now},
	}

	// Add timestamp based on status
	switch status {
	case string(JobStatusViewed):
		updates = append(updates, firestore.Update{Path: "viewed_at", Value: now})
	case string(JobStatusSaved):
		updates = append(updates, firestore.Update{Path: "saved_at", Value: now})
	case string(JobStatusApplied):
		updates = append(updates, firestore.Update{Path: "applied_at", Value: now})
	case string(JobStatusRejected):
		updates = append(updates, firestore.Update{Path: "rejected_at", Value: now})
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job status: %w", err)
	}

	return nil
}

// UpdateUserJobNotes updates the notes for a user's job
func UpdateUserJobNotes(ctx context.Context, fs *firestore.Client, userID, jobID, notes string) error {
	updates := []firestore.Update{
		{Path: "notes", Value: notes},
		{Path: "updated_at", Value: time.Now()},
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job notes: %w", err)
	}

	return nil
}

// GetUserJobsSorted retrieves user jobs with a specific status and supports paging
func GetUserJobsSorted(ctx context.Context, fs *firestore.Client, userID string, status JobStatus, pageSize int, pageToken string) ([]*UserJob, string, error) {
	// Create base query for jobs with the specified status and ordered by creation time
	baseQuery := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).
		Where("status", "==", string(status)).
		OrderBy("created_at", firestore.Asc)

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
		// There is a side effect that if the page size is evenly divisible by
		// the total number of jobs, we will be returning a value for `nextPageToke`
		// when in fact there are no more jobs to fetch.
		nextPageToken = jobs[len(jobs)-1].ID
	} else {
		nextPageToken = ""
	}

	return jobs, nextPageToken, nil
}

// GetJobsCount returns the count of jobs with the specified status for a user
func GetJobsCount(ctx context.Context, fs *firestore.Client, userID string, status JobStatus) (int64, error) {
	if userID == "" {
		return 0, fmt.Errorf("user ID is required")
	}

	query := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).
		Where("status", "==", string(status))

	// Create an aggregation query with a count using "count" as the alias
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
func GetAllJobStatusCounts(ctx context.Context, fs *firestore.Client, userID string) (map[string]int64, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Initialize the result map with all possible statuses set to 0
	result := map[string]int64{
		string(JobStatusNew):      0,
		string(JobStatusViewed):   0,
		string(JobStatusSaved):    0,
		string(JobStatusApplied):  0,
		string(JobStatusRejected): 0,
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
func UserHasAnyJobs(ctx context.Context, fs *firestore.Client, userID string) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user ID is required")
	}

	// Just get the first job document - if any exist
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Limit(1).Documents(ctx)

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
		"Viewed",
		"Saved",
		"Applied",
		"Rejected",
	}
}

// StatusToDisplayName converts a raw status string to a properly formatted display name
func StatusToDisplayName(status string) string {
	switch status {
	case string(JobStatusNew):
		return "New"
	case string(JobStatusViewed):
		return "Viewed"
	case string(JobStatusSaved):
		return "Saved"
	case string(JobStatusApplied):
		return "Applied"
	case string(JobStatusRejected):
		return "Rejected"
	default:
		return status
	}
}
