package internal

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSearchCriteria(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("newTestFirestoreClient() error = %v", err)
	}

	// Clean up test data before and after tests
	testPrefix := "test-criteria-"
	DeleteTestSearchCriteria(ctx, t, client, testPrefix)
	t.Cleanup(func() {
		DeleteTestSearchCriteria(ctx, t, client, testPrefix)
		client.Close()
	})

	t.Run("CreateAndGetSearchCriteria", func(t *testing.T) {
		// Create unique test data
		timestamp := time.Now().UnixNano()
		query := fmt.Sprintf("%s-query-%d", testPrefix, timestamp)
		geoID := "us-wa-seattle"
		maxPages := 5
		age := ListingAgeMonth
		workType := WorkTypeRemote
		expLevel := ExperienceLevelEntryLevel

		// Create search criteria
		created, err := CreateSearchCriteria(ctx, client, query, geoID, workType, expLevel, maxPages, age)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() error = %v", err)
		}

		// Verify fields
		if created.Query != query {
			t.Errorf("Query = %v, want %v", created.Query, query)
		}
		if created.GeoID != geoID {
			t.Errorf("GeoID = %v, want %v", created.GeoID, geoID)
		}
		if created.MaxPages != maxPages {
			t.Errorf("MaxPages = %v, want %v", created.MaxPages, maxPages)
		}
		if created.LastSearchedAt != nil {
			t.Errorf("LastSearchedAt = %v, want nil", created.LastSearchedAt)
		}

		if created.ListingAge != age {
			t.Errorf("ListingAge = %v, want %v", created.ListingAge, age)
		}

		// Get created criteria by ID
		got, found, err := GetSearchCriteria(ctx, client, created.ID)
		if err != nil {
			t.Fatalf("GetSearchCriteria() error = %v", err)
		}
		if !found {
			t.Fatalf("GetSearchCriteria() found = %v, want %v", found, true)
		}

		// Verify retrieved data matches
		if got.ID != created.ID {
			t.Errorf("ID = %v, want %v", got.ID, created.ID)
		}
		if got.Query != query {
			t.Errorf("Query = %v, want %v", got.Query, query)
		}
		if got.GeoID != geoID {
			t.Errorf("GeoID = %v, want %v", got.GeoID, geoID)
		}
		if got.MaxPages != maxPages {
			t.Errorf("MaxPages = %v, want %v", got.MaxPages, maxPages)
		}
	})

	t.Run("UpdateSearchCriteria", func(t *testing.T) {
		// Create test data
		timestamp := time.Now().UnixNano()
		query := fmt.Sprintf("%s-query-update-%d", testPrefix, timestamp)
		geoID := "us-ca-sanfrancisco"
		maxPages := 3
		age := ListingAgeMonth
		workType := WorkTypeHybrid
		expLevel := ExperienceLevelEntryLevel

		// Create search criteria
		created, err := CreateSearchCriteria(ctx, client, query, geoID, workType, expLevel, maxPages, age)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() error = %v", err)
		}

		// Update LastSearchedAt
		now := time.Now()
		created.LastSearchedAt = &now

		err = UpdateSearchCriteria(ctx, client, created)
		if err != nil {
			t.Fatalf("UpdateSearchCriteria() error = %v", err)
		}

		// Get updated criteria
		updated, found, err := GetSearchCriteria(ctx, client, created.ID)
		if err != nil {
			t.Fatalf("GetSearchCriteria() after update error = %v", err)
		}
		if !found {
			t.Fatalf("GetSearchCriteria() after update found = %v, want %v", found, true)
		}

		// Verify LastSearchedAt was updated
		if updated.LastSearchedAt == nil {
			t.Fatal("LastSearchedAt = nil, want non-nil")
		}

		// Compare time within a reasonable margin (1 second)
		timeDiff := updated.LastSearchedAt.Sub(now)
		if timeDiff < -time.Second || timeDiff > time.Second {
			t.Errorf("LastSearchedAt time difference too large: %v", timeDiff)
		}
	})

	t.Run("GetSearchCriteriaNeedsSearch", func(t *testing.T) {
		// Create test users for subscriptions
		timestamp := time.Now().UnixNano()
		user1 := CreateTestUser(t, client, "Test User 1", fmt.Sprintf("user1-%d@test.com", timestamp), "1234567890", false)
		user2 := CreateTestUser(t, client, "Test User 2", fmt.Sprintf("user2-%d@test.com", timestamp), "1234567891", false)

		// Clean up test users
		t.Cleanup(func() {
			DeleteTestUserCriteria(ctx, t, client, user1.ID)
			DeleteTestUserCriteria(ctx, t, client, user2.ID)
		})

		// Create criteria that needs search (never searched) with subscribers
		query1 := fmt.Sprintf("%s-never-searched-%d", testPrefix, timestamp)
		workType1 := WorkTypeRemote
		expLevel1 := ExperienceLevelEntryLevel
		created1, err := CreateSearchCriteria(ctx, client, query1, "us-ny-newyork", workType1, expLevel1, 2, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() never searched error = %v", err)
		}

		// Add subscribers to created1
		err = SubscribeToCriteria(ctx, client, user1.ID, created1.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user1 and created1 error = %v", err)
		}
		err = SubscribeToCriteria(ctx, client, user2.ID, created1.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user2 and created1 error = %v", err)
		}

		// Create criteria searched more than 24 hours ago with subscribers
		query2 := fmt.Sprintf("%s-old-search-%d", testPrefix, timestamp)
		workType2 := WorkTypeAtWork
		expLevel2 := ExperienceLevelEntryLevel
		created2, err := CreateSearchCriteria(ctx, client, query2, "us-tx-austin", workType2, expLevel2, 2, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() old search error = %v", err)
		}

		// Add subscribers to created2
		err = SubscribeToCriteria(ctx, client, user1.ID, created2.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user1 and created2 error = %v", err)
		}
		err = SubscribeToCriteria(ctx, client, user2.ID, created2.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user2 and created2 error = %v", err)
		}

		// Set LastSearchedAt to 25 hours ago
		oldTime := time.Now().Add(-25 * time.Hour)
		created2.LastSearchedAt = &oldTime
		err = UpdateSearchCriteria(ctx, client, created2)
		if err != nil {
			t.Fatalf("UpdateSearchCriteria() old search error = %v", err)
		}

		// Create criteria searched recently (should not need search) with subscribers
		query3 := fmt.Sprintf("%s-recent-search-%d", testPrefix, timestamp)
		workType3 := WorkTypeHybrid
		expLevel3 := ExperienceLevelEntryLevel
		created3, err := CreateSearchCriteria(ctx, client, query3, "us-fl-miami", workType3, expLevel3, 2, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() recent search error = %v", err)
		}

		// Add subscribers to created3
		err = SubscribeToCriteria(ctx, client, user1.ID, created3.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user1 and created3 error = %v", err)
		}
		err = SubscribeToCriteria(ctx, client, user2.ID, created3.ID)
		if err != nil {
			t.Fatalf("SubscribeToCriteria() for user2 and created3 error = %v", err)
		}

		// Set LastSearchedAt to 1 hour ago
		recentTime := time.Now().Add(-1 * time.Hour)
		created3.LastSearchedAt = &recentTime
		err = UpdateSearchCriteria(ctx, client, created3)
		if err != nil {
			t.Fatalf("UpdateSearchCriteria() recent search error = %v", err)
		}

		// Create criteria that needs search (never searched) with NO subscribers
		query4 := fmt.Sprintf("%s-never-searched-no-subs-%d", testPrefix, timestamp)
		workType4 := WorkTypeRemote
		expLevel4 := ExperienceLevelEntryLevel
		created4, err := CreateSearchCriteria(ctx, client, query4, "us-il-chicago", workType4, expLevel4, 2, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() no subscribers error = %v", err)
		}

		// Create criteria searched more than 24 hours ago with NO subscribers
		query5 := fmt.Sprintf("%s-old-search-no-subs-%d", testPrefix, timestamp)
		workType5 := WorkTypeAtWork
		expLevel5 := ExperienceLevelEntryLevel
		created5, err := CreateSearchCriteria(ctx, client, query5, "us-ga-atlanta", workType5, expLevel5, 2, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() old search no subscribers error = %v", err)
		}
		// Set LastSearchedAt to 25 hours ago and subscriber count to 0
		oldTime = time.Now().Add(-25 * time.Hour)
		created5.LastSearchedAt = &oldTime
		err = UpdateSearchCriteria(ctx, client, created5)
		if err != nil {
			t.Fatalf("UpdateSearchCriteria() old search no subscribers error = %v", err)
		}

		// Get criteria that needs search
		needsSearch, err := GetSearchCriteriaNeedsSearch(ctx, client)
		if err != nil {
			t.Fatalf("GetSearchCriteriaNeedsSearch() error = %v", err)
		}

		// Create map of IDs for easy checking
		foundIDs := make(map[string]bool)
		for _, c := range needsSearch {
			foundIDs[c.ID] = true
		}

		// Check if never searched criteria WITH subscribers is found
		if !foundIDs[created1.ID] {
			t.Errorf("Never searched criteria WITH subscribers (ID %s) not found in results", created1.ID)
		}

		// Check if old search criteria WITH subscribers is found
		if !foundIDs[created2.ID] {
			t.Errorf("Old search criteria WITH subscribers (ID %s) not found in results", created2.ID)
		}

		// Check that recent search criteria WITH subscribers is NOT found (due to recent search)
		if foundIDs[created3.ID] {
			t.Errorf("Recent search criteria WITH subscribers (ID %s) was found, should not be included due to recent search", created3.ID)
		}

		// Check that never searched criteria with NO subscribers is NOT found (due to subscriber count)
		if foundIDs[created4.ID] {
			t.Errorf("Never searched criteria with NO subscribers (ID %s) was found, should not be included due to subscriber count", created4.ID)
		}

		// Check that old search criteria with NO subscribers is NOT found (due to subscriber count)
		if foundIDs[created5.ID] {
			t.Errorf("Old search criteria with NO subscribers (ID %s) was found, should not be included due to subscriber count", created5.ID)
		}

		// Count expected criteria from our test set
		expectedTestCriteriaCount := 0
		if foundIDs[created1.ID] {
			expectedTestCriteriaCount++
		}
		if foundIDs[created2.ID] {
			expectedTestCriteriaCount++
		}
		if foundIDs[created3.ID] {
			expectedTestCriteriaCount++
		}
		if foundIDs[created4.ID] {
			expectedTestCriteriaCount++
		}
		if foundIDs[created5.ID] {
			expectedTestCriteriaCount++
		}

		// Create a set of expected IDs for easier comparison
		expectedIDs := map[string]bool{
			created1.ID: true,
			created2.ID: true,
		}

		// Check for any unexpected criteria in results
		for _, c := range needsSearch {
			if !expectedIDs[c.ID] &&
				// Ignore criteria not part of our test (they may exist from other tests)
				(c.ID == created1.ID || c.ID == created2.ID || c.ID == created3.ID ||
					c.ID == created4.ID || c.ID == created5.ID) {
				t.Errorf("Unexpected criteria ID %s found in results that should not be there", c.ID)
			}
		}

		// Check for the exact number of criteria from our specific test
		expectedCount := 2 // Only created1 and created2 should match
		if expectedTestCriteriaCount != expectedCount {
			t.Errorf("Got %d test criteria results, want exactly %d", expectedTestCriteriaCount, expectedCount)
		}

		// If there are results from other tests, ensure they satisfy our criteria
		// This verifies the filter logic is working correctly
		for _, c := range needsSearch {
			// Skip our test criteria, we've already verified them
			if c.ID == created1.ID || c.ID == created2.ID || c.ID == created3.ID ||
				c.ID == created4.ID || c.ID == created5.ID {
				continue
			}

			// Verify all results need search (LastSearchedAt is nil or old)
			if c.LastSearchedAt != nil &&
				c.LastSearchedAt.After(time.Now().Add(-24*time.Hour)) {
				t.Errorf("Found criteria %s with recent LastSearchedAt = %v, all results should need search",
					c.ID, c.LastSearchedAt)
			}
		}
	})

	t.Run("GetNonExistentSearchCriteria", func(t *testing.T) {
		_, found, err := GetSearchCriteria(ctx, client, "non-existent-id")
		if err != nil {
			t.Fatalf("GetSearchCriteria() error = %v", err)
		}
		if found {
			t.Error("GetSearchCriteria() with non-existent ID should return false")
		}
	})

	t.Run("GetAllSearchCriteria", func(t *testing.T) {
		// Create a few test criteria with unique timestamps
		timestamp := time.Now().UnixNano()

		// Create first criteria
		query1 := fmt.Sprintf("%s-all-1-%d", testPrefix, timestamp)
		workType1 := WorkTypeRemote
		expLevel1 := ExperienceLevelEntryLevel
		created1, err := CreateSearchCriteria(ctx, client, query1, "us-wa-seattle", workType1, expLevel1, 3, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() for GetAll test error = %v", err)
		}

		// Create second criteria
		query2 := fmt.Sprintf("%s-all-2-%d", testPrefix, timestamp)
		workType2 := WorkTypeHybrid
		expLevel2 := ExperienceLevelEntryLevel
		created2, err := CreateSearchCriteria(ctx, client, query2, "us-ca-sanfrancisco", workType2, expLevel2, 4, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() for GetAll test error = %v", err)
		}

		// Get all criteria
		allCriteria, err := ListSearchCriteria(ctx, client)
		if err != nil {
			t.Fatalf("GetAllSearchCriteria() error = %v", err)
		}

		// Check that we have at least the two criteria we just created
		foundIDs := make(map[string]bool)
		for _, c := range allCriteria {
			foundIDs[c.ID] = true
		}

		// Verify both test criteria are found
		if !foundIDs[created1.ID] {
			t.Errorf("First test criteria ID %s not found in GetAllSearchCriteria results", created1.ID)
		}
		if !foundIDs[created2.ID] {
			t.Errorf("Second test criteria ID %s not found in GetAllSearchCriteria results", created2.ID)
		}
	})

	t.Run("DeleteSearchCriteria", func(t *testing.T) {
		// Create a criteria to delete
		timestamp := time.Now().UnixNano()
		query := fmt.Sprintf("%s-delete-%d", testPrefix, timestamp)
		workType := WorkTypeRemote
		expLevel := ExperienceLevelEntryLevel
		created, err := CreateSearchCriteria(ctx, client, query, "us-co-denver", workType, expLevel, 5, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() for DeleteSearchCriteria test error = %v", err)
		}

		// Verify it exists
		_, found, err := GetSearchCriteria(ctx, client, created.ID)
		if err != nil {
			t.Fatalf("GetSearchCriteria() before delete error = %v", err)
		}
		if !found {
			t.Fatalf("Created criteria not found before delete")
		}

		// Delete the criteria
		err = DeleteSearchCriteria(ctx, client, created.ID)
		if err != nil {
			t.Fatalf("DeleteSearchCriteria() error = %v", err)
		}

		// Verify it's gone
		_, found, err = GetSearchCriteria(ctx, client, created.ID)
		if err != nil {
			t.Fatalf("GetSearchCriteria() after delete error = %v", err)
		}
		if found {
			t.Errorf("Criteria still exists after deletion")
		}

		// Attempt to delete non-existent criteria (should not error)
		err = DeleteSearchCriteria(ctx, client, "non-existent-id")
		if err != nil {
			t.Errorf("DeleteSearchCriteria() with non-existent ID returned error = %v, want nil", err)
		}
	})

	t.Run("GetJobIDsForCriteria", func(t *testing.T) {
		// Create test search criteria
		timestamp := time.Now().UnixNano()
		query := fmt.Sprintf("%s-jobtests-%d", testPrefix, timestamp)
		geoID := "us-co-denver"
		maxPages := 2
		workType := WorkTypeRemote
		expLevel := ExperienceLevelEntryLevel

		criteria, err := CreateSearchCriteria(ctx, client, query, geoID, workType, expLevel, maxPages, ListingAgeMonth)
		if err != nil {
			t.Fatalf("CreateSearchCriteria() error = %v", err)
		}

		// Create test job-criteria links in Firestore using the new subcollection structure
		jobID1 := fmt.Sprintf("job1-%d", timestamp)
		jobID2 := fmt.Sprintf("job2-%d", timestamp)

		// Create links between jobs and criteria using subcollections
		ctx := context.Background()
		linkID1 := fmt.Sprintf("%s_%s", criteria.ID, jobID1)
		linkID2 := fmt.Sprintf("%s_%s", criteria.ID, jobID2)

		_, err = client.Collection("search_criteria").Doc(criteria.ID).Collection("job_details").Doc(linkID1).
			Create(ctx, map[string]interface{}{
				"CriteriaID": criteria.ID,
				"JobID":      jobID1,
				"CreatedAt":  time.Now(),
			})
		if err != nil {
			t.Fatalf("Failed to create job-criteria link 1: %v", err)
		}

		_, err = client.Collection("search_criteria").Doc(criteria.ID).Collection("job_details").Doc(linkID2).
			Create(ctx, map[string]interface{}{
				"CriteriaID": criteria.ID,
				"JobID":      jobID2,
				"CreatedAt":  time.Now(),
			})
		if err != nil {
			t.Fatalf("Failed to create job-criteria link 2: %v", err)
		}

		// Test cleanup for job_details subcollection
		t.Cleanup(func() {
			client.Collection("search_criteria").Doc(criteria.ID).Collection("job_details").Doc(linkID1).Delete(ctx)
			client.Collection("search_criteria").Doc(criteria.ID).Collection("job_details").Doc(linkID2).Delete(ctx)
		})

		// Get job IDs for criteria
		jobIDs, err := GetJobIDsForCriteria(ctx, client, criteria.ID)
		if err != nil {
			t.Fatalf("GetJobIDsForCriteria() error = %v", err)
		}

		// Check that both job IDs are found
		foundJob1 := false
		foundJob2 := false
		for _, id := range jobIDs {
			if id == jobID1 {
				foundJob1 = true
			}
			if id == jobID2 {
				foundJob2 = true
			}
		}

		if !foundJob1 {
			t.Errorf("Job ID %s not found in results", jobID1)
		}
		if !foundJob2 {
			t.Errorf("Job ID %s not found in results", jobID2)
		}

		// Check that we got exactly the expected number of jobs
		if len(jobIDs) != 2 {
			t.Errorf("Expected 2 job IDs, got %d", len(jobIDs))
		}
	})
}
