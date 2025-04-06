package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"time"
)

// CreateTestUserAndJob creates a test user and associates a job with them
// Returns userID, jobID, error
func CreateTestUserAndJob(ctx context.Context) (string, string, error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Generate unique IDs based on timestamp
	now := time.Now().Format("20060102150405")
	testUserID := fmt.Sprintf("test-user-%s", now)
	testJobID := fmt.Sprintf("test-job-%s", now)
	logger.Info("Creating test data", "user_id", testUserID, "job_id", testJobID)

	// Create test user
	userRef := fs.Collection("users").Doc(testUserID)
	_, err = userRef.Set(ctx, map[string]interface{}{
		"email":      fmt.Sprintf("test-%s@example.com", testUserID),
		"created_at": time.Now(),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create test user: %w", err)
	}

	// Create test job detail
	jobRef := fs.Collection("job_details").Doc(testJobID)
	_, err = jobRef.Set(ctx, map[string]interface{}{
		"id":              testJobID,
		"title":           "Software Engineer",
		"company":         "Test Company Inc.",
		"location":        "Remote, US",
		"provider":        "test-provider",
		"provider_job_id": testJobID,
		"posted_date":     time.Now().AddDate(0, 0, -3), // 3 days ago
		"created_at":      time.Now(),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create test job: %w", err)
	}

	// Create user job
	searchHash := "test-search-hash"
	userJob := &internal.UserJob{
		ID:         testJobID,
		SearchHash: searchHash,
		Status:     "new",
		JobRef:     jobRef,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = internal.SaveUserJob(ctx, fs, testUserID, userJob)
	if err != nil {
		return "", "", fmt.Errorf("failed to save user job: %w", err)
	}

	logger.Info("Successfully created test data",
		"user_id", testUserID,
		"job_id", testJobID,
		"job_ref", jobRef.Path)

	return testUserID, testJobID, nil
}
