package internal

import (
	"context"
	"fmt"
	"time"

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
)

// SearchTask represents a queued job search with a provider.
type SearchTask struct {
	ID        string
	Criteria  SearchCriteria
	Provider  string
	Status    SearchTaskStatus
	Error     *string
	CreatedAt time.Time
	UpdatedAt *time.Time
}

func CreateSearchTask(ctx context.Context, fs *FirestoreClient, criteria SearchCriteria, provider string) (*SearchTask, error) {
	nowUTC := time.Now().UTC()
	jobSearch := &SearchTask{
		ID:        uuid.New().String(),
		Criteria:  criteria,
		Provider:  provider,
		Status:    SearchTaskStatusQueued,
		CreatedAt: nowUTC,
	}

	_, err := fs.Collection(SearchTaskCollection).Doc(jobSearch.ID).Set(ctx, jobSearch)
	return jobSearch, err
}

func UpdateSearchTask(ctx context.Context, fs *FirestoreClient, task *SearchTask) error {
	_, err := fs.Collection(SearchTaskCollection).Doc(task.ID).Set(ctx, task)
	return err
}

func GetIncompleteSearchTasks(ctx context.Context, fs *FirestoreClient) ([]*SearchTask, error) {
	iter := fs.Collection(SearchTaskCollection).Where("Status", "in", []SearchTaskStatus{SearchTaskStatusQueued, SearchTaskStatusProcessing}).Documents(ctx)

	searchTasks := []*SearchTask{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		searchTask := &SearchTask{}
		if err = doc.DataTo(searchTask); err != nil {
			return nil, err
		}
		searchTasks = append(searchTasks, searchTask)
	}

	return searchTasks, nil
}

func SetSearchTaskStatus(ctx context.Context, fs *FirestoreClient, id string, status SearchTaskStatus) error {
	fsUpdates := []firestore.Update{
		{
			Path:  "Status",
			Value: status,
		},
	}

	_, err := fs.Collection(SearchTaskCollection).Doc(id).Set(ctx, fsUpdates)
	return err
}

// AddJobDetailToSearch adds a JobDetail as a subcollection doc under a SearchTask.
func AddJobDetailToSearchTask(ctx context.Context, fs *FirestoreClient, searchTaskID string, job *JobDetail) error {
	if searchTaskID == "" || job == nil || job.ID == "" {
		return fmt.Errorf("searchTaskID and job.ID are required")
	}
	jobRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection).Doc(job.ID)
	_, err := jobRef.Set(ctx, job)
	return err
}

// AddJobDetailToSearchTaskTx adds a JobDetail as a subcollection doc under a SearchTask.
func AddJobDetailToSearchTaskTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, searchTaskID string, job *JobDetail) error {
	if searchTaskID == "" || job == nil || job.ID == "" {
		return fmt.Errorf("searchTaskID and job.ID are required")
	}
	jobRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection).Doc(job.ID)

	// Use the transaction to set the job detail
	if err := tx.Set(jobRef, job); err != nil {
		return err
	}

	return nil
}

func GetJobDetailsForSearchTask(ctx context.Context, fs *FirestoreClient, searchTaskID string) ([]JobDetail, error) {
	if searchTaskID == "" {
		return nil, fmt.Errorf("searchTaskID is required")
	}
	colRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection)
	iter := colRef.Documents(ctx)
	var jobDetails []JobDetail
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var job JobDetail
		if err := doc.DataTo(&job); err != nil {
			return nil, err
		}
		jobDetails = append(jobDetails, job)
	}
	return jobDetails, nil
}

// ListJobDetailsForSearch lists all JobDetail docs for a given SearchTask ID from the subcollection.
func ListJobDetailsForSearch(ctx context.Context, fs *FirestoreClient, searchTaskID string) ([]*JobDetail, error) {
	if searchTaskID == "" {
		return nil, fmt.Errorf("searchTaskID is required")
	}
	colRef := fs.Collection(SearchTaskCollection).Doc(searchTaskID).Collection(JobDetailsCollection)
	iter := colRef.Documents(ctx)
	var jobs []*JobDetail
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var job JobDetail
		if err := doc.DataTo(&job); err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}
	return jobs, nil
}
