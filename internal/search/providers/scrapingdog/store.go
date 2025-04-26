package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
	"time"

	"talent-rodeo/internal"

	"github.com/dgraph-io/ristretto/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	cacheDurationJobListing = 24 * time.Hour
)

// JobStore is responsible for managing job listings and caching them.
type JobStore struct {
	fs     *internal.FirestoreClient
	logger *slog.Logger
	cache  *ristretto.Cache[string, any]
}

// NewJobStore creates a new JobStore instance.
func NewJobStore(ctx context.Context, logger *slog.Logger) (*JobStore, error) {
	logger.Debug("scrapingdog: creating new job store")

	logger.Debug("scrapingdog: creating firestore client")
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	logger.Debug("scrapingdog: creating ristretto cache")
	cache, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M)
		MaxCost:     1 << 30, // maximum cost of cache (1GB)
		BufferItems: 64,      // number of keys per Get buffer
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ristretto cache: %w", err)
	}

	store := &JobStore{
		fs:     fs,
		logger: logger,
		cache:  cache,
	}

	logger.Debug("scrapingdog: job store created successfully")
	return store, nil
}

// GetSearchHash creates a unique hash for a search query
func (js *JobStore) GetSearchHash(keywords []string, location string) string {
	// Join all keywords with dash to create a unique combined search
	kw := strings.Join(keywords, "-")
	return funcs.Slugify(fmt.Sprintf("%s %s %s", kw, location, providerName))
}

// GetCachedJobs retrieves cached job listings based on the search criteria.
func (js *JobStore) GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	js.logger.Debug("scrapingdog: getting cached jobs",
		"keywords", criteria.Keywords,
		"location", criteria.Location)

	// Create a unique search hash for caching
	searchHash := js.GetSearchHash(criteria.Keywords, criteria.Location)
	js.logger.Debug("scrapingdog: generated search hash", "hash", searchHash)

	// First check search record to see if cache is still valid
	js.logger.Debug("scrapingdog: checking search record in Firestore", "hash", searchHash)
	searchDoc, err := js.fs.Collection("job_searches").Doc(searchHash).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			js.logger.Debug("scrapingdog: no cache entry found in Firestore", "hash", searchHash)
			return nil, nil // No cache entry found
		}
		return nil, fmt.Errorf("failed to get search record: %w", err)
	}

	// Check if the search is still fresh (within cacheDurationJobListing)
	var searchData struct {
		LastSearchedAt time.Time
	}
	if err := searchDoc.DataTo(&searchData); err != nil {
		return nil, fmt.Errorf("failed to parse search data: %w", err)
	}

	if time.Since(searchData.LastSearchedAt) > cacheDurationJobListing {
		js.logger.Debug("scrapingdog: cache is stale",
			"hash", searchHash,
			"LastSearchedAt", searchData.LastSearchedAt)
		// Clear Ristretto cache for this search
		key := fmt.Sprintf("search:%s", searchHash)
		js.cache.Del(key)
		// Return empty to trigger new search since the cache is stale
		return nil, nil
	}

	// Check Ristretto cache since search is still valid
	key := fmt.Sprintf("search:%s", searchHash)
	if cachedValue, found := js.cache.Get(key); found {
		if cachedJobs, ok := cachedValue.([]search.JobListing); ok {
			// Ensure SearchHash is set on all jobs
			for i := range cachedJobs {
				cachedJobs[i].SearchHash = searchHash
			}
			js.logger.Debug("scrapingdog: found jobs in Ristretto cache",
				"hash", searchHash,
				"num_jobs", len(cachedJobs))
			return cachedJobs, nil
		}
		js.logger.Warn("scrapingdog: invalid type in cache", "hash", searchHash)
	}

	// Get the job listings from Firestore
	js.logger.Debug("scrapingdog: retrieving cached job listings from Firestore", "hash", searchHash)
	iter := js.fs.Collection("job_listings").Where("SearchHash", "==", searchHash).Documents(ctx)

	var jobs []search.JobListing
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get job listings: %w", err)
		}
		var job search.JobListing
		if err := doc.DataTo(&job); err != nil {
			return nil, fmt.Errorf("failed to parse job data: %w", err)
		}
		// Ensure SearchHash is set
		job.SearchHash = searchHash
		jobs = append(jobs, job)
	}

	// Store in Ristretto cache for future use if we found jobs
	if len(jobs) > 0 {
		js.logger.Debug("scrapingdog: storing jobs in Ristretto cache",
			"hash", searchHash,
			"num_jobs", len(jobs))
		js.cache.Set(key, jobs, int64(len(jobs)))
		js.cache.Wait()
	}

	js.logger.Debug("scrapingdog: found cached jobs in Firestore",
		"hash", searchHash,
		"num_jobs", len(jobs))
	return jobs, nil
}

