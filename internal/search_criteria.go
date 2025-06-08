package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ListingAge string
type WorkType string
type ExperienceLevel string

const (
	DefaultMaxPages          = 5
	searchCriteriaCollection = "search_criteria"

	ListingAgeDay   ListingAge = "day"   // Last 24 hours
	ListingAgeWeek  ListingAge = "week"  // Last 7 days
	ListingAgeMonth ListingAge = "month" // Last 30 days

	DefaultListingAge ListingAge = ListingAgeDay // Default to "day" if not specified

	WorkTypeAtWork WorkType = "at_work" // Jobs that require physical presence at the workplace
	WorkTypeRemote WorkType = "remote"  // Jobs that can be done from anywhere
	WorkTypeHybrid WorkType = "hybrid"  // Jobs that have a mix of remote and in-office work

	ExperienceLevelInternship     ExperienceLevel = "internship"
	ExperienceLevelEntryLevel     ExperienceLevel = "entry_level"
	ExperienceLevelAssociate      ExperienceLevel = "associate"
	ExperienceLevelMidSeniorLevel ExperienceLevel = "mid_senior_level"
	ExperienceLevelDirector       ExperienceLevel = "director"
)

// SearchCriteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type SearchCriteria struct {
	ID              string
	Query           string
	GeoID           string
	WorkType        WorkType        // e.g., "at_work", "remote", "hybrid"
	ExperienceLevel ExperienceLevel // e.g., "internship", "entry_level", "mid_senior_level", etc.
	MaxPages        int
	ListingAge      ListingAge // e.g., "day", "week", "month"
	Error           string
	LastSearchedAt  *time.Time
	LastSearchJobID string
	LastPublishedAt *time.Time
}

func (c SearchCriteria) GetSearchHash() string {
	type hashfields struct {
		Query           string
		GeoID           string
		JobType         string
		ExperienceLevel string
	}

	hf := hashfields{
		Query:           c.Query,
		GeoID:           c.GeoID,
		JobType:         string(c.WorkType),
		ExperienceLevel: string(c.ExperienceLevel),
	}

	// serialize the criteria to a json string and get the SHA256 hash
	json, err := json.Marshal(hf)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(json))
}

func MakeSearchCriteriaID(query, geoID string, workType WorkType, expLevel ExperienceLevel) string {
	return funcs.Slugify(fmt.Sprintf("%s-%s-%s-%s", query, geoID, workType, expLevel))
}

func CreateSearchCriteria(ctx context.Context, fs *FirestoreClient, query, geoID string, workType WorkType, expLevel ExperienceLevel, maxPages int, age ListingAge) (*SearchCriteria, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if geoID == "" {
		return nil, fmt.Errorf("geoID cannot be empty")
	}
	if maxPages <= 0 {
		maxPages = DefaultMaxPages // Use default if not specified
	}
	if age == "" {
		age = DefaultListingAge // Use default if not specified
	}
	if workType == "" {
		return nil, fmt.Errorf("workType cannot be empty")
	}
	if expLevel == "" {
		return nil, fmt.Errorf("experienceLevel cannot be empty")
	}
	criteria := &SearchCriteria{
		ID:              MakeSearchCriteriaID(query, geoID, workType, expLevel),
		Query:           query,
		GeoID:           geoID,
		MaxPages:        maxPages,
		ListingAge:      age,
		WorkType:        workType,
		ExperienceLevel: expLevel,
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
// been searched in the last 24 hours, and don't already have incomplete search tasks.
func GetSearchCriteriaNeedsSearch(ctx context.Context, fs *FirestoreClient) ([]SearchCriteria, error) {
	ids, err := ListSubscribedCriteriaIDs(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscribed criteria IDs: %w", err)
	}
	if len(ids) == 0 {
		// No subscribed criteria, return empty slice
		return []SearchCriteria{}, nil
	}

	// Get incomplete search tasks to exclude criteria that are already being processed
	incompleteTasks, err := GetIncompleteSearchTasks(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to get incomplete search tasks: %w", err)
	}

	// Create a set of criteria IDs that already have incomplete tasks
	excludedCriteriaIDs := make(map[string]bool)
	for _, task := range incompleteTasks {
		excludedCriteriaIDs[task.Criteria.ID] = true
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

		// Skip criteria that already have incomplete search tasks
		if excludedCriteriaIDs[criteria.ID] {
			continue
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
	// Query the subcollection under search_criteria/{criteriaID}/job_details/
	iter := fs.Collection(searchCriteriaCollection).Doc(criteriaID).Collection(JobDetailsCollection).Documents(ctx)
	defer iter.Stop()

	var jobs []string
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			return nil, fmt.Errorf("failed iterating job_details subcollection: %w", err)
		}

		var link JobDetailCriteriaLink
		if err := doc.DataTo(&link); err != nil {
			return nil, fmt.Errorf("failed to parse job detail link: %w", err)
		}
		jobs = append(jobs, link.JobID)
	}

	return jobs, nil
}

func GetJobIDsForCriteriaCreatedAfter(ctx context.Context, fs *FirestoreClient, criteriaID string, createdAfter time.Time) ([]string, error) {
	// Query the subcollection under search_criteria/{criteriaID}/job_details/ with CreatedAt filter
	iter := fs.Collection(searchCriteriaCollection).Doc(criteriaID).Collection(JobDetailsCollection).WhereEntity(firestore.PropertyFilter{Path: "CreatedAt", Operator: ">", Value: createdAfter}).Documents(ctx)
	defer iter.Stop()

	var jobs []string
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // End of results
			}
			return nil, fmt.Errorf("failed iterating job_details subcollection: %w", err)
		}

		var link JobDetailCriteriaLink
		if err := doc.DataTo(&link); err != nil {
			return nil, fmt.Errorf("failed to parse job detail link: %w", err)
		}
		jobs = append(jobs, link.JobID)
	}

	return jobs, nil
}

