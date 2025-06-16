package internal

import (
	"context"
	"fmt"
	"time"

	"errors"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

type SearchTaskStatus string

const (
	SearchTaskCollection = "search_tasks"

	SearchTaskStatusQueued     SearchTaskStatus = "queued"
	SearchTaskStatusProcessing SearchTaskStatus = "processing"
	SearchTaskStatusCompleted  SearchTaskStatus = "completed"
	SearchTaskStatusFailed     SearchTaskStatus = "failed"
	SearchTaskStatusCancelled  SearchTaskStatus = "cancelled"
)

// SearchTask represents a queued job search with a provider.
type SearchTask struct {
	ID string
	// ADD: UserID to track who initiated the search for progress updates
	UserID *string
	// Snapshot of the search criteria used for this task
	Criteria       SearchCriteria
	Provider       string
	Status         SearchTaskStatus
	Error          *string
	PagesCompleted int
	CreatedAt      time.Time
	UpdatedAt      *time.Time
}

type JobDetailRef struct {
	JobID     string
	CreatedAt time.Time
}

var ErrSearchTaskExists = errors.New("search task already exists")

// Added: Define common errors for nil parameters
var (
	ErrFirestoreClientIsNil = errors.New("firestore client is nil")
	ErrTransactionIsNil     = errors.New("firestore transaction is nil")
	ErrSearchTaskIsNil      = errors.New("search task is nil")
)

func CreateSearchTask(ctx context.Context, fs *FirestoreClient, criteria SearchCriteria, provider string, userID *string) (*SearchTask, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	// Check for existing active tasks for this criteria
	activeTasks := fs.Collection(SearchTaskCollection).
		Where("Criteria.ID", "==", criteria.ID).
		Where("Status", "in", []SearchTaskStatus{SearchTaskStatusQueued, SearchTaskStatusProcessing})

	iter := activeTasks.Documents(ctx)
	defer iter.Stop()

	// If we find any document, there's already an active task
	if doc, err := iter.Next(); err != iterator.Done {
		if err != nil {
			return nil, fmt.Errorf("error checking for existing tasks: %w", err)
		}
		// Task already exists, return it instead of creating a new one
		existingTask := &SearchTask{}
		if err := doc.DataTo(existingTask); err != nil {
			return nil, fmt.Errorf("error parsing existing task: %w", err)
		}
		return existingTask, ErrSearchTaskExists
	}

	nowUTC := time.Now().UTC()
	jobSearch := &SearchTask{
		ID:             uuid.New().String(),
		UserID:         userID,
		Criteria:       criteria,
		Provider:       provider,
		Status:         SearchTaskStatusQueued,
		CreatedAt:      nowUTC,
		PagesCompleted: 0,
	}

	_, err := fs.Collection(SearchTaskCollection).Doc(jobSearch.ID).Set(ctx, jobSearch)
	return jobSearch, err
}

func CreateSearchTaskTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, criteria SearchCriteria, provider string, userID *string) (*SearchTask, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	if tx == nil {
		return nil, ErrTransactionIsNil
	}
	nowUTC := time.Now().UTC()
	jobSearch := &SearchTask{
		ID:        uuid.New().String(),
		UserID:    userID,
		Criteria:  criteria,
		Provider:  provider,
		Status:    SearchTaskStatusQueued,
		CreatedAt: nowUTC,
	}

	if err := tx.Set(fs.Collection(SearchTaskCollection).Doc(jobSearch.ID), jobSearch); err != nil {
		return nil, err
	}

	return jobSearch, nil
}

func UpdateSearchTask(ctx context.Context, fs *FirestoreClient, task *SearchTask) error {
	if fs == nil {
		return ErrFirestoreClientIsNil
	}
	if task == nil {
		return ErrSearchTaskIsNil
	}
	_, err := fs.Collection(SearchTaskCollection).Doc(task.ID).Set(ctx, task)
	if err != nil {
		return fmt.Errorf("UpdateSearchTask: firestore.Set: %w", err)
	}
	return nil
}

func UpdateSearchTaskTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, task *SearchTask) error {
	if fs == nil {
		return ErrFirestoreClientIsNil
	}
	if tx == nil {
		return ErrTransactionIsNil
	}
	if task == nil {
		return ErrSearchTaskIsNil
	}

	if err := tx.Set(fs.Collection(SearchTaskCollection).Doc(task.ID), task); err != nil {
		return fmt.Errorf("UpdateSearchTaskTx: firestore.Transaction.Set: %w", err)
	}
	return nil
}

