package subscription

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestProcessorIntegration(t *testing.T) {
	// Skip test if SHORT is set
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}

	// Create a logger that writes to a buffer for validation
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create test prefix for cleanup
	testPrefix := fmt.Sprintf("test-processor-%d", time.Now().UnixNano())

	// Set up cleanup
	t.Cleanup(func() {
		// Clean up test data
		internal.DeleteTestData(t, fs, testPrefix)
		internal.DeleteTestSearchCriteria(ctx, t, fs, testPrefix)
		internal.DeleteTestUserCriteriaByPrefix(ctx, t, fs, testPrefix)
		fs.Close()
	})

	t.Run("TestStart_AlreadyRunning", func(t *testing.T) {
		// Create a processor and start it
		processor := NewProcessor(fs, logger)
		processor.WithInterval(50 * time.Millisecond) // Use short interval for testing

		// Start the processor
		err := processor.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start processor: %v", err)
		}
		defer processor.Stop()

		// Trying to start it again should fail
		err = processor.Start(ctx)
		if err == nil {
			t.Fatal("Starting an already running processor should return an error")
		}
	})

	t.Run("TestProcess_EligibleCriteria", func(t *testing.T) {
		// Create processor with shorter interval for testing
		processor := NewProcessor(fs, logger)
		processor.WithInterval(100 * time.Millisecond)

		// Create a test user with steering query
		userEmail := fmt.Sprintf("%s@example.com", testPrefix)
		userPhone := "+12345678900"
		testUser := internal.CreateTestUser(t, fs, testPrefix, userEmail, userPhone, false)

		// Create a steering query for the user
		steeringQuery := "I am looking for a software engineer position with Go and cloud experience"
		testUser.SteeringQuery, err = internal.NewSteeringQuery(steeringQuery, internal.PineconeEmbedDense)
		if err != nil {
			t.Fatalf("Failed to create steering query: %v", err)
		}

		// Update user with steering query
		err := internal.UpdateUser(ctx, fs, testUser.ID, map[string]any{
			"SteeringQuery": testUser.SteeringQuery,
		})
		if err != nil {
			t.Fatalf("Failed to update user with steering query: %v", err)
		}

		recentTime := time.Now().Add(-12 * time.Hour)
		oldTime := time.Now().Add(-48 * time.Hour)

		criteria, found, err := internal.GetSearchCriteria(ctx, fs, "go-golang-103644278")
		if err != nil {
			t.Fatalf("Failed to get search criteria: %v", err)
		}
		if !found {
			criteria := &internal.SearchCriteria{
				ID:              "go-golang-103644278", // real criteria ID in pinecone
				Query:           "Go (Golang)",
				GeoID:           "us-ny-newyork",
				SubscriberCount: 1,
			}
			_, err = fs.Collection(`search_criteria`).Doc(criteria.ID).Create(ctx, criteria)
			if err != nil {
				t.Fatalf("Failed to create test search criteria: %v", err)
			}
		}
		criteria.LastPublishedAt = &oldTime
		criteria.LastSearchedAt = &recentTime
		err = internal.UpdateSearchCriteria(ctx, fs, criteria)
		if err != nil {
			t.Fatalf("Failed to update search criteria: %v", err)
		}

		// Subscribe the test user to the criteria
		err = internal.SubscribeToCriteria(ctx, fs, testUser.ID, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to subscribe user to criteria: %v", err)
		}
		defer func() {
			internal.UnsubscribeFromCriteria(ctx, fs, testUser.ID, criteria.ID)
		}()

		jobDetail := &internal.JobDetail{
			ID:          "scrapingdog:linkedin:3787738366",
			Title:       "Software Engineer",
			Company:     "Scrapingdog",
			Location:    "New York, NY",
			Description: "We are looking for a software engineer with 3+ years of experience in Go and cloud technologies.",
			URL:         "https://www.scrapingdog.com/jobs/software-engineer",
		}
		err = internal.SaveJobDetail(ctx, fs, jobDetail)
		if err != nil {
			t.Fatalf("Failed to create test job detail: %v", err)
		}

		err = internal.LinkJobDetailToCriteria(ctx, fs, criteria.ID, jobDetail.ID)
		if err != nil {
			t.Fatalf("Failed to link job detail to criteria: %v", err)
		}

		// Start the processor
		err = processor.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start processor: %v", err)
		}
		defer processor.Stop()

		// Wait for the processor to run and process criteria
		time.Sleep(200 * time.Millisecond)

		// Verify that the LastPublishedAt field of the criteria was updated
		updatedCriteria, found, err := internal.GetSearchCriteria(ctx, fs, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to get updated criteria: %v", err)
		}
		if !found {
			t.Fatal("Criteria not found after processing")
		}

		// Check LastPublishedAt is recent (within the last few minutes)
		if updatedCriteria.LastPublishedAt == nil {
			t.Fatal("LastPublishedAt is still nil after processing")
		}

		timeSinceUpdate := time.Since(*updatedCriteria.LastPublishedAt)
		if timeSinceUpdate > time.Minute {
			t.Fatalf("LastPublishedAt was not updated recently: %v", timeSinceUpdate)
		}
	})

	t.Run("TestProcessCriteria_NoSubscribers", func(t *testing.T) {
		// Create a processor (not started)
		processor := NewProcessor(fs, logger)

		// Create test criteria with no subscribers
		criteriaQuery := fmt.Sprintf("%s-no-subs", testPrefix)
		geoID := "us-ny-newyork"
		criteria, err := internal.CreateSearchCriteria(ctx, fs, criteriaQuery, geoID, 2)
		if err != nil {
			t.Fatalf("Failed to create test criteria: %v", err)
		}
		defer func() {
			internal.DeleteTestSearchCriteria(ctx, t, fs, criteriaQuery)
		}()

		// Set LastSearchedAt to a recent time
		recentTime := time.Now().Add(-12 * time.Hour)
		criteria.LastSearchedAt = &recentTime

		// Ensure SubscriberCount is 0
		criteria.SubscriberCount = 0

		// Update criteria
		err = internal.UpdateSearchCriteria(ctx, fs, criteria)
		if err != nil {
			t.Fatalf("Failed to update criteria: %v", err)
		}

		// Process the criteria - should skip due to no subscribers
		err = processor.processCriteria(ctx, *criteria)
		if err != nil {
			t.Fatalf("processCriteria returned error: %v", err)
		}

		// Verify LastPublishedAt was not updated
		updatedCriteria, found, err := internal.GetSearchCriteria(ctx, fs, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to get updated criteria: %v", err)
		}
		if !found {
			t.Fatal("Criteria not found after processing")
		}

		// LastPublishedAt should still be nil
		if updatedCriteria.LastPublishedAt != nil {
			t.Fatal("LastPublishedAt was updated when it should not have been")
		}
	})

	t.Run("TestProcessCriteria_NoNewJobs", func(t *testing.T) {
		// Create a processor (not started)
		processor := NewProcessor(fs, logger)

		// Create a test user
		userEmail := fmt.Sprintf("%s-nojobs@example.com", testPrefix)
		userPhone := "+12345678901"
		testUser := internal.CreateTestUser(t, fs, fmt.Sprintf("%s-nojobs", testPrefix), userEmail, userPhone, false)

		// Create test criteria with no recent jobs
		criteriaQuery := fmt.Sprintf("%s-no-jobs", testPrefix)
		geoID := "us-ca-losangeles"
		criteria, err := internal.CreateSearchCriteria(ctx, fs, criteriaQuery, geoID, 2)
		if err != nil {
			t.Fatalf("Failed to create test criteria: %v", err)
		}

		// Set LastSearchedAt to a recent time
		recentTime := time.Now().Add(-12 * time.Hour)
		criteria.LastSearchedAt = &recentTime

		// Ensure SubscriberCount is 1
		criteria.SubscriberCount = 1

		// Update criteria
		err = internal.UpdateSearchCriteria(ctx, fs, criteria)
		if err != nil {
			t.Fatalf("Failed to update criteria: %v", err)
		}

		// Subscribe the user to the criteria
		err = internal.SubscribeToCriteria(ctx, fs, testUser.ID, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to subscribe user to criteria: %v", err)
		}

		// Process the criteria - should skip due to no new jobs
		err = processor.processCriteria(ctx, *criteria)
		if err != nil {
			t.Fatalf("processCriteria returned error: %v", err)
		}

		// Verify LastPublishedAt was not updated
		updatedCriteria, found, err := internal.GetSearchCriteria(ctx, fs, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to get updated criteria: %v", err)
		}
		if !found {
			t.Fatal("Criteria not found after processing")
		}

		// LastPublishedAt should still be nil (or old)
		if updatedCriteria.LastPublishedAt != nil && time.Since(*updatedCriteria.LastPublishedAt) < time.Hour {
			t.Fatal("LastPublishedAt was updated when it should not have been")
		}
	})

	t.Run("TestProcessUserJobs_NoSteeringQuery", func(t *testing.T) {
		// Create a test user without steering query
		userEmail := fmt.Sprintf("%s-nosteering@example.com", testPrefix)
		userPhone := "+12345678902"
		testUser := internal.CreateTestUser(t, fs, fmt.Sprintf("%s-nosteering", testPrefix), userEmail, userPhone, false)

		// Create test criteria
		criteriaQuery := fmt.Sprintf("%s-nosteering", testPrefix)
		criteria, err := internal.CreateSearchCriteria(ctx, fs, criteriaQuery, "us-tx-dallas", 2)
		if err != nil {
			t.Fatalf("Failed to create test criteria: %v", err)
		}

		// Process user jobs - should skip due to no steering query
		err = internal.RefreshUserJobs(ctx, fs, testUser, *criteria)
		if err == nil {
			t.Fatalf("RefreshUserJobs should return an error for user without steering query")
		}

		// Verify no user jobs were created
		userJobs, err := internal.ListUserJobs(ctx, fs, testUser.ID)
		if err != nil {
			t.Fatalf("Failed to list user jobs: %v", err)
		}

		if len(userJobs) > 0 {
			t.Fatal("User jobs were created when they should not have been")
		}
	})

	t.Run("TestProcess_EnsureFrequencyThresholds", func(t *testing.T) {
		// Create a processor (not started)
		processor := NewProcessor(fs, logger)

		// Create test criteria that has been published recently
		criteriaQuery := fmt.Sprintf("%s-recent-pub", testPrefix)
		geoID := "us-fl-miami"
		criteria, err := internal.CreateSearchCriteria(ctx, fs, criteriaQuery, geoID, 2)
		if err != nil {
			t.Fatalf("Failed to create test criteria: %v", err)
		}

		// Set SubscriberCount to 1
		criteria.SubscriberCount = 1

		// Set LastSearchedAt to a recent time
		recentTime := time.Now().Add(-12 * time.Hour)
		criteria.LastSearchedAt = &recentTime

		// Set LastPublishedAt to a recent time (within 24 hours)
		recentPubTime := time.Now().Add(-12 * time.Hour)
		criteria.LastPublishedAt = &recentPubTime

		// Update criteria
		err = internal.UpdateSearchCriteria(ctx, fs, criteria)
		if err != nil {
			t.Fatalf("Failed to update criteria: %v", err)
		}

		// Process function will filter out this criteria
		// because it's been published too recently
		err = processor.process(ctx)
		if err != nil {
			t.Fatalf("process returned error: %v", err)
		}

		// Verify LastPublishedAt was not updated
		updatedCriteria, found, err := internal.GetSearchCriteria(ctx, fs, criteria.ID)
		if err != nil {
			t.Fatalf("Failed to get updated criteria: %v", err)
		}
		if !found {
			t.Fatal("Criteria not found after processing")
		}

		// LastPublishedAt should not have been updated (should still be the same)
		if updatedCriteria.LastPublishedAt == nil ||
			updatedCriteria.LastPublishedAt.Unix() != recentPubTime.Unix() {
			t.Fatal("LastPublishedAt was changed when it should not have been")
		}
	})
}
