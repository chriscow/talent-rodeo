package search

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

// MockPusher implements the PusherInterface for testing
type MockPusher struct {
	Events []struct {
		Channel string
		Event   string
		Data    interface{}
	}
}

func (p *MockPusher) Trigger(channel string, eventName string, data interface{}) error {
	p.Events = append(p.Events, struct {
		Channel string
		Event   string
		Data    interface{}
	}{
		Channel: channel,
		Event:   eventName,
		Data:    data,
	})
	return nil
}

func newTestFirestore(t *testing.T) *internal.FirestoreClient {
	// Use the emulator for testing
	os.Setenv("FIRESTORE_EMULATOR_HOST", "localhost:8080")

	// Initialize Firebase app
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil, option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create Firebase app: %v", err)
		return nil
	}

	// Get Firestore client
	client, err := app.Firestore(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
		return nil
	}

	t.Cleanup(func() {
		client.Close()
	})

	return &internal.FirestoreClient{Client: client}
}

func TestServiceProgressTracking(t *testing.T) {
	// Skip if not running in CI or with emulator
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("Skipping test: FIRESTORE_EMULATOR_HOST not set")
	}

	// Create test dependencies
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockPusher := &MockPusher{}
	fs := newTestFirestore(t)

	// Create the service with the progress cache
	service := NewService(fs, logger, nil, nil, "test@example.com", mockPusher)
	if service.progressCache == nil {
		t.Fatal("Progress cache should be initialized")
	}

	// Setup test user ID
	userID := "test-user-progress"

	// Test getOrCreateProgress creates a new entry
	progress := service.getOrCreateProgress(userID)
	if progress == nil {
		t.Fatal("Progress should not be nil")
	}
	if progress.UserID != userID {
		t.Errorf("Expected userID %s, got %s", userID, progress.UserID)
	}
	if progress.TotalJobsFound != 0 {
		t.Errorf("Expected TotalJobsFound 0, got %d", progress.TotalJobsFound)
	}

	// Test updateProgress updates the progress
	service.updateProgress(userID, func(p *SearchProgress) {
		p.TotalJobsFound = 5
		p.JobsAnalyzed = 0
		p.AnalysisInProgress = true
	})

	// Verify progress was updated
	progress = service.getOrCreateProgress(userID)
	if progress.TotalJobsFound != 5 {
		t.Errorf("Expected TotalJobsFound 5, got %d", progress.TotalJobsFound)
	}
	if progress.JobsAnalyzed != 0 {
		t.Errorf("Expected JobsAnalyzed 0, got %d", progress.JobsAnalyzed)
	}
	if !progress.AnalysisInProgress {
		t.Error("Expected AnalysisInProgress to be true")
	}

	// Test recordSearchCompleted marks the search as complete
	service.recordSearchCompleted(userID, 5)

	progress = service.getOrCreateProgress(userID)
	if !progress.SearchCompleted {
		t.Error("Expected SearchCompleted to be true")
	}
	if progress.TotalJobsFound != 5 {
		t.Errorf("Expected TotalJobsFound 5, got %d", progress.TotalJobsFound)
	}

	// Test recordJobAnalyzed increments analyzed jobs
	for i := 0; i < 5; i++ {
		service.recordJobAnalyzed(userID, i%2 == 0) // Alternate between match and not match
	}

	// Verify all jobs analyzed
	progress = service.getOrCreateProgress(userID)
	if progress.JobsAnalyzed != 5 {
		t.Errorf("Expected JobsAnalyzed 5, got %d", progress.JobsAnalyzed)
	}
	if progress.JobsMatched != 3 {
		t.Errorf("Expected JobsMatched 3, got %d", progress.JobsMatched)
	}
	if progress.JobsNotMatched != 2 {
		t.Errorf("Expected JobsNotMatched 2, got %d", progress.JobsNotMatched)
	}

	// Wait a short time for any cleanup goroutines to complete
	time.Sleep(6 * time.Second)

	// Progress should be cleaned up now
	_, found := service.progressCache.Get(userID)
	if found {
		t.Error("Progress should have been cleaned up")
	}
}

func TestServiceNoAutoCleanupWithPendingJobs(t *testing.T) {
	// Skip if not running in CI or with emulator
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("Skipping test: FIRESTORE_EMULATOR_HOST not set")
	}

	// Create test dependencies
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockPusher := &MockPusher{}
	fs := newTestFirestore(t)

	ctx := context.Background()

	// Create the service with the progress cache
	service := NewService(fs, logger, nil, nil, "test@example.com", mockPusher)
	if service.progressCache == nil {
		t.Fatal("Progress cache should be initialized")
	}

	// Setup test user ID
	userID := "test-user-with-pending"

	// Create a test job that's unprocessed
	job := &internal.UserJob{
		ID:     "test-job-1",
		Status: internal.JobStatusUnprocessed,
	}
	err := internal.CreateUserJob(ctx, fs, userID, job)
	if err != nil {
		t.Fatalf("Failed to save user job: %v", err)
	}

	// Initialize and update progress
	service.updateProgress(userID, func(p *SearchProgress) {
		p.TotalJobsFound = 1
		p.JobsAnalyzed = 0
		p.AnalysisInProgress = true
	})

	// Mark search completed
	service.recordSearchCompleted(userID, 1)

	// Wait for cleanup attempt
	time.Sleep(6 * time.Second)

	// Progress should still exist because there's a pending job
	_, found := service.progressCache.Get(userID)
	if !found {
		t.Error("Progress should not be cleaned up when there are pending jobs")
	}

	// Update the job to be processed
	job.Status = internal.JobStatusNew
	err = internal.CreateUserJob(ctx, fs, userID, job)
	if err != nil {
		t.Fatalf("Failed to update user job: %v", err)
	}

	// Record that the job was analyzed
	service.recordJobAnalyzed(userID, true)

	// Wait for cleanup attempt
	time.Sleep(6 * time.Second)

	// Progress should be cleaned up now
	_, found = service.progressCache.Get(userID)
	if found {
		t.Error("Progress should be cleaned up after all jobs are processed")
	}
}