func GetIncompleteSearchTasks(ctx context.Context, fs *FirestoreClient) ([]*SearchTask, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	iter := fs.Collection(SearchTaskCollection).Where("Status", "in", []SearchTaskStatus{SearchTaskStatusQueued, SearchTaskStatusProcessing}).Documents(ctx)

	searchTasks := []*SearchTask{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GetIncompleteSearchTasks: error iterating search tasks: %w", err)
		}
		searchTask := &SearchTask{}
		if err = doc.DataTo(searchTask); err != nil {
			return nil, fmt.Errorf("GetIncompleteSearchTasks: error parsing search task data: %w", err)
		}
		searchTasks = append(searchTasks, searchTask)
	}

	return searchTasks, nil
}

func SetSearchTaskStatus(ctx context.Context, fs *FirestoreClient, id string, status SearchTaskStatus) error {
	if fs == nil {
		return ErrFirestoreClientIsNil
	}
	now := time.Now().UTC()
	fsUpdates := []firestore.Update{
		{
			Path:  "Status",
			Value: status,
		},
		{
			Path:  "UpdatedAt",
			Value: now,
		},
	}

	_, err := fs.Collection(SearchTaskCollection).Doc(id).Update(ctx, fsUpdates)
	if err != nil {
		return fmt.Errorf("SetSearchTaskStatus: firestore.Update: %w", err)
	}
	return nil
}

// LinkJobDetailToSearchTaskTx adds a JobDetail as a subcollection doc under a SearchTask.
func LinkJobDetailToSearchTaskTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, searchTaskID string, jobID string) error {
	if fs == nil {
		return ErrFirestoreClientIsNil
	}
	if tx == nil {
		return ErrTransactionIsNil
	}
	if searchTaskID == "" || jobID == "" {
		return fmt.Errorf("searchTaskID and job.ID are required")
	}

	jobRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection).Doc(jobID)
	ref := JobDetailRef{
		JobID:     jobID,
		CreatedAt: time.Now().UTC(),
	}

	return tx.Set(jobRef, ref)
}

func GetJobIDsForSearchTask(ctx context.Context, fs *FirestoreClient, searchTaskID string) ([]string, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	if searchTaskID == "" {
		return nil, fmt.Errorf("searchTaskID is required")
	}
	colRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection)
	iter := colRef.Documents(ctx)
	var jobIDs []string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GetJobIDsForSearchTask: error iterating job details: %w", err)
		}
		var job JobDetailRef
		if err := doc.DataTo(&job); err != nil {
			return nil, fmt.Errorf("GetJobIDsForSearchTask: error parsing job detail: %w", err)
		}
		jobIDs = append(jobIDs, job.JobID)
	}
	return jobIDs, nil
}

func GetSearchTasksForCriteria(ctx context.Context, fs *FirestoreClient, criteriaID string) ([]SearchTask, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	if criteriaID == "" {
		return nil, fmt.Errorf("criteriaID is required")
	}

	iter := fs.Collection(SearchTaskCollection).Where("Criteria.ID", "==", criteriaID).Documents(ctx)

	var searchTasks []SearchTask
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		searchTask := SearchTask{}
		if err = doc.DataTo(&searchTask); err != nil {
			return nil, err
		}
		searchTasks = append(searchTasks, searchTask)
	}

	return searchTasks, nil
}

func DeleteSearchTaskTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, searchTaskID string) error {
	if fs == nil {
		return ErrFirestoreClientIsNil
	}
	if tx == nil {
		return ErrTransactionIsNil
	}
	if searchTaskID == "" {
		return fmt.Errorf("searchTaskID is required")
	}

	// Delete all job details subcollection documents
	jobDetailsCol := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection)
	iter := jobDetailsCol.Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error iterating job details: %w", err)
		}
		if err := tx.Delete(doc.Ref); err != nil {
			return fmt.Errorf("failed to delete job detail %s: %w", doc.Ref.ID, err)
		}
	}

	// Delete the main search task document
	if err := tx.Delete(fs.Collection(SearchTaskCollection).Doc(searchTaskID)); err != nil {
		return fmt.Errorf("failed to delete search task: %w", err)
	}

	return nil
}
