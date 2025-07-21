package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestGetUserJobsQualityCount(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewTestFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-quality-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-quality-")
		client.Close()
	})

	// Create test user
	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("quality-test-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Quality Test User", uniqueEmail, "+14251234567", false)

	t.Run("NoJobsReturnsZero", func(t *testing.T) {
		count, err := GetUserJobsQualityCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsQualityCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUserJobsQualityCount() got = %v, want 0", count)
		}
	})

	// Create test jobs with different ratings
	jobs := []struct {
		jobID  string
		rating int
		status JobStatus
	}{
		{"job-1", 5, JobStatusNew},      // Should count (5 stars)
		{"job-2", 4, JobStatusNew},      // Should count (4 stars)
		{"job-3", 3, JobStatusNew},      // Should NOT count (3 stars)
		{"job-4", 2, JobStatusNew},      // Should NOT count (2 stars)
		{"job-5", 1, JobStatusNew},      // Should NOT count (1 star)
		{"job-6", 5, JobStatusSaved},    // Should count (5 stars, status irrelevant)
		{"job-7", 4, JobStatusArchived}, // Should count (4 stars, status irrelevant)
		{"job-8", 0, JobStatusNew},      // Should NOT count (0 stars)
	}

	// Create the jobs and set their ratings
	for _, job := range jobs {
		// Create the UserJob
		_, err := CreateUserJob(ctx, client, user.ID, job.jobID)
		if err != nil {
			t.Fatalf("Failed to create test job %s: %v", job.jobID, err)
		}

		// Update the job with the desired rating and status
		err = UpdateUserJobStatus(ctx, client, user.ID, job.jobID, job.status)
		if err != nil {
			t.Fatalf("Failed to update job %s status: %v", job.jobID, err)
		}

		// Update rating using transaction (following the pattern from existing code)
		err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			return UpdateUserJobRatingTx(ctx, client, tx, user.ID, job.jobID, job.rating)
		})
		if err != nil {
			t.Fatalf("Failed to update job %s rating: %v", job.jobID, err)
		}
	}

	t.Run("CountsQualityJobsCorrectly", func(t *testing.T) {
		count, err := GetUserJobsQualityCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsQualityCount() error = %v", err)
		}
		// Expected: job-1(5), job-2(4), job-6(5), job-7(4) = 4 total
		expectedCount := int64(4)
		if count != expectedCount {
			t.Errorf("GetUserJobsQualityCount() got = %v, want %v", count, expectedCount)
		}
	})

	t.Run("InvalidUserIDReturnsError", func(t *testing.T) {
		_, err := GetUserJobsQualityCount(ctx, client, "")
		if err == nil {
			t.Error("GetUserJobsQualityCount() with empty userID should return error")
		}
	})

	t.Run("NilFirestoreClientReturnsError", func(t *testing.T) {
		_, err := GetUserJobsQualityCount(ctx, nil, user.ID)
		if err == nil {
			t.Error("GetUserJobsQualityCount() with nil client should return error")
		}
	})
}

