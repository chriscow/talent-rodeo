package internal

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../.env"); err != nil {
		log.Println("No .env file found, using system env")
	}
}

func TestUserJobs(t *testing.T) {
	fs, err := NewFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	defer func() {
		DeleteTestData(t, fs, "test-user")
		if err := fs.Close(); err != nil {
			t.Fatalf("Failed to close firestore client: %v", err)
		}
	}()

	ctx := context.Background()

	t.Run("save and retrieve user job", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Create a test job
		jobID := "test-job-123"
		searchHash := "test-search-hash"
		userJob := &UserJob{
			ID:         jobID,
			SearchHash: searchHash,
			JobRef:     fs.Collection("job_details").Doc(jobID),
			Status:     "new",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Save the job
		err := SaveUserJob(ctx, fs, user.ID, userJob)
		if err != nil {
			t.Fatalf("SaveUserJob() error = %v", err)
		}

		// Retrieve jobs
		jobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		// Verify we got one job back
		if len(jobs) != 1 {
			t.Fatalf("GetUserJobs() returned %d jobs, want 1", len(jobs))
		}

		// Verify job details
		job := jobs[0]
		if job.ID != jobID {
			t.Errorf("job.ID = %v, want %v", job.ID, jobID)
		}
		if job.SearchHash != searchHash {
			t.Errorf("job.SearchHash = %v, want %v", job.SearchHash, searchHash)
		}
		if job.Status != "new" {
			t.Errorf("job.Status = %v, want new", job.Status)
		}
	})

	t.Run("update job status", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Create a test job
		jobID := "test-job-456"
		userJob := &UserJob{
			ID:         jobID,
			SearchHash: "test-search-hash",
			JobRef:     fs.Collection("job_details").Doc(jobID),
			Status:     "new",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Save the job
		err := SaveUserJob(ctx, fs, user.ID, userJob)
		if err != nil {
			t.Fatalf("SaveUserJob() error = %v", err)
		}

		// Update status to viewed
		err = UpdateUserJobStatus(ctx, fs, user.ID, jobID, "viewed")
		if err != nil {
			t.Fatalf("UpdateUserJobStatus() error = %v", err)
		}

		// Retrieve jobs
		jobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		// Find our job
		var updatedJob *UserJob
		for _, j := range jobs {
			if j.ID == jobID {
				updatedJob = j
				break
			}
		}

		if updatedJob == nil {
			t.Fatal("Job not found after update")
		}

		// Verify status and timestamp
		if updatedJob.Status != "viewed" {
			t.Errorf("job.Status = %v, want viewed", updatedJob.Status)
		}
		if updatedJob.ViewedAt == nil {
			t.Error("ViewedAt timestamp not set")
		}
	})

	t.Run("update job notes", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Create a test job
		jobID := "test-job-789"
		userJob := &UserJob{
			ID:         jobID,
			SearchHash: "test-search-hash",
			JobRef:     fs.Collection("job_details").Doc(jobID),
			Status:     "new",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Save the job
		err := SaveUserJob(ctx, fs, user.ID, userJob)
		if err != nil {
			t.Fatalf("SaveUserJob() error = %v", err)
		}

		// Update notes
		testNotes := "Interesting position, follow up next week"
		err = UpdateUserJobNotes(ctx, fs, user.ID, jobID, testNotes)
		if err != nil {
			t.Fatalf("UpdateUserJobNotes() error = %v", err)
		}

		// Retrieve jobs
		jobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		// Find our job
		var updatedJob *UserJob
		for _, j := range jobs {
			if j.ID == jobID {
				updatedJob = j
				break
			}
		}

		if updatedJob == nil {
			t.Fatal("Job not found after update")
		}

		// Verify notes
		if updatedJob.Notes != testNotes {
			t.Errorf("job.Notes = %v, want %v", updatedJob.Notes, testNotes)
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Create a test job
		jobID := "test-job-dedupe"
		userJob := &UserJob{
			ID:         jobID,
			SearchHash: "test-search-hash-1",
			JobRef:     fs.Collection("job_details").Doc(jobID),
			Status:     "new",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Save the job first time
		err := SaveUserJob(ctx, fs, user.ID, userJob)
		if err != nil {
			t.Fatalf("First SaveUserJob() error = %v", err)
		}

		// Try to save the same job with a different search hash
		userJob.SearchHash = "test-search-hash-2"
		err = SaveUserJob(ctx, fs, user.ID, userJob)
		if err != nil {
			t.Fatalf("Second SaveUserJob() error = %v", err)
		}

		// Retrieve jobs
		jobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		// Should only have one job
		if len(jobs) != 1 {
			t.Errorf("Got %d jobs, want 1 (deduplication failed)", len(jobs))
		}

		// The job should have the latest search hash
		if jobs[0].SearchHash != "test-search-hash-2" {
			t.Errorf("job.SearchHash = %v, want test-search-hash-2", jobs[0].SearchHash)
		}
	})

	t.Run("error cases", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Test missing user ID
		err := SaveUserJob(ctx, fs, "", &UserJob{ID: "test"})
		if err == nil {
			t.Error("SaveUserJob() with empty userID should return error")
		}

		// Test missing job ID
		err = SaveUserJob(ctx, fs, user.ID, &UserJob{})
		if err == nil {
			t.Error("SaveUserJob() with empty job ID should return error")
		}

		// Test invalid user ID for status update
		err = UpdateUserJobStatus(ctx, fs, "invalid-user", "test", "viewed")
		if err == nil {
			t.Error("UpdateUserJobStatus() with invalid user ID should return error")
		}

		// Test invalid job ID for status update
		err = UpdateUserJobStatus(ctx, fs, user.ID, "invalid-job", "viewed")
		if err == nil {
			t.Error("UpdateUserJobStatus() with invalid job ID should return error")
		}
	})
}

func TestSaveAndGetUserJobs(t *testing.T) {
	// Skip in CI environments
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Create a test user with a unique ID
	testUserID := "test-user-" + time.Now().Format("20060102150405")

	// Create test job
	testJobID := "test-job-" + time.Now().Format("20060102150405")
	searchHash := "test-search-hash"

	// Create user job
	userJob := &UserJob{
		ID:         testJobID,
		SearchHash: searchHash,
		Status:     "new",
		JobRef:     fs.Collection("job_details").Doc(testJobID),
	}

	// Save user job
	err = SaveUserJob(ctx, fs, testUserID, userJob)
	if err != nil {
		t.Fatalf("Failed to save user job: %v", err)
	}

	// Retrieve user jobs
	jobs, err := GetUserJobs(ctx, fs, testUserID)
	if err != nil {
		t.Fatalf("Failed to get user jobs: %v", err)
	}

	// Verify we got the job back
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	retrievedJob := jobs[0]
	if retrievedJob.ID != testJobID {
		t.Errorf("Expected job ID %s, got %s", testJobID, retrievedJob.ID)
	}
	if retrievedJob.Status != "new" {
		t.Errorf("Expected status 'new', got %s", retrievedJob.Status)
	}
	if retrievedJob.JobRef == nil {
		t.Error("Expected JobRef to be set, got nil")
	}
	if retrievedJob.SearchHash != searchHash {
		t.Errorf("Expected search hash %s, got %s", searchHash, retrievedJob.SearchHash)
	}

	// Clean up test data
	_, err = fs.Collection(usersCollection).Doc(testUserID).Collection(userJobsCollection).Doc(testJobID).Delete(ctx)
	if err != nil {
		t.Logf("Failed to clean up test data: %v", err)
	}
}
