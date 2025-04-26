package tasks

import (
	"context"
	"fmt"
	"time"

	"talent-rodeo/internal"
	// "talent-rodeo/internal/firestore"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/iterator"
)

// TaskFilter represents the filtering options for retrieving tasks
type TaskFilter struct {
	Status string // Only status filtering is needed for MVP
}

// TaskStorage defines the interface for task persistence operations
type TaskStorage interface {
	// CreateTask creates a new task
	CreateTask(ctx context.Context, task *internal.Task) error

	// GetTask retrieves a task by ID
	GetTask(ctx context.Context, id string) (*internal.Task, error)

	// UpdateTask updates an existing task
	UpdateTask(ctx context.Context, task *internal.Task) error

	// DeleteTask deletes a task by ID
	DeleteTask(ctx context.Context, id string) error

	// GetPendingTasks retrieves all pending tasks that are due to run
	GetPendingTasks(ctx context.Context) ([]*internal.Task, error)

	// RunTransaction runs the given function within a transaction
	RunTransaction(ctx context.Context, fn func(context.Context, *firestore.Transaction) error) error
}

// FirestoreTaskStorage implements TaskStorage using Firestore
type FirestoreTaskStorage struct {
	client *internal.FirestoreClient
}

// NewFirestoreTaskStorage creates a new FirestoreTaskStorage instance
func NewFirestoreTaskStorage(client *internal.FirestoreClient) *FirestoreTaskStorage {
	return &FirestoreTaskStorage{
		client: client,
	}
}

func (s *FirestoreTaskStorage) tasksCollection() string {
	return "tasks"
}

func (s *FirestoreTaskStorage) CreateTask(ctx context.Context, task *internal.Task) error {
	_, err := s.client.Collection(s.tasksCollection()).Doc(task.ID).Set(ctx, task)
	return err
}

func (s *FirestoreTaskStorage) GetTask(ctx context.Context, id string) (*internal.Task, error) {
	doc, err := s.client.Collection(s.tasksCollection()).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}

	var task internal.Task
	if err := doc.DataTo(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

func (s *FirestoreTaskStorage) UpdateTask(ctx context.Context, task *internal.Task) error {
	task.UpdatedAt = time.Now()
	_, err := s.client.Collection(s.tasksCollection()).Doc(task.ID).Set(ctx, task)
	return err
}

func (s *FirestoreTaskStorage) DeleteTask(ctx context.Context, id string) error {
	_, err := s.client.Collection(s.tasksCollection()).Doc(id).Delete(ctx)
	return err
}

func (s *FirestoreTaskStorage) GetPendingTasks(ctx context.Context) ([]*internal.Task, error) {
	query := s.client.Collection(s.tasksCollection()).
		Where("Status", "in", []string{internal.TaskStatusPending, internal.TaskStatusRetrying}).
		Where("RunAt", "<=", time.Now()).
		OrderBy("RunAt", firestore.Asc)

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	var tasks []*internal.Task
	for _, doc := range docs {
		var task internal.Task
		if err := doc.DataTo(&task); err != nil {
			return nil, err
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

func (s *FirestoreTaskStorage) RunTransaction(ctx context.Context, fn func(context.Context, *firestore.Transaction) error) error {
	return s.client.RunTransaction(ctx, fn)
}

// GetTasksWithFilter retrieves tasks with optional status filtering
func (s *FirestoreTaskStorage) GetTasksWithFilter(ctx context.Context, filter *TaskFilter) ([]*internal.Task, error) {
	query := s.client.Collection(s.tasksCollection()).Query

	// Add status filter if specified
	if filter.Status != "" {
		query = query.Where("Status", "==", filter.Status)
	}

	// Execute query
	iter := query.Documents(ctx)
	defer iter.Stop()

	var tasks []*internal.Task
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate tasks: %w", err)
		}

		task := &internal.Task{}
		if err := doc.DataTo(task); err != nil {
			return nil, fmt.Errorf("failed to convert document to task: %w", err)
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetTasksCount returns the total count of tasks matching the given status filter
func (s *FirestoreTaskStorage) GetTasksCount(ctx context.Context, statuses []string) (int, error) {
	var query firestore.Query = s.client.Collection(s.tasksCollection()).Query

	// Add status filter if provided
	if len(statuses) > 0 {
		query = query.Where("Status", "in", statuses)
	}

	// Use the more efficient aggregation query to get the count
	aggregationQuery := query.NewAggregationQuery().WithCount("count")
	results, err := aggregationQuery.Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get tasks count: %w", err)
	}

	// Extract the count from the results using the "count" alias
	countResult, ok := results["count"]
	if !ok {
		return 0, fmt.Errorf("count not found in aggregation results")
	}

	// Convert the result to the correct type and extract the integer value
	countValue := countResult.(*firestorepb.Value)
	return int(countValue.GetIntegerValue()), nil
}
