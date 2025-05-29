package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ListingAge string

const (
	DefaultMaxPages          = 5
	searchCriteriaCollection = "search_criteria"

	ListingAgeDay   ListingAge = "day"   // Last 24 hours
	ListingAgeWeek  ListingAge = "week"  // Last 7 days
	ListingAgeMonth ListingAge = "month" // Last 30 days

	DefaultListingAge ListingAge = ListingAgeDay // Default to "day" if not specified
)

// SearchCriteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type SearchCriteria struct {
	ID              string
	Query           string
	GeoID           string
	JobType         []string
	Experience      []string
	WorkType        *string
	MaxPages        int
	ListingAge      ListingAge // e.g., "day", "week", "month"
	Error           string
	SubscriberCount int
	LastSearchedAt  *time.Time
	LastSearchJobID string
	LastPublishedAt *time.Time
}

func (c SearchCriteria) GetSearchHash() string {
	// serialize the criteria to a json string and get the SHA256 hash
	json, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(json))
}

func MakeSearchCriteriaID(query string, geoID string) string {
	return funcs.Slugify(fmt.Sprintf("%s-%s", query, geoID))
}

func CreateSearchCriteria(ctx context.Context, fs *FirestoreClient, query, geoID string, maxPages int, age ListingAge) (*SearchCriteria, error) {
	criteria := &SearchCriteria{
		ID:              MakeSearchCriteriaID(query, geoID),
		Query:           query,
		GeoID:           geoID,
		MaxPages:        maxPages,
		ListingAge:      age,
		SubscriberCount: 0,
	}

	_, err := fs.Collection(searchCriteriaCollection).Doc(criteria.ID).Create(ctx, criteria)
	if err != nil {
		return nil, err
	}

	return criteria, nil
}

func GetSearchCriteria(ctx context.Context, fs *FirestoreClient, id string) (*SearchCriteria, bool, error) {
	criteria := &SearchCriteria{}

	doc, err := fs.Collection(searchCriteriaCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, err
	}

	err = doc.DataTo(criteria)
	if err != nil {
		return nil, false, err
	}

	return criteria, true, nil
}

// GetSearchCriteriaNeedsSearch retrieves all search criteria from Firestore
// that either have never been searched (LastSearchedAt is nil) or haven't
// been searched in the last 24 hours.
func GetSearchCriteriaNeedsSearch(ctx context.Context, fs *FirestoreClient) ([]SearchCriteria, error) {
	ids, err := ListSubscribedCriteriaIDs(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscribed criteria IDs: %w", err)
	}
	if len(ids) == 0 {
		// No subscribed criteria, return empty slice
		return []SearchCriteria{}, nil
	}

	coll := fs.Collection(searchCriteriaCollection)
	// Filter the collection to only include criteria that are subscribed
	baseQuery := coll.WhereEntity(firestore.PropertyFilter{Path: "ID", Operator: "in", Value: ids})

	// Calculate 24 hours ago
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)

	// Add condition for search timing using an OR filter
	q := baseQuery.WhereEntity(firestore.OrFilter{
		Filters: []firestore.EntityFilter{
			firestore.PropertyFilter{Path: "LastSearchedAt", Operator: "==", Value: nil},
			firestore.PropertyFilter{Path: "LastSearchedAt", Operator: "<", Value: twentyFourHoursAgo},
		},
	})

	iter := q.Documents(ctx)
	defer iter.Stop() // Ensure the iterator is properly closed

	var items []SearchCriteria
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			// Return any other error encountered during iteration
			return nil, fmt.Errorf("failed iterating search criteria: %w", err)
		}

		var criteria SearchCriteria
		if err := doc.DataTo(&criteria); err != nil {
			// Log or handle documents that fail to parse, maybe continue?
			// For now, return the error to avoid partial results.
			return nil, fmt.Errorf("failed parsing criteria data (ID: %s): %w", doc.Ref.ID, err)
		}
		items = append(items, criteria)
	}

	return items, nil
}

// ListSearchCriteria retrieves all search criteria from Firestore
func ListSearchCriteria(ctx context.Context, fs *FirestoreClient) ([]SearchCriteria, error) {
	coll := fs.Collection(searchCriteriaCollection)

	// Query all documents in the collection
	iter := coll.Documents(ctx)
	defer iter.Stop() // Ensure the iterator is properly closed

	var items []SearchCriteria
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			// Return any other error encountered during iteration
			return nil, fmt.Errorf("failed iterating search criteria: %w", err)
		}

		var criteria SearchCriteria
		if err := doc.DataTo(&criteria); err != nil {
			// Log or handle documents that fail to parse, maybe continue?
			// For now, return the error to avoid partial results.
			return nil, fmt.Errorf("failed parsing criteria data (ID: %s): %w", doc.Ref.ID, err)
		}
		items = append(items, criteria)
	}

	return items, nil
}

func GetJobIDsForCriteria(ctx context.Context, fs *FirestoreClient, criteriaID string) ([]string, error) {
	iter := fs.Collection(JobCriteriaLinkCollection).WhereEntity(firestore.PropertyFilter{Path: "CriteriaID", Operator: "==", Value: criteriaID}).Documents(ctx)
	defer iter.Stop()

	var jobs []string
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			return nil, fmt.Errorf("failed iterating job_criteria_links: %w", err)
		}
		jobs = append(jobs, doc.Data()["JobID"].(string))
	}

	return jobs, nil
}

func GetJobIDsForCriteriaCreatedAfter(ctx context.Context, fs *FirestoreClient, criteriaID string, createdAfter time.Time) ([]string, error) {
	iter := fs.Collection(JobCriteriaLinkCollection).WhereEntity(firestore.PropertyFilter{Path: "CriteriaID", Operator: "==", Value: criteriaID}).WhereEntity(firestore.PropertyFilter{Path: "CreatedAt", Operator: ">", Value: createdAfter}).Documents(ctx)
	defer iter.Stop()

	var jobs []string
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			return nil, fmt.Errorf("failed iterating job_criteria_links: %w", err)
		}
		jobs = append(jobs, doc.Data()["JobID"].(string))
	}

	return jobs, nil
}

func UpdateSearchCriteria(ctx context.Context, fs *FirestoreClient, criteria *SearchCriteria) error {
	_, err := fs.Collection(searchCriteriaCollection).Doc(criteria.ID).Set(ctx, criteria)
	if err != nil {
		return err
	}
	return nil
}

func DeleteSearchCriteria(ctx context.Context, fs *FirestoreClient, id string) error {
	_, err := fs.Collection(searchCriteriaCollection).Doc(id).Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}
