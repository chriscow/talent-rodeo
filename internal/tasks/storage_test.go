/*
Package tasks_test implements integration tests for the Firestore task storage implementation.

Test Setup Requirements:
1. Environment Variables (in .env file):
   - FIRESTORE_FIREBASE_PROJECT_ID: Your Firebase project ID
   - FIREBASE_CREDENTIALS_FILE: Path to service account credentials JSON

2. Required Firestore Indexes:
   The tests use queries that require composite indexes. Create them using Firebase CLI:

   # Create index for task listing (Status + CreatedAt desc)
   firebase firestore:indexes --project $FIREBASE_PROJECT_ID \
     '{"collectionGroup": "tasks",
       "queryScope": "COLLECTION",
       "fields": [
         {"fieldPath": "Status", "order": "ASCENDING"},
         {"fieldPath": "CreatedAt", "order": "DESCENDING"}
       ]}'

   # Create index for pending tasks (Status + RunAt asc)
   firebase firestore:indexes --project $FIREBASE_PROJECT_ID \
     '{"collectionGroup": "tasks",
       "queryScope": "COLLECTION",
       "fields": [
         {"fieldPath": "Status", "order": "ASCENDING"},
         {"fieldPath": "RunAt", "order": "ASCENDING"}
       ]}'

Test Structure:
- Uses table-driven tests for comprehensive coverage
- Implements cursor-based pagination for efficient querying
- Follows Firestore best practices for querying and indexing
- Includes cleanup to remove test data after each test

Note: These tests require an actual Firestore instance and will modify data.
Make sure to use a development/test environment, not production.
*/

package tasks

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Fatalf("Failed to load .env file: %v", err)
	}
}

func setupTestStorage(t *testing.T) *FirestoreTaskStorage {
	t.Helper()

	ctx := context.Background()
	client, err := internal.NewFirestoreClient(ctx, os.Getenv("FIRESTORE_FIREBASE_PROJECT_ID"), os.Getenv("FIREBASE_CREDENTIALS_FILE"))
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	})

	return NewFirestoreTaskStorage(client)
}

func cleanupTestTasks(t *testing.T, storage *FirestoreTaskStorage) {
	ctx := context.Background()

	// Get all tasks in the collection
	docs, err := storage.client.Collection(storage.tasksCollection()).Documents(ctx).GetAll()
	if err != nil {
		t.Logf("Failed to get tasks for cleanup: %v", err)
		return
	}

	// Delete each task
	for _, doc := range docs {
		_, err := doc.Ref.Delete(ctx)
		if err != nil {
			t.Logf("Failed to delete task %s: %v", doc.Ref.ID, err)
		}
	}
}

