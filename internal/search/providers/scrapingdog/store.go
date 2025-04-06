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

	"cloud.google.com/go/firestore"
	"github.com/dgraph-io/ristretto/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// JobStore is responsible for managing job listings and caching them.
type JobStore struct {
	fs     *firestore.Client
	logger *slog.Logger
	cache  *ristretto.Cache[string, *search.JobDetail]
}

// NewJobStore creates a new JobStore instance.
func NewJobStore(ctx context.Context, logger *slog.Logger) (*JobStore, error) {
	logger.Debug("creating new job store")

	logger.Debug("creating firestore client")
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	logger.Debug("creating ristretto cache")
	cache, err := ristretto.NewCache(&ristretto.Config[string, *search.JobDetail]{
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

	logger.Debug("job store created successfully")
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
	js.logger.Debug("getting cached jobs",
		"keywords", criteria.Keywords,
		"location", criteria.Location)

	// Create a unique search hash for caching
	searchHash := js.GetSearchHash(criteria.Keywords, criteria.Location)
	js.logger.Debug("generated search hash", "hash", searchHash)

	// Get the search record first to check if we have cached results
	js.logger.Debug("checking search record", "hash", searchHash)
	searchDoc, err := js.fs.Collection("job_searches").Doc(searchHash).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			js.logger.Debug("no cache entry found", "hash", searchHash)
			return nil, nil // No cache entry found
		}
		return nil, fmt.Errorf("failed to get search record: %w", err)
	}

	// Check if the search is still fresh (within 4 hours)
	var searchData struct {
		LastSearchedAt time.Time `firestore:"last_searched_at"`
	}
	if err := searchDoc.DataTo(&searchData); err != nil {
		return nil, fmt.Errorf("failed to parse search data: %w", err)
	}

	if time.Since(searchData.LastSearchedAt) > 4*time.Hour {
		js.logger.Debug("cache is stale",
			"hash", searchHash,
			"last_searched_at", searchData.LastSearchedAt)
		return nil, nil // Cache is stale, return empty to trigger new search
	}

	// Get the job listings for this search
	js.logger.Debug("retrieving cached job listings", "hash", searchHash)
	iter := js.fs.Collection("job_listings").Where("search_hash", "==", searchHash).Documents(ctx)

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
		jobs = append(jobs, job)
	}

	js.logger.Debug("found cached jobs",
		"hash", searchHash,
		"num_jobs", len(jobs))
	return jobs, nil
}

// SaveJobListings saves job listings to Firestore.
func (js *JobStore) SaveJobListings(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	js.logger.Debug("saving job listings",
		"keywords", criteria.Keywords,
		"location", criteria.Location,
		"num_jobs", len(jobs))

	searchHash := js.GetSearchHash(criteria.Keywords, criteria.Location)
	js.logger.Debug("generated search hash", "hash", searchHash)

	// Delete existing job listings for this search hash
	js.logger.Debug("cleaning up old job listings", "hash", searchHash)
	oldJobs, err := js.fs.Collection("job_listings").Where("search_hash", "==", searchHash).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to get old job listings: %w", err)
	}
	for _, doc := range oldJobs {
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete old job listing: %w", err)
		}
	}

	// Update the search record
	js.logger.Debug("updating search record", "hash", searchHash)
	_, err = js.fs.Collection("job_searches").Doc(searchHash).Set(ctx, map[string]any{
		"keywords":         criteria.Keywords,
		"location":         criteria.Location,
		"last_searched_at": time.Now(),
	})
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

		js.logger.Debug("saving job listing",
			"job_id", job.ID,
			"provider_job_id", job.ProviderJobID)
		_, err := js.fs.Collection("job_listings").Doc(job.ID).Set(ctx, job)
		if err != nil {
			return fmt.Errorf("failed to save job listing: %w", err)
		}
	}

	js.logger.Debug("successfully saved all job listings",
		"hash", searchHash,
		"num_jobs", len(jobs))
	return nil
}

// GetCachedJobDetails retrieves cached job details from Ristretto cache
func (js *JobStore) GetCachedJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	js.logger.Debug("getting cached job details", "job_id", jobID)
	key := fmt.Sprintf("job_details:%s", jobID)

	details, found := js.cache.Get(key)
	if !found {
		js.logger.Debug("no cached job details found", "job_id", jobID)
		return nil, nil
	}

	js.logger.Debug("found cached job details", "job_id", jobID)
	return details, nil
}

// SaveJobDetails saves job details to both Firestore and Ristretto cache
func (js *JobStore) SaveJobDetails(ctx context.Context, details *search.JobDetail) error {
	js.logger.Debug("saving job details",
		"job_id", details.ID,
		"provider_job_id", details.ProviderJobID)

	// Save to Firestore
	js.logger.Debug("saving job details to firestore",
		"provider_job_id", details.ProviderJobID)
	_, err := js.fs.Collection("job_details").Doc(details.ProviderJobID).Set(ctx, details)
	if err != nil {
		return fmt.Errorf("failed to save job details to firestore: %w", err)
	}

	// Save to Ristretto cache with cost of 1
	js.logger.Debug("saving job details to ristretto cache",
		"provider_job_id", details.ProviderJobID)
	key := fmt.Sprintf("job_details:%s", details.ProviderJobID)
	js.cache.Set(key, details, 1)
	js.cache.Wait() // ensure the write is complete

	js.logger.Debug("successfully saved job details",
		"provider_job_id", details.ProviderJobID)
	return nil
}
