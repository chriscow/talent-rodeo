package scrapingdog

/*
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

type JobListingSearch struct {
	Criteria  search.Criteria
	Jobs      []search.JobListing
	CreatedAt time.Time
}

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

	var data JobListingSearch
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
func (s *FirestoreStore) SaveJobs(ctx context.Context, criteria search.Criteria, listings []search.JobListing) error {
	if !s.cacheEnabled || len(listings) == 0 {
		return nil
	}

	searchHash := criteria.GetSearchHash()
	s.logger.InfoContext(ctx, "Saving jobs to cache",
		"hash", searchHash,
		"count", len(listings))

	// Set created timestamp on each job listing
	now := time.Now()
	for i := range listings {
		if listings[i].FirstSeenAt.IsZero() {
			listings[i].FirstSeenAt = now
		}
		listings[i].SearchHash = searchHash
	}

	// Get Firestore document reference for this search hash
	docRef := s.fs.Collection(jobListingsCollection).Doc(searchHash)

	// Save to Firestore
	_, err := docRef.Set(ctx, JobListingSearch{
		Criteria:  criteria,
		Jobs:      listings,
		CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("failed to save jobs to cache: %w", err)
	}

	s.logger.InfoContext(ctx, "Successfully saved jobs to cache",
		"hash", searchHash,
		"count", len(listings))

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
*/