func UpdateSearchCriteria(ctx context.Context, fs *FirestoreClient, criteria *SearchCriteria) error {
	_, err := fs.Collection(searchCriteriaCollection).Doc(criteria.ID).Set(ctx, criteria)
	if err != nil {
		return fmt.Errorf("UpdateSearchCriteria: firestore.Set: %w", err)
	}
	return nil
}

// DeleteSearchCriteria deletes a search criteria and all related data:
// - Unsubscribes all users from the criteria
// - Unlinks all job details from the criteria
// - Deletes all search tasks associated with the criteria
func DeleteSearchCriteria(ctx context.Context, fs *FirestoreClient, id string) error {
	subscribers, err := GetCriteriaSubscribers(ctx, fs, id)
	if err != nil {
		return fmt.Errorf("failed to get subscribers for search criteria ID %s: %w", id, err)
	}

	jobIDs, err := GetJobIDsForCriteria(ctx, fs, id)
	if err != nil {
		return fmt.Errorf("failed to get job IDs for search criteria ID %s: %w", id, err)
	}

	searchTasks, err := GetSearchTasksForCriteria(ctx, fs, id)
	if err != nil {
		return fmt.Errorf("failed to get search tasks for search criteria ID %s: %w", id, err)
	}

	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {

		for _, sub := range subscribers {
			err = UnsubscribeFromCriteriaTx(ctx, fs, tx, sub.UserID, id)
			if err != nil {
				return fmt.Errorf("failed to unsubscribe user %s from search criteria ID %s: %w", sub.UserID, id, err)
			}
		}

		for _, jobID := range jobIDs {
			err = UnlinkJobDetailFromCriteriaTx(ctx, fs, tx, id, jobID)
			if err != nil {
				return fmt.Errorf("failed to unlink job ID %s from search criteria ID %s: %w", jobID, id, err)
			}

		}

		for _, searchTask := range searchTasks {
			err = DeleteSearchTaskTx(ctx, fs, tx, searchTask.ID)
			if err != nil {
				return fmt.Errorf("failed to delete search task ID %s for search criteria ID %s: %w", searchTask.ID, id, err)
			}
		}

		if len(jobIDs) > 0 {
			denseHost := os.Getenv("PINECONE_DENSE_INDEX_HOST")
			if denseHost == "" {
				return fmt.Errorf("PINECONE_DENSE_INDEX_HOST is not set")
			}

			sparseHost := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
			if sparseHost == "" {
				return fmt.Errorf("PINECONE_SPARSE_INDEX_HOST is not set")
			}

			pc, err := NewPineconeClient()
			if err != nil {
				return fmt.Errorf("failed to create Pinecone client: %w", err)
			}
			if err := pc.DeleteRecords(ctx, denseHost, "job-details", jobIDs); err != nil {
				return fmt.Errorf("failed to delete records from Pinecone: %w", err)
			}
			if err := pc.DeleteRecords(ctx, sparseHost, "job-details", jobIDs); err != nil {
				return fmt.Errorf("failed to delete records from Pinecone: %w", err)
			}
		}

		err = tx.Delete(fs.Collection(searchCriteriaCollection).Doc(id))
		if err != nil {
			return fmt.Errorf("failed to delete search criteria ID %s: %w", id, err)
		}

		return nil
	})
}
