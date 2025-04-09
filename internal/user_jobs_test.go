package internal

import (
	"context"
	"fmt"
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

func TestGetUserJobsSorted(t *testing.T) {
	fs, err := NewFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}

	ctx := context.Background()

	// Test 1: Basic filtering by status
	t.Run("get sorted user jobs by status with paging", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user-sorted", "sorted@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Create test jobs with different statuses
		statuses := []JobStatus{JobStatusNew, JobStatusViewed, JobStatusSaved, JobStatusApplied, JobStatusRejected}
		for i, status := range statuses {
			jobID := fmt.Sprintf("test-job-sorted-%d", i)
			job := CreateTestJob(t, fs, user.ID, jobID, status, time.Now().Add(time.Duration(i)*time.Minute))
			t.Logf("Created test job with ID: %s, Status: %s", job.ID, job.Status)
		}

		// Now let's verify the jobs were actually created with the right status
		allJobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}
		t.Logf("Total jobs created: %d", len(allJobs))

		// Count jobs by status
		statusCounts := make(map[JobStatus]int)
		for _, job := range allJobs {
			t.Logf("Job in DB: ID=%s, Status=%s", job.ID, job.Status)
			statusCounts[job.Status]++
		}

		for status, count := range statusCounts {
			t.Logf("Status count - %s: %d", status, count)
		}

		// Add a delay to allow Firestore indexing...
		t.Log("Waiting for Firestore indexing...")
		time.Sleep(2 * time.Second)

		// Retrieve jobs with a specific status and paging
		pageSize := 3
		pageToken := ""
		result, nextPageToken, err := GetUserJobsSorted(ctx, fs, user.ID, JobStatusNew, pageSize, pageToken)
		if err != nil {
			t.Fatalf("GetUserJobsSorted() error = %v", err)
		}

		// Log the retrieved jobs for debugging
		t.Logf("Retrieved jobs with status 'new': %d", len(result))
		for i, job := range result {
			t.Logf("Result job %d: ID=%s, Status=%s", i, job.ID, job.Status)
		}

		// Verify the jobs retrieved match the requested status
		if len(result) != 1 {
			t.Errorf("Expected 1 'new' job, got %d", len(result))
		} else if result[0].Status != JobStatusNew {
			t.Errorf("Expected job status to be 'new', got '%s'", result[0].Status)
		}

		// Verify next page token is empty since there is only one job with this status
		if nextPageToken != "" {
			t.Errorf("Expected empty nextPageToken, got %s", nextPageToken)
		}
	})

	// Test 2: Comprehensive pagination test with multiple pages - evenly divisible
	t.Run("pagination with evenly divisible pages", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user-pagination", "pagination@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Create 9 jobs with "new" status - this should give us 3 pages with pageSize=3
		const jobCount = 9
		const pageSize = 3

		var createdJobs []*UserJob
		for i := 0; i < jobCount; i++ {
			jobID := fmt.Sprintf("test-pagination-job-%d", i)
			// Create jobs with timestamps 1 minute apart to ensure consistent ordering
			timestamp := time.Now().Add(time.Duration(i) * time.Minute)
			job := CreateTestJob(t, fs, user.ID, jobID, JobStatusNew, timestamp)
			createdJobs = append(createdJobs, job)
			t.Logf("Created test job with ID: %s, Status: %s, Time: %s", job.ID, job.Status, job.CreatedAt)
		}

		// Verify all jobs were created
		allJobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		newJobsCount := 0
		for _, job := range allJobs {
			if job.Status == JobStatusNew {
				newJobsCount++
			}
		}

		if newJobsCount != jobCount {
			t.Fatalf("Expected %d 'new' jobs, got %d", jobCount, newJobsCount)
		}

		// Add a delay to allow Firestore indexing
		t.Log("Waiting for Firestore indexing...")
		time.Sleep(2 * time.Second)

		// Test pagination - we should have 3 pages with 3 jobs per page
		var allRetrievedJobs []*UserJob
		pageToken := ""
		pagesRetrieved := 0

		for {
			pagesRetrieved++

			// Get the next page of results
			jobs, nextPageToken, err := GetUserJobsSorted(ctx, fs, user.ID, JobStatusNew, pageSize, pageToken)
			if err != nil {
				t.Fatalf("GetUserJobsSorted() error on page %d: %v", pagesRetrieved, err)
			}

			t.Logf("Page %d: Retrieved %d jobs, nextPageToken=%s", pagesRetrieved, len(jobs), nextPageToken)

			// Add jobs from this page to our collection of all retrieved jobs
			allRetrievedJobs = append(allRetrievedJobs, jobs...)

			// Verify we got the expected number of jobs on this page
			expectedJobsOnPage := pageSize
			if pagesRetrieved == 3 { // Last page
				expectedJobsOnPage = jobCount - (pagesRetrieved-1)*pageSize // Should be 3
			}

			if len(jobs) != expectedJobsOnPage {
				t.Errorf("Page %d: Expected %d jobs, got %d", pagesRetrieved, expectedJobsOnPage, len(jobs))
			}

			// Verify job status
			for i, job := range jobs {
				if job.Status != JobStatusNew {
					t.Errorf("Page %d, Job %d: Expected status 'new', got '%s'", pagesRetrieved, i, job.Status)
				}
			}

			// Check if we should continue to the next page
			if nextPageToken == "" || pagesRetrieved >= 3 {
				// We should have a nextPageToken for pages 1 and 2, but not for page 3
				if pagesRetrieved < 3 && nextPageToken == "" {
					t.Errorf("Page %d: Expected non-empty nextPageToken, got empty", pagesRetrieved)
				}
				if pagesRetrieved == 3 && nextPageToken != "" {
					// There is a side effect that if the page size is evenly divisible by
					// the total number of jobs, we will be returning a value for `nextPageToke`
					// when in fact there are no more jobs to fetch.
					t.Logf("Page %d: nextPageToken, got '%s'", pagesRetrieved, nextPageToken)
				}

				break
			}

			// Move to the next page
			pageToken = nextPageToken
		}

		// Verify we got the expected number of pages
		if pagesRetrieved != 3 {
			t.Errorf("Expected 3 pages, got %d", pagesRetrieved)
		}

		// Verify we got all jobs
		if len(allRetrievedJobs) != jobCount {
			t.Errorf("Retrieved %d total jobs across all pages, expected %d", len(allRetrievedJobs), jobCount)
		}

		// Verify jobs are in the correct order (by created_at)
		for i := 1; i < len(allRetrievedJobs); i++ {
			if allRetrievedJobs[i].CreatedAt.Before(allRetrievedJobs[i-1].CreatedAt) {
				t.Errorf("Jobs not in correct order: job[%d].CreatedAt=%v should be after job[%d].CreatedAt=%v",
					i, allRetrievedJobs[i].CreatedAt, i-1, allRetrievedJobs[i-1].CreatedAt)
			}
		}
	})

	// Test 3: Pagination test when job count is not evenly divisible by page size
	t.Run("pagination with non-evenly divisible pages", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user-pagination-uneven", "pagination-uneven@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Create 9 jobs with "new" status - with page size 2, this should give us 5 pages (4 full, 1 partial)
		const jobCount = 9
		const pageSize = 2

		var createdJobs []*UserJob
		for i := 0; i < jobCount; i++ {
			jobID := fmt.Sprintf("test-pagination-uneven-job-%d", i)
			// Create jobs with timestamps 1 minute apart to ensure consistent ordering
			timestamp := time.Now().Add(time.Duration(i) * time.Minute)
			job := CreateTestJob(t, fs, user.ID, jobID, JobStatusNew, timestamp)
			createdJobs = append(createdJobs, job)
			t.Logf("Created test job with ID: %s, Status: %s, Time: %s", job.ID, job.Status, job.CreatedAt)
		}

		// Verify all jobs were created
		allJobs, err := GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetUserJobs() error = %v", err)
		}

		newJobsCount := 0
		for _, job := range allJobs {
			if job.Status == JobStatusNew {
				newJobsCount++
			}
		}

		if newJobsCount != jobCount {
			t.Fatalf("Expected %d 'new' jobs, got %d", jobCount, newJobsCount)
		}

		// Add a delay to allow Firestore indexing
		t.Log("Waiting for Firestore indexing...")
		time.Sleep(2 * time.Second)

		// Test pagination - we should have 5 pages: 4 with 2 jobs per page and 1 with 1 job
		var allRetrievedJobs []*UserJob
		pageToken := ""
		pagesRetrieved := 0

		for {
			pagesRetrieved++

			// Get the next page of results
			jobs, nextPageToken, err := GetUserJobsSorted(ctx, fs, user.ID, JobStatusNew, pageSize, pageToken)
			if err != nil {
				t.Fatalf("GetUserJobsSorted() error on page %d: %v", pagesRetrieved, err)
			}

			t.Logf("Page %d: Retrieved %d jobs, nextPageToken=%s", pagesRetrieved, len(jobs), nextPageToken)

			// Add jobs from this page to our collection of all retrieved jobs
			allRetrievedJobs = append(allRetrievedJobs, jobs...)

			// Verify we got the expected number of jobs on this page
			expectedJobsOnPage := pageSize
			if pagesRetrieved == 5 { // Last page
				expectedJobsOnPage = jobCount % pageSize // Should be 1
			}

			if len(jobs) != expectedJobsOnPage {
				t.Errorf("Page %d: Expected %d jobs, got %d", pagesRetrieved, expectedJobsOnPage, len(jobs))
			}

			// Verify job status
			for i, job := range jobs {
				if job.Status != JobStatusNew {
					t.Errorf("Page %d, Job %d: Expected status 'new', got '%s'", pagesRetrieved, i, job.Status)
				}
			}

			// Check if we should continue to the next page
			if nextPageToken == "" || pagesRetrieved >= 5 {
				// We should have a nextPageToken for pages 1-4, but not for page 5
				if pagesRetrieved < 5 && nextPageToken == "" {
					t.Errorf("Page %d: Expected non-empty nextPageToken, got empty", pagesRetrieved)
				}
				if pagesRetrieved == 5 && nextPageToken != "" {
					t.Errorf("Page %d: Expected empty nextPageToken, got '%s'", pagesRetrieved, nextPageToken)
				}

				break
			}

			// Move to the next page
			pageToken = nextPageToken
		}

		// Verify we got the expected number of pages
		if pagesRetrieved != 5 {
			t.Errorf("Expected 5 pages, got %d", pagesRetrieved)
		}

		// Verify we got all jobs
		if len(allRetrievedJobs) != jobCount {
			t.Errorf("Retrieved %d total jobs across all pages, expected %d", len(allRetrievedJobs), jobCount)
		}

		// Verify jobs are in the correct order (by created_at)
		for i := 1; i < len(allRetrievedJobs); i++ {
			if allRetrievedJobs[i].CreatedAt.Before(allRetrievedJobs[i-1].CreatedAt) {
				t.Errorf("Jobs not in correct order: job[%d].CreatedAt=%v should be after job[%d].CreatedAt=%v",
					i, allRetrievedJobs[i].CreatedAt, i-1, allRetrievedJobs[i-1].CreatedAt)
			}
		}
	})
}