func TestFirestoreTaskStorage_CreateAndGetTask(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create a test task
	now := time.Now()
	task := &internal.Task{
		ID:          now.UnixNano(),
		Type:        "test_task",
		Payload:     []byte(`{"test": "data"}`),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Test CreateTask
	if err := storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Test GetTask
	got, err := storage.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if got.ID != task.ID {
		t.Errorf("task ID = %d; want %d", got.ID, task.ID)
	}
	if got.Type != task.Type {
		t.Errorf("task type = %q; want %q", got.Type, task.Type)
	}
	if string(got.Payload) != string(task.Payload) {
		t.Errorf("task payload = %s; want %s", got.Payload, task.Payload)
	}
	if got.Status != task.Status {
		t.Errorf("task status = %v; want %v", got.Status, task.Status)
	}
}

func TestFirestoreTaskStorage_UpdateTask(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create a test task
	now := time.Now()
	task := &internal.Task{
		ID:          now.UnixNano(),
		Type:        "test_task",
		Payload:     []byte(`{"test": "data"}`),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Update task
	task.Status = internal.TaskStatusRunning
	task.Attempts = 1
	task.Error = "test error"

	if err := storage.UpdateTask(ctx, task); err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Verify update
	got, err := storage.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get updated task: %v", err)
	}

	if got.Status != internal.TaskStatusRunning {
		t.Errorf("task status = %v; want %v", got.Status, internal.TaskStatusRunning)
	}
	if got.Attempts != 1 {
		t.Errorf("task attempts = %d; want %d", got.Attempts, 1)
	}
	if got.Error != "test error" {
		t.Errorf("task error = %q; want %q", got.Error, "test error")
	}
}

func TestFirestoreTaskStorage_DeleteTask(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create a test task
	now := time.Now()
	task := &internal.Task{
		ID:          now.UnixNano(),
		Type:        "test_task",
		Payload:     []byte(`{"test": "data"}`),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Delete task
	if err := storage.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify deletion
	got, err := storage.GetTask(ctx, task.ID)
	if err == nil {
		t.Error("expected error getting deleted task, got nil")
	}
	if got != nil {
		t.Errorf("got task = %v; want nil", got)
	}
}

func TestFirestoreTaskStorage_GetPendingTasks(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create test tasks with different states
	baseTime := time.Now()
	tasks := []*internal.Task{
		{
			ID:          baseTime.UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data1"}`),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(1 * time.Second).UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data2"}`),
			Status:      internal.TaskStatusRunning,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(2 * time.Second).UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data3"}`),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
	}

	// Create all tasks
	for _, task := range tasks {
		if err := storage.CreateTask(ctx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// Get pending tasks
	got, err := storage.GetPendingTasks(ctx)
	if err != nil {
		t.Fatalf("failed to get pending tasks: %v", err)
	}

	// Should get two pending tasks
	if len(got) != 2 {
		t.Errorf("got %d pending tasks; want 2", len(got))
	}

	// Verify we got the right tasks by status
	for _, task := range got {
		if task.Status != internal.TaskStatusPending {
			t.Errorf("got task with status %s; want %s", task.Status, internal.TaskStatusPending)
		}
	}
}

func TestFirestoreTaskStorage_RunTransaction(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create a test task
	now := time.Now()
	task := &internal.Task{
		ID:          now.UnixNano(),
		Type:        "test_task",
		Payload:     []byte(`{"test": "data"}`),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Run a transaction that updates the task
	err := storage.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Get task in transaction
		doc := storage.client.Collection(storage.tasksCollection()).Doc(storage.taskID(task.ID))
		docsnap, err := tx.Get(doc)
		if err != nil {
			return err
		}

		var t internal.Task
		if err := docsnap.DataTo(&t); err != nil {
			return err
		}

		// Update task in transaction
		t.Status = internal.TaskStatusRunning
		t.Attempts = 1
		return tx.Set(doc, t)
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify transaction changes
	got, err := storage.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task after transaction: %v", err)
	}

	if got.Status != internal.TaskStatusRunning {
		t.Errorf("task status = %v; want %v", got.Status, internal.TaskStatusRunning)
	}
	if got.Attempts != 1 {
		t.Errorf("task attempts = %d; want %d", got.Attempts, 1)
	}
}

func TestFirestoreTaskStorage_GetTasksWithFilter(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create test tasks with different states
	baseTime := time.Now()
	tasks := []*internal.Task{
		{
			ID:          baseTime.UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data1"}`),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(1 * time.Second).UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data2"}`),
			Status:      internal.TaskStatusRunning,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(2 * time.Second).UnixNano(),
			Type:        "test_task",
			Payload:     []byte(`{"test": "data3"}`),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
	}

	// Create all tasks
	for _, task := range tasks {
		if err := storage.CreateTask(ctx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	tests := []struct {
		name   string
		filter *TaskFilter
		want   int // expected number of tasks
	}{
		{
			name:   "no filter, get all",
			filter: &TaskFilter{},
			want:   3,
		},
		{
			name:   "filter by pending status",
			filter: &TaskFilter{Status: internal.TaskStatusPending},
			want:   2,
		},
		{
			name:   "filter by running status",
			filter: &TaskFilter{Status: internal.TaskStatusRunning},
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := storage.GetTasksWithFilter(ctx, tt.filter)
			if err != nil {
				t.Fatalf("GetTasksWithFilter() error = %v", err)
			}

			if len(got) != tt.want {
				t.Errorf("GetTasksWithFilter() got %d tasks, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFirestoreTaskStorage_GetTasksCount(t *testing.T) {
	storage := setupTestStorage(t)
	defer cleanupTestTasks(t, storage)
	ctx := context.Background()

	// Create test tasks with different states
	baseTime := time.Now()
	tasks := []*internal.Task{
		{
			ID:          baseTime.UnixNano(),
			Type:        "test_task",
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(1 * time.Second).UnixNano(), // Ensure different ID
			Type:        "test_task",
			Status:      internal.TaskStatusRunning,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime.Add(-12 * time.Hour), // 12 hours ago
			UpdatedAt:   baseTime,
		},
		{
			ID:          baseTime.Add(2 * time.Second).UnixNano(), // Ensure different ID
			Type:        "test_task",
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       baseTime,
			CreatedAt:   baseTime, // Current time
			UpdatedAt:   baseTime,
		},
	}

	// Create all tasks
	for _, task := range tasks {
		if err := storage.CreateTask(ctx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// Inspect raw Firestore data
	docs, err := storage.client.Collection(storage.tasksCollection()).Documents(ctx).GetAll()
	if err != nil {
		t.Logf("Failed to inspect Firestore data: %v", err)
	} else {
		t.Logf("\nRaw Firestore content: %d documents", len(docs))
		for _, doc := range docs {
			var data map[string]interface{}
			doc.DataTo(&data)
			t.Logf("Document ID: %s\nData: %+v\n", doc.Ref.ID, data)
		}
	}

	// Debug log: print the task details
	t.Log("\nTest tasks created:")
	for i, task := range tasks {
		t.Logf("Task[%d]: ID=%d, CreatedAt=%v (%d ns), Status=%s",
			i, task.ID, task.CreatedAt, task.CreatedAt.UnixNano(), task.Status)
	}

	tests := []struct {
		name     string
		statuses []string
		want     int
	}{
		{
			name:     "no filter, count all",
			statuses: nil,
			want:     3,
		},
		{
			name:     "filter by pending status",
			statuses: []string{internal.TaskStatusPending},
			want:     2,
		},
		{
			name:     "filter by running status",
			statuses: []string{internal.TaskStatusRunning},
			want:     1,
		},
		{
			name:     "filter by non-existent status",
			statuses: []string{internal.TaskStatusFailed},
			want:     0,
		},
		{
			name:     "filter by multiple statuses",
			statuses: []string{internal.TaskStatusPending, internal.TaskStatusRunning},
			want:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := storage.GetTasksCount(ctx, tt.statuses)
			if err != nil {
				t.Fatalf("GetTasksCount() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("GetTasksCount() = %v, want %v", got, tt.want)
			}
		})
	}
}