func TestGetUserJobsRejectedCount(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewTestFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-rejected-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-rejected-")
		client.Close()
	})

	// Create test user
	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("rejected-test-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Rejected Test User", uniqueEmail, "+14251234568", false)

	t.Run("NoJobsReturnsZero", func(t *testing.T) {
		count, err := GetUserJobsRejectedCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsRejectedCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUserJobsRejectedCount() got = %v, want 0", count)
		}
	})

	// Create test jobs with different ratings and statuses to test various scenarios
	jobs := []struct {
		jobID          string
		rating         int
		status         JobStatus
		expectRejected bool
		description    string
	}{
		{"job-1", 5, JobStatusNew, false, "5-star new job"},
		{"job-2", 4, JobStatusNew, false, "4-star new job"},
		{"job-3", 3, JobStatusNew, true, "3-star new job (low rating)"},
		{"job-4", 2, JobStatusNew, true, "2-star new job (low rating)"},
		{"job-5", 1, JobStatusNew, true, "1-star new job (low rating)"},
		{"job-6", 0, JobStatusNew, true, "0-star new job (low rating)"},
		{"job-7", 5, JobStatusArchived, true, "5-star archived job (archived status)"},
		{"job-8", 4, JobStatusArchived, true, "4-star archived job (archived status)"},
		{"job-9", 3, JobStatusArchived, true, "3-star archived job (both low rating AND archived - should count once)"},
		{"job-10", 5, JobStatusDismissed, true, "5-star dismissed job (dismissed status)"},
		{"job-11", 2, JobStatusDismissed, true, "2-star dismissed job (both low rating AND dismissed - should count once)"},
		{"job-12", 4, JobStatusSaved, false, "4-star saved job (neither low rating nor archived/dismissed)"},
		{"job-13", 5, JobStatusApplied, false, "5-star applied job (neither low rating nor archived/dismissed)"},
	}

	// Create the jobs and set their ratings/statuses
	for _, job := range jobs {
		// Create the UserJob
		_, err := CreateUserJob(ctx, client, user.ID, job.jobID)
		if err != nil {
			t.Fatalf("Failed to create test job %s: %v", job.jobID, err)
		}

		// Update the job with the desired status
		err = UpdateUserJobStatus(ctx, client, user.ID, job.jobID, job.status)
		if err != nil {
			t.Fatalf("Failed to update job %s status: %v", job.jobID, err)
		}

		// Update rating using transaction
		err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			return UpdateUserJobRatingTx(ctx, client, tx, user.ID, job.jobID, job.rating)
		})
		if err != nil {
			t.Fatalf("Failed to update job %s rating: %v", job.jobID, err)
		}
	}

	t.Run("CountsRejectedJobsCorrectly", func(t *testing.T) {
		count, err := GetUserJobsRejectedCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsRejectedCount() error = %v", err)
		}

		// Count expected rejected jobs
		var expectedCount int64
		for _, job := range jobs {
			if job.expectRejected {
				expectedCount++
			}
		}

		if count != expectedCount {
			t.Errorf("GetUserJobsRejectedCount() got = %v, want %v", count, expectedCount)

			// Debug: print what should be counted
			t.Logf("Expected rejected jobs:")
			for _, job := range jobs {
				if job.expectRejected {
					t.Logf("  - %s: %s", job.jobID, job.description)
				}
			}
		}
	})

	t.Run("InvalidUserIDReturnsError", func(t *testing.T) {
		_, err := GetUserJobsRejectedCount(ctx, client, "")
		if err == nil {
			t.Error("GetUserJobsRejectedCount() with empty userID should return error")
		}
	})

	t.Run("NilFirestoreClientReturnsError", func(t *testing.T) {
		_, err := GetUserJobsRejectedCount(ctx, nil, user.ID)
		if err == nil {
			t.Error("GetUserJobsRejectedCount() with nil client should return error")
		}
	})
}

// TestUserJobsCountingMath verifies that quality + rejected counts equal total user jobs
func TestUserJobsCountingMath(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewTestFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-math-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-math-")
		client.Close()
	})

	// Create test user
	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("math-test-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Math Test User", uniqueEmail, "+14251234569", false)

	// Create a comprehensive set of jobs
	jobs := []struct {
		jobID  string
		rating int
		status JobStatus
	}{
		{"math-job-1", 5, JobStatusNew},
		{"math-job-2", 4, JobStatusNew},
		{"math-job-3", 3, JobStatusNew},
		{"math-job-4", 2, JobStatusArchived},
		{"math-job-5", 1, JobStatusDismissed},
		{"math-job-6", 5, JobStatusArchived},
		{"math-job-7", 4, JobStatusSaved},
		{"math-job-8", 0, JobStatusNew},
	}

	// Create the jobs
	for _, job := range jobs {
		_, err := CreateUserJob(ctx, client, user.ID, job.jobID)
		if err != nil {
			t.Fatalf("Failed to create test job %s: %v", job.jobID, err)
		}

		err = UpdateUserJobStatus(ctx, client, user.ID, job.jobID, job.status)
		if err != nil {
			t.Fatalf("Failed to update job %s status: %v", job.jobID, err)
		}

		err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			return UpdateUserJobRatingTx(ctx, client, tx, user.ID, job.jobID, job.rating)
		})
		if err != nil {
			t.Fatalf("Failed to update job %s rating: %v", job.jobID, err)
		}
	}

	t.Run("QualityPlusRejectedEqualsTotal", func(t *testing.T) {
		qualityCount, err := GetUserJobsQualityCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsQualityCount() error = %v", err)
		}

		rejectedCount, err := GetUserJobsRejectedCount(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobsRejectedCount() error = %v", err)
		}

		// Get total count by listing all jobs
		allJobs, err := ListUserJobs(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("ListUserJobs() error = %v", err)
		}
		totalCount := int64(len(allJobs))

		t.Logf("Quality jobs: %d, Rejected jobs: %d, Total jobs: %d", qualityCount, rejectedCount, totalCount)

		if qualityCount+rejectedCount != totalCount {
			t.Errorf("Quality + Rejected should equal Total. Got: %d + %d = %d, want %d",
				qualityCount, rejectedCount, qualityCount+rejectedCount, totalCount)
		}
	})
}
