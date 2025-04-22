package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/search"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	jobListingsCollection = "job_listings"
	jobDetailsCollection  = "job_details"
	userJobsCollection    = "user_jobs"
	jobSearchCacheTimeout = 24 * time.Hour
)

// FirestoreStore implements the JobStore interface using Firestore
type FirestoreStore struct {
	fs           *internal.FirestoreClient
	logger       *slog.Logger
	cacheEnabled bool
	cacheTimeout time.Duration
}

// StoreOption is a function type for configuring the store
type StoreOption func(*FirestoreStore)

// WithCacheEnabled enables or disables caching
func WithCacheEnabled(enabled bool) StoreOption {
	return func(s *FirestoreStore) {
		s.cacheEnabled = enabled
	}
}

// WithCacheTimeout sets the cache timeout duration
func WithCacheTimeout(timeout time.Duration) StoreOption {
	return func(s *FirestoreStore) {
		s.cacheTimeout = timeout
	}
}

// WithStoreLogger sets a custom logger for the store
func WithStoreLogger(logger *slog.Logger) StoreOption {
	return func(s *FirestoreStore) {
		s.logger = logger
	}
}

// NewFirestoreStore creates a new Firestore-backed job store
func NewFirestoreStore(fs *internal.FirestoreClient, opts ...StoreOption) *FirestoreStore {
	store := &FirestoreStore{
		fs:           fs,
		logger:       slog.Default(),
		cacheEnabled: true,
		cacheTimeout: jobSearchCacheTimeout,
	}

	for _, opt := range opts {
		opt(store)
	}

	return store
}

// // GenerateSearchHash creates a deterministic hash for search criteria
// func (s *FirestoreStore) GenerateSearchHash(criteria search.Criteria) string {
// 	// Create a normalized representation of the search criteria
// 	data := struct {
// 		Keywords   []string `json:"keywords"`
// 		Location   string   `json:"location"`
// 		JobType    []string `json:"job_type"`
// 		Experience []string `json:"experience"`
// 		WorkType   string   `json:"work_type"`
// 	}{
// 		Keywords:   criteria.Keywords,
// 		Location:   criteria.Location,
// 		JobType:    criteria.JobType,
// 		Experience: criteria.Experience,
// 		WorkType:   criteria.WorkType,
// 	}

// 	// Sort slices for deterministic output
// 	sort.Strings(data.Keywords)
// 	sort.Strings(data.JobType)
// 	sort.Strings(data.Experience)

// 	// Marshal to JSON
// 	jsonData, err := json.Marshal(data)
// 	if err != nil {
// 		// Fallback to simple string concatenation if JSON marshaling fails
// 		return hashString(strings.Join(criteria.Keywords, "+") +
// 			criteria.Location +
// 			strings.Join(criteria.JobType, "+") +
// 			strings.Join(criteria.Experience, "+") +
// 			criteria.WorkType)
// 	}

// 	// Return MD5 hash of the JSON string
// 	return hashString(string(jsonData))
// }

// // hashString creates an MD5 hash of a string
// func hashString(s string) string {
// 	hash := md5.Sum([]byte(s))
// 	return hex.EncodeToString(hash[:])
// }

// GetCachedJobs retrieves jobs from cache for the given criteria
func (s *FirestoreStore) GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	if !s.cacheEnabled {
		return nil, nil
	}

	searchHash := criteria.GetSearchHash()
	s.logger.InfoContext(ctx, "Looking up cached jobs", "hash", searchHash)

	// Get Firestore document reference for this search hash
	docRef := s.fs.Collection(jobListingsCollection).Doc(searchHash)

	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Cache miss is not an error
			s.logger.InfoContext(ctx, "No cached jobs found", "hash", searchHash)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get cached jobs: %w", err)
	}

	// Parse document data
	var data struct {
		Jobs      []search.JobListing `firestore:"jobs"`
		CreatedAt time.Time           `firestore:"created_at"`
	}

	if err := doc.DataTo(&data); err != nil {
		return nil, fmt.Errorf("failed to parse cached jobs: %w", err)
	}

	// Check if cache has expired
	if time.Since(data.CreatedAt) > s.cacheTimeout {
		s.logger.InfoContext(ctx, "Cache expired",
			"hash", searchHash,
			"age", time.Since(data.CreatedAt))
		return nil, nil
	}

	s.logger.InfoContext(ctx, "Found cached jobs",
		"hash", searchHash,
		"count", len(data.Jobs))

	return data.Jobs, nil
}

