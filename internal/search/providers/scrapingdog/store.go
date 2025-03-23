package scrapingdog

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// JobStore is responsible for managing job listings and caching them.
type JobStore struct {
	fs     *firestore.Client
	logger *slog.Logger
}

// NewJobStore creates a new JobStore instance.
func NewJobStore(ctx context.Context, logger *slog.Logger) (*JobStore, error) {
	opt := option.WithCredentialsFile(os.Getenv("FIREBASE_CREDENTIALS_FILE"))
	config := &firebase.Config{ProjectID: os.Getenv("FIREBASE_PROJECT_ID")}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	store := &JobStore{
		fs:     fs,
		logger: logger,
	}

	return store, nil
}

// getSearchHash creates a unique hash for a search query
func (js *JobStore) getSearchHash(keywords []string, location string) string {
	// Join all keywords with dash to create a unique combined search
	kw := strings.Join(keywords, "-")
	return funcs.Slugify(fmt.Sprintf("%s %s %s", kw, location, providerName))
}

// GetCachedJobs retrieves cached job listings based on the search criteria.
func (js *JobStore) GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	log.Printf("Getting cached jobs for criteria: %+v", criteria)

	// Create a unique search hash for caching
	searchHash := js.getSearchHash(criteria.Keywords, criteria.Location)

	// Get the search record first to check if we have cached results
	searchDoc, err := js.fs.Collection("job_searches").Doc(searchHash).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
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
		return nil, nil // Cache is stale, return empty to trigger new search
	}

	// Get the job listings for this search
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

	log.Printf("Found %d cached jobs", len(jobs))
	return jobs, nil
}

// SaveJobListings saves job listings to Firestore.
func (js *JobStore) SaveJobListings(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	log.Printf("Saving %d jobs for criteria: %+v", len(jobs), criteria)

	searchHash := js.getSearchHash(criteria.Keywords, criteria.Location)

	// Update the search record
	_, err := js.fs.Collection("job_searches").Doc(searchHash).Set(ctx, map[string]interface{}{
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
		_, err := js.fs.Collection("job_listings").Doc(job.ID).Set(ctx, job)
		if err != nil {
			return fmt.Errorf("failed to save job listing: %w", err)
		}
	}

	return nil
}
