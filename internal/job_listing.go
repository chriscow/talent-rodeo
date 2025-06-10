package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const (
	JobListingCollection = "job_listings"
)

// JobListing represents a basic job listing.
// This is the minimal set of fields returned from a search operation.
// More detailed information is available via GetJobDetails.
type JobListing struct {
	ID             string
	Provider       string
	ProviderJobID  string // Original ID from the provider
	Title          string // maps to Position in ScrapingDog
	Company        string // maps to CompanyName in ScrapingDog
	Location       string
	PostedDate     time.Time
	Description    string // Brief description or snippet if available. ScrapingDog does not return this.
	URL            string // Provider-specific URL, maps to Link in ScrapingDog
	CompanyProfile string
	CompanyLogo    string

	SearchHash  string
	SearchJobID string
	CreatedAt   time.Time
}

func (j *JobListing) GetCacheKey() string {
	return fmt.Sprintf("%s:%s", j.Provider, j.ProviderJobID)
}

func SaveJobListing(ctx context.Context, fs *FirestoreClient, jobSearch SearchTask, jobListing *JobListing) error {
	if fs == nil {
		return fmt.Errorf("SaveJobListing: FirestoreClient is nil")
	}
	if jobListing == nil {
		return fmt.Errorf("SaveJobListing: jobListing is nil")
	}
	if fs.jobListingCache == nil {
		return fmt.Errorf("SaveJobListing: jobListingCache is nil")
	}
	if jobListing.ID == "" {
		jobListing.ID = jobListing.GetCacheKey()
	}

	if jobListing.SearchHash == "" {
		jobListing.SearchHash = jobSearch.Criteria.GetSearchHash()
	}

	if jobListing.SearchJobID == "" {
		jobListing.SearchJobID = jobSearch.ID
	}

	if jobListing.CreatedAt.IsZero() {
		jobListing.CreatedAt = time.Now()
	}

	_, err := fs.Collection(JobListingCollection).Doc(jobListing.ID).Set(ctx, jobListing)
	if err != nil {
		return fmt.Errorf("SaveJobListing: firestore.Set: %w", err)
	}

	fs.jobListingCache.SetWithTTL(jobListing.ID, jobListing, 1, 1*time.Hour)
	return nil
}

func GetJobListing(ctx context.Context, fs *FirestoreClient, id string) (*JobListing, error) {
	if fs == nil {
		return nil, fmt.Errorf("GetJobListing: FirestoreClient is nil")
	}
	if fs.jobListingCache == nil {
		return nil, fmt.Errorf("GetJobListing: jobListingCache is nil")
	}
	if id == "" {
		return nil, fmt.Errorf("GetJobListing: id is empty")
	}
	if jobListing, ok := fs.jobListingCache.Get(id); ok {
		return jobListing, nil
	}

	doc, err := fs.Collection(JobListingCollection).Doc(id).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetJobListing: firestore.Get: %w", err)
	}

	var jobListing JobListing
	if err := doc.DataTo(&jobListing); err != nil {
		return nil, fmt.Errorf("GetJobListing: firestore.DataTo: %w", err)
	}

	return &jobListing, nil
}

// ListJobListings retrieves all job listings with optional limit and offset
func ListJobListings(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]map[string]any, error) {
	if fs == nil {
		return nil, fmt.Errorf("ListJobListings: FirestoreClient is nil")
	}
	if limit < 0 {
		return nil, fmt.Errorf("ListJobListings: limit cannot be negative")
	}
	if offset < 0 {
		return nil, fmt.Errorf("ListJobListings: offset cannot be negative")
	}
	query := fs.Collection("job_listings").OrderBy("CreatedAt", firestore.Desc)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	iter := query.Documents(ctx)
	var listings []map[string]any

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate job listings: %w", err)
		}

		listings = append(listings, doc.Data())
	}

	return listings, nil
}

// DeleteJobListingsByPrefix deletes all job listings with IDs starting with the given prefix
func DeleteJobListingsByPrefix(ctx context.Context, fs *FirestoreClient, prefix string) (int, error) {
	if fs == nil {
		return 0, fmt.Errorf("DeleteJobListingsByPrefix: FirestoreClient is nil")
	}
	if prefix == "" {
		return 0, fmt.Errorf("prefix is required")
	}

	// Query for documents with IDs starting with the prefix
	// We can't directly query by document ID prefix, so we'll need to fetch and filter
	iter := fs.Collection("job_listings").Documents(ctx)
	deleted := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return deleted, fmt.Errorf("failed to iterate job listings: %w", err)
		}

		// Check if document ID starts with prefix
		if len(doc.Ref.ID) >= len(prefix) && doc.Ref.ID[:len(prefix)] == prefix {
			if _, err := doc.Ref.Delete(ctx); err != nil {
				return deleted, fmt.Errorf("failed to delete job listing %s: %w", doc.Ref.ID, err)
			}
			deleted++
		}
	}

	return deleted, nil
}