// SaveJobs stores jobs in the cache for the given criteria
func (s *FirestoreStore) SaveJobs(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	if !s.cacheEnabled || len(jobs) == 0 {
		return nil
	}

	searchHash := criteria.GetSearchHash()
	s.logger.InfoContext(ctx, "Saving jobs to cache",
		"hash", searchHash,
		"count", len(jobs))

	// Set created timestamp on each job listing
	now := time.Now()
	for i := range jobs {
		if jobs[i].FirstSeenAt.IsZero() {
			jobs[i].FirstSeenAt = now
		}
		jobs[i].SearchHash = searchHash
	}

	// Get Firestore document reference for this search hash
	docRef := s.fs.Collection(jobListingsCollection).Doc(searchHash)

	// Data to save
	data := map[string]interface{}{
		"jobs":       jobs,
		"created_at": now,
		"keywords":   criteria.Query,
		"location":   criteria.Location,
		"job_type":   criteria.JobType,
		"experience": criteria.Experience,
		"work_type":  criteria.WorkType,
		"provider":   ProviderName,
	}

	// Save to Firestore
	_, err := docRef.Set(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to save jobs to cache: %w", err)
	}

	s.logger.InfoContext(ctx, "Successfully saved jobs to cache",
		"hash", searchHash,
		"count", len(jobs))

	return nil
}

// GetJobDetails retrieves job details from the store
func (s *FirestoreStore) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	s.logger.InfoContext(ctx, "Getting job details from store", "jobID", jobID)

	// Get Firestore document reference
	docRef := s.fs.Collection(jobDetailsCollection).Doc(jobID)

	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Not found is expected and not an error
			s.logger.InfoContext(ctx, "Job details not found in store", "jobID", jobID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job details: %w", err)
	}

	// Parse document data
	var detail search.JobDetail
	if err := doc.DataTo(&detail); err != nil {
		return nil, fmt.Errorf("failed to parse job details: %w", err)
	}

	s.logger.InfoContext(ctx, "Found job details in store",
		"jobID", jobID,
		"title", detail.Title)

	return &detail, nil
}

// SaveJobDetails stores job details
func (s *FirestoreStore) SaveJobDetails(ctx context.Context, jobDetail search.JobDetail) error {
	s.logger.InfoContext(ctx, "Saving job details to store",
		"jobID", jobDetail.ID,
		"title", jobDetail.Title)

	// Set created time if not already set
	if jobDetail.CreatedAt.IsZero() {
		jobDetail.CreatedAt = time.Now()
	}

	// Get Firestore document reference
	docRef := s.fs.Collection(jobDetailsCollection).Doc(jobDetail.ID)

	// Save to Firestore
	_, err := docRef.Set(ctx, jobDetail)
	if err != nil {
		return fmt.Errorf("failed to save job details: %w", err)
	}

	s.logger.InfoContext(ctx, "Successfully saved job details",
		"jobID", jobDetail.ID)

	return nil
}

// CreateUserJob creates a job entry for a specific user
// func (s *FirestoreStore) CreateUserJob(ctx context.Context, userID string, searchHash string, job search.JobListing, details *search.JobDetail) error {
// 	if details == nil {
// 		return fmt.Errorf("cannot create user job with nil details")
// 	}

// 	s.logger.InfoContext(ctx, "Creating user job",
// 		"userID", userID,
// 		"jobID", job.ID,
// 		"title", job.Title)

// 	// Generate a unique ID for this user-job combination
// 	userJobID := funcs.Slugify(fmt.Sprintf("%s %s", userID, job.ID))

// 	// Build user job data
// 	userJob := map[string]interface{}{
// 		"id":          userJobID,
// 		"user_id":     userID,
// 		"job_id":      job.ID,
// 		"title":       job.Title,
// 		"company":     job.Company,
// 		"location":    job.Location,
// 		"url":         job.URL,
// 		"description": details.FullDescription,
// 		"posted_date": job.PostedDate,
// 		"provider":    job.Provider,
// 		"search_hash": searchHash,
// 		"status":      "new",
// 		"created_at":  time.Now(),
// 		"salary_info": details.SalaryInfo,
// 		"job_type":    details.EmploymentType,
// 		"work_type":   details.WorkplaceType,
// 	}

// 	// Create user job in Firestore
// 	_, err := s.fs.Collection(userJobsCollection).Doc(userJobID).Set(ctx, userJob)
// 	if err != nil {
// 		return fmt.Errorf("failed to create user job: %w", err)
// 	}

// 	// Note: We're not incrementing the user's job count here since the function doesn't exist
// 	// When this code is used in production, a proper way to update the user's job count should be implemented

// 	s.logger.InfoContext(ctx, "Successfully created user job",
// 		"userID", userID,
// 		"jobID", job.ID)

// 	return nil
// }