func TestJobCounts(t *testing.T) {
	fs, err := NewFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	defer fs.Close()

	ctx := context.Background()

	t.Run("get job count for a specific status", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user-counts", "counts@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Create test jobs with different statuses
		statuses := []JobStatus{
			JobStatusNew, JobStatusNew, JobStatusNew, // 3 new jobs
			JobStatusViewed, JobStatusViewed, // 2 viewed jobs
			JobStatusSaved,                       // 1 saved job
			JobStatusApplied,                     // 1 applied job
			JobStatusRejected, JobStatusRejected, // 2 rejected jobs
		}

		for i, status := range statuses {
			jobID := fmt.Sprintf("test-count-job-%d", i)
			CreateTestJob(t, fs, user.ID, jobID, status, time.Now())
		}

		// Add a delay to allow Firestore to process the writes
		t.Log("Waiting for Firestore processing...")
		time.Sleep(2 * time.Second)

		// Test GetJobsCount for each status
		statusCountMap := map[JobStatus]int64{
			JobStatusNew:      3,
			JobStatusViewed:   2,
			JobStatusSaved:    1,
			JobStatusApplied:  1,
			JobStatusRejected: 2,
		}

		for status, expectedCount := range statusCountMap {
			count, err := GetJobsCount(ctx, fs, user.ID, status)
			if err != nil {
				t.Errorf("GetJobsCount() error = %v", err)
				continue
			}

			if count != expectedCount {
				t.Errorf("GetJobsCount() for status %s got count = %d, want %d",
					status, count, expectedCount)
			} else {
				t.Logf("Successfully counted %d jobs with status %s", count, status)
			}
		}
	})

	t.Run("get all job status counts", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user-all-counts", "all-counts@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Create test jobs with different statuses
		statusCounts := map[JobStatus]int{
			JobStatusNew:      4,
			JobStatusViewed:   3,
			JobStatusSaved:    2,
			JobStatusApplied:  1,
			JobStatusRejected: 2,
		}

		jobCounter := 0
		for status, count := range statusCounts {
			for i := 0; i < count; i++ {
				jobID := fmt.Sprintf("test-all-count-job-%d", jobCounter)
				CreateTestJob(t, fs, user.ID, jobID, status, time.Now())
				jobCounter++
			}
		}

		// Add a delay to allow Firestore to process the writes
		t.Log("Waiting for Firestore processing...")
		time.Sleep(2 * time.Second)

		// Test GetAllJobStatusCounts
		allCounts, err := GetAllJobStatusCounts(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetAllJobStatusCounts() error = %v", err)
		}

		// Verify each status count
		for status, expectedCount := range statusCounts {
			statusStr := string(status)
			actualCount, exists := allCounts[statusStr]

			if !exists {
				t.Errorf("GetAllJobStatusCounts() missing count for status %s", status)
				continue
			}

			if actualCount != int64(expectedCount) {
				t.Errorf("GetAllJobStatusCounts() for status %s got count = %d, want %d",
					status, actualCount, expectedCount)
			} else {
				t.Logf("Successfully counted %d jobs with status %s", actualCount, status)
			}
		}

		// Check total count across all statuses
		expectedTotal := int64(0)
		for _, count := range statusCounts {
			expectedTotal += int64(count)
		}

		actualTotal := int64(0)
		for _, count := range allCounts {
			actualTotal += count
		}

		if actualTotal != expectedTotal {
			t.Errorf("GetAllJobStatusCounts() total count = %d, want %d", actualTotal, expectedTotal)
		} else {
			t.Logf("Successfully counted %d total jobs across all statuses", actualTotal)
		}
	})

	t.Run("get job count with no jobs", func(t *testing.T) {
		// Create test user with no jobs
		user := CreateTestUser(t, fs, "test-user-no-jobs", "no-jobs@example.com", "+1234567890", false)
		defer DeleteTestData(t, fs, user.ID)

		// Test GetJobsCount for a status
		count, err := GetJobsCount(ctx, fs, user.ID, JobStatusNew)
		if err != nil {
			t.Errorf("GetJobsCount() error = %v", err)
		}

		if count != 0 {
			t.Errorf("GetJobsCount() for user with no jobs got count = %d, want 0", count)
		}

		// Test GetAllJobStatusCounts
		allCounts, err := GetAllJobStatusCounts(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("GetAllJobStatusCounts() error = %v", err)
		}

		for status, count := range allCounts {
			if count != 0 {
				t.Errorf("GetAllJobStatusCounts() for status %s got count = %d, want 0", status, count)
			}
		}
	})

	t.Run("error cases", func(t *testing.T) {
		// Test with empty user ID
		_, err := GetJobsCount(ctx, fs, "", JobStatusNew)
		if err == nil {
			t.Error("GetJobsCount() with empty user ID should return error")
		}

		_, err = GetAllJobStatusCounts(ctx, fs, "")
		if err == nil {
			t.Error("GetAllJobStatusCounts() with empty user ID should return error")
		}
	})
}
