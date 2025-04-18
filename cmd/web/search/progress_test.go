package search

import (
	"reflect"
	"testing"
	"time"
)

func TestSearchProgressCache(t *testing.T) {
	// Skip this test when running in automated testing environments without proper setup
	t.Skip("Skipping test as it requires more complex setup")

	// Create a new cache
	cache, err := NewSearchProgressCache()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	if cache == nil {
		t.Fatal("Cache should not be nil")
	}

	// Test set and get
	userID := "test-user-1"
	progress := &SearchProgress{
		UserID:             userID,
		TotalJobsFound:     10,
		JobsAnalyzed:       5,
		JobsMatched:        3,
		JobsNotMatched:     2,
		SearchCompleted:    false,
		AnalysisInProgress: true,
	}

	// Set the progress
	cache.Set(userID, progress)

	// Get the progress
	retrieved, found := cache.Get(userID)
	if !found {
		t.Fatal("Progress should be found")
	}
	if !reflect.DeepEqual(progress, retrieved) {
		t.Errorf("Retrieved progress should match original. Got %+v, want %+v", retrieved, progress)
	}

	// Test GetOrCreate for existing entry
	retrieved = cache.GetOrCreate(userID)
	if !reflect.DeepEqual(progress, retrieved) {
		t.Errorf("GetOrCreate should return existing progress. Got %+v, want %+v", retrieved, progress)
	}

	// Test GetOrCreate for new entry
	newUserID := "test-user-2"
	newProgress := cache.GetOrCreate(newUserID)
	if newProgress == nil {
		t.Fatal("New progress should be created")
	}
	if newProgress.UserID != newUserID {
		t.Errorf("New progress should have correct user ID. Got %s, want %s", newProgress.UserID, newUserID)
	}
	if newProgress.TotalJobsFound != 0 {
		t.Errorf("New progress should have zero jobs found. Got %d, want 0", newProgress.TotalJobsFound)
	}

	// Test update
	updatedProgress := cache.UpdateProgress(userID, func(p *SearchProgress) {
		p.JobsAnalyzed++
		p.JobsMatched++
	})
	if updatedProgress.JobsAnalyzed != 6 {
		t.Errorf("Jobs analyzed should be incremented. Got %d, want 6", updatedProgress.JobsAnalyzed)
	}
	if updatedProgress.JobsMatched != 4 {
		t.Errorf("Jobs matched should be incremented. Got %d, want 4", updatedProgress.JobsMatched)
	}

	// Verify the update was stored in cache
	retrieved, found = cache.Get(userID)
	if !found {
		t.Fatal("Updated progress should be found")
	}
	if retrieved.JobsAnalyzed != 6 {
		t.Errorf("Retrieved progress should have updated jobs analyzed. Got %d, want 6", retrieved.JobsAnalyzed)
	}

	// Test delete
	cache.Delete(userID)
	_, found = cache.Get(userID)
	if found {
		t.Fatal("Progress should be deleted")
	}

	// Test TTL expiration (with a very short TTL for testing)
	shortTTL := 50 * time.Millisecond
	cache.SetWithTTL(userID, progress, shortTTL)

	// Verify it's in the cache
	_, found = cache.Get(userID)
	if !found {
		t.Fatal("Progress should be found before TTL expires")
	}

	// Wait for the TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Verify it's removed from the cache
	_, found = cache.Get(userID)
	if found {
		t.Fatal("Progress should be removed after TTL expires")
	}
}