// GetCachedJobDetails retrieves cached job details from Ristretto or Firestore
func (js *JobStore) GetCachedJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	// Try Ristretto cache first
	key := fmt.Sprintf("job_details:%s", jobID)
	if value, found := js.cache.Get(key); found {
		if details, ok := value.(*search.JobDetail); ok {
			js.logger.Debug("scrapingdog: found job details in Ristretto cache", "job_id", jobID)
			return details, nil
		}
	}

	// Try Firestore if not in Ristretto
	js.logger.Debug("scrapingdog: checking job details in Firestore", "job_id", jobID)
	doc, err := js.fs.Collection("job_details").Doc(jobID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			js.logger.Debug("scrapingdog: no job details found in Firestore", "job_id", jobID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job details from firestore: %w", err)
	}

	// Parse the document to a JobDetail
	var details search.JobDetail
	if err := doc.DataTo(&details); err != nil {
		return nil, fmt.Errorf("failed to parse job details: %w", err)
	}

	// Cache in Ristretto for future use
	js.logger.Debug("scrapingdog: caching job details in Ristretto", "job_id", jobID)
	js.cache.Set(key, &details, 1)
	js.cache.Wait()

	return &details, nil
}

// SaveJobDetails saves job details to both Firestore and Ristretto cache
func (js *JobStore) SaveJobDetails(ctx context.Context, details *search.JobDetail) error {
	js.logger.Debug("scrapingdog: saving job details",
		"job_id", details.ID,
		"provider_job_id", details.ProviderJobID)

	// Ensure we have a proper ID for the job details
	if details.ID == "" {
		details.ID = funcs.Slugify(fmt.Sprintf("%s %s", details.Provider, details.ProviderJobID))
	}

	// Save to Firestore
	js.logger.Debug("scrapingdog: saving job details to firestore",
		"job_id", details.ID,
		"provider_job_id", details.ProviderJobID)
	_, err := js.fs.Collection("job_details").Doc(details.ID).Set(ctx, details)
	if err != nil {
		return fmt.Errorf("failed to save job details to firestore: %w", err)
	}

	// Save to Ristretto cache with cost of 1
	js.logger.Debug("scrapingdog: saving job details to ristretto cache",
		"job_id", details.ID,
		"provider_job_id", details.ProviderJobID)
	key := fmt.Sprintf("job_details:%s", details.ID)
	js.cache.Set(key, details, 1)
	js.cache.Wait() // ensure the write is complete

	js.logger.Debug("scrapingdog: successfully saved job details",
		"job_id", details.ID,
		"provider_job_id", details.ProviderJobID)
	return nil
}

// SaveJobListings saves job listings to Firestore and updates the Ristretto cache.
func (js *JobStore) SaveJobListings(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	searchHash := js.GetSearchHash(criteria.Keywords, criteria.Location)
	js.logger.Debug("scrapingdog: generated search hash", "hash", searchHash)

	// Delete existing job listings for this search hash
	js.logger.Debug("scrapingdog: cleaning up old job listings", "hash", searchHash)
	oldJobs, err := js.fs.Collection("job_listings").Where("SearchHash", "==", searchHash).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to get old job listings: %w", err)
	}
	for _, doc := range oldJobs {
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete old job listing: %w", err)
		}
	}

	// Update the search record
	now := time.Now()
	criteria.LastSearchedAt = &now
	js.logger.Debug("scrapingdog: updating search record", "hash", searchHash)
	_, err = js.fs.Collection("job_searches").Doc(searchHash).Set(ctx, criteria)
	if err != nil {
		return fmt.Errorf("failed to save search record: %w", err)
	}

	// Save each job listing
	for _, job := range jobs {
		if job.Provider == "" {
			return fmt.Errorf("provider is required")
		}

		if job.ProviderJobID == "" {
			return fmt.Errorf("provider job ID is required")
		}

		if job.ID == "" {
			job.ID = funcs.Slugify(fmt.Sprintf("%s %s", job.Provider, job.ProviderJobID))
		}

		job.SearchHash = searchHash
		job.FirstSeenAt = time.Now()

		js.logger.Debug("scrapingdog: saving job listing",
			"job_id", job.ID,
			"provider_job_id", job.ProviderJobID)
		_, err := js.fs.Collection("job_listings").Doc(job.ID).Set(ctx, job)
		if err != nil {
			return fmt.Errorf("failed to save job listing: %w", err)
		}
	}

	// Update Ristretto cache with the new job listings
	key := fmt.Sprintf("search:%s", searchHash)
	js.logger.Debug("scrapingdog: updating jobs in Ristretto cache",
		"hash", searchHash,
		"num_jobs", len(jobs))
	js.cache.Set(key, jobs, int64(len(jobs)))
	js.cache.Wait()

	js.logger.Debug("scrapingdog: successfully saved all job listings",
		"hash", searchHash,
		"num_jobs", len(jobs))
	return nil
}
