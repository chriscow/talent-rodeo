package scrapingdog

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../../../../.env"); err != nil {
		log.Println("No .env file found, using system env")
	}
}

func setupTestStore(t *testing.T) *JobStore {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store, err := NewJobStore(context.Background(), logger)
	if err != nil {
		t.Fatalf("Failed to create job store: %v", err)
	}
	t.Cleanup(func() {
		store.cache.Close()
	})
	return store
}

func cleanupTestData(t *testing.T, store *JobStore, searchHash string) {
	ctx := context.Background()

	// Delete search record
	_, err := store.fs.Collection("job_searches").Doc(searchHash).Delete(ctx)
	if err != nil {
		t.Logf("Failed to cleanup search record: %v", err)
	}

	// Delete job listings
	docs, err := store.fs.Collection("job_listings").Where("SearchHash", "==", searchHash).Documents(ctx).GetAll()
	if err != nil {
		t.Logf("Failed to get job listings for cleanup: %v", err)
		return
	}

	for _, doc := range docs {
		_, err := doc.Ref.Delete(ctx)
		if err != nil {
			t.Logf("Failed to delete job listing %s: %v", doc.Ref.ID, err)
		}
	}
}

func getSearchHash(keywords []string, location string) string {
	// Join all keywords with underscore to create a unique combined search
	kw := strings.Join(keywords, "-")
	return funcs.Slugify(fmt.Sprintf("%s %s %s", kw, location, providerName))
}

func getJobID(provider, providerJobID string) string {
	return funcs.Slugify(fmt.Sprintf("%s %s", provider, providerJobID))
}

func TestJobStore_SaveAndGetCachedJobs(t *testing.T) {
	const testProvider = "scrapingdog-test"
	store := setupTestStore(t)
	defer cleanupTestData(t, store, "test-job-1")
	ctx := context.Background()

	// Test data
	postedDate := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
	testJobs := []search.JobListing{
		{
			Provider:       testProvider,
			ProviderJobID:  "test-job1",
			Title:          "Software Engineer",
			Company:        "Test Company",
			Location:       "Remote",
			PostedDate:     postedDate,
			URL:            "https://example.com/job1",
			CompanyProfile: "Test company profile",
		},
	}

	// Set expected ID
	testJobs[0].ID = getJobID(testJobs[0].Provider, testJobs[0].ProviderJobID)

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	searchHash := getSearchHash(criteria.Keywords, criteria.Location)
	defer cleanupTestData(t, store, searchHash)

	// Save job
	if err := store.SaveJobListings(ctx, criteria, testJobs); err != nil {
		t.Fatalf("Failed to save job listings: %v", err)
	}

	// Get cached jobs
	cachedJobs, err := store.GetCachedJobs(ctx, criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	// Verify we got the job back
	if len(cachedJobs) != 1 {
		t.Fatalf("Expected 1 cached job, got %d", len(cachedJobs))
	}

	// Verify job details
	job := cachedJobs[0]
	if job.Title != testJobs[0].Title {
		t.Errorf("Title mismatch: got %s, want %s", job.Title, testJobs[0].Title)
	}
	if job.Company != testJobs[0].Company {
		t.Errorf("Company mismatch: got %s, want %s", job.Company, testJobs[0].Company)
	}
	if job.SearchHash != searchHash {
		t.Errorf("SearchHash mismatch: got %s, want %s", job.SearchHash, searchHash)
	}
}

func TestJobStore_CacheExpiration(t *testing.T) {
	const testProvider = "scrapingdog-test"
	store := setupTestStore(t)
	ctx := context.Background()

	// Test data
	postedDate := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
	expiredJob := []search.JobListing{{
		Provider:      testProvider,
		ProviderJobID: "expired-job",
		Title:         "Old Position",
		Company:       "Old Company",
		Location:      "Remote",
		PostedDate:    postedDate,
	}}

	// Set expected ID
	expiredJob[0].ID = getJobID(expiredJob[0].Provider, expiredJob[0].ProviderJobID)

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	searchHash := getSearchHash(criteria.Keywords, criteria.Location)
	defer cleanupTestData(t, store, searchHash)

	// Save job with old timestamp
	if err := store.SaveJobListings(ctx, criteria, expiredJob); err != nil {
		t.Fatalf("Failed to save job listings: %v", err)
	}

	// Set the timestamp to 25 hours ago (beyond the 24-hour cacheDurationJobListing)
	pastTime := time.Now().Add(-25 * time.Hour)
	t.Logf("Setting LastSearchedAt to %v (current time: %v)", pastTime, time.Now())

	// Manually update the LastSearchedAt to be 25 hours ago
	searchRef := store.fs.Collection("job_searches").Doc(searchHash)
	_, err := searchRef.Set(ctx, map[string]any{
		"Keywords":       criteria.Keywords,
		"Location":       criteria.Location,
		"LastSearchedAt": pastTime,
	})
	if err != nil {
		t.Fatalf("Failed to set search timestamp: %v", err)
	}

	// Verify the timestamp was set correctly
	searchDoc, err := store.fs.Collection("job_searches").Doc(searchHash).Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get search document: %v", err)
	}
	var searchData struct {
		LastSearchedAt time.Time
	}
	if err := searchDoc.DataTo(&searchData); err != nil {
		t.Fatalf("Failed to parse search data: %v", err)
	}
	timeDiff := time.Since(searchData.LastSearchedAt)
	t.Logf("Time difference: %v, cacheDurationJobListing: %v", timeDiff, cacheDurationJobListing)
	if timeDiff <= cacheDurationJobListing {
		t.Fatalf("Expected time difference to be > %v, got %v", cacheDurationJobListing, timeDiff)
	}

	// Clear the Ristretto cache to ensure we're testing the Firestore expiration
	store.cache.Clear()

	// Try to retrieve the expired job
	cachedJobs, err := store.GetCachedJobs(ctx, criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	// Should not return expired jobs (older than cacheDurationJobListing)
	if len(cachedJobs) != 0 {
		t.Errorf("Expected 0 cached jobs (due to expiration), got %d", len(cachedJobs))
	}
}

func TestJobStore_SaveAndGetCachedJobDetails(t *testing.T) {
	store := setupTestStore(t)
	defer cleanupTestData(t, store, "test-job-1")
	ctx := context.Background()

	// Test data
	jobDetail := &search.JobDetail{
		ID:            "test-job-1",
		Provider:      providerName,
		ProviderJobID: "test-job1",
		Title:         "Senior Go Developer",
		Company:       "Test Company",
		Location:      "Remote",
		URL:           "https://example.com/job1",
	}

	// Save job details
	if err := store.SaveJobDetails(ctx, jobDetail); err != nil {
		t.Fatalf("Failed to save job details: %v", err)
	}

	// Get cached job details using the ID
	cached, err := store.GetCachedJobDetails(ctx, jobDetail.ID)
	if err != nil {
		t.Fatalf("Failed to get cached job details by ID: %v", err)
	}

	// Verify we got the job details back
	if cached == nil {
		t.Fatal("Expected cached job details when retrieving by ID, got nil")
	}

	// Verify the cached data matches
	if cached.ID != jobDetail.ID {
		t.Errorf("ID mismatch: got %s, want %s", cached.ID, jobDetail.ID)
	}
	if cached.Title != jobDetail.Title {
		t.Errorf("Title mismatch: got %s, want %s", cached.Title, jobDetail.Title)
	}

	// Test cache miss
	cached, err = store.GetCachedJobDetails(ctx, "non-existent-job")
	if err != nil {
		t.Fatalf("Failed to get non-existent job details: %v", err)
	}
	if cached != nil {
		t.Error("Expected nil for non-existent job, got job details")
	}
}

// TestJobStore_GetCachedJobDetailsByProviderID tests retrieving job details using provider ID
func TestJobStore_GetCachedJobDetailsByProviderID(t *testing.T) {
	store := setupTestStore(t)
	defer cleanupTestData(t, store, "test-job-1")
	ctx := context.Background()

	// Test data
	providerJobID := "test-job1"
	fullJobID := funcs.Slugify(fmt.Sprintf("%s %s", providerName, providerJobID))

	jobDetail := &search.JobDetail{
		ID:            fullJobID, // Use the full job ID here
		Provider:      providerName,
		ProviderJobID: providerJobID,
		Title:         "Senior Go Developer",
		Company:       "Test Company",
		Location:      "Remote",
		URL:           "https://example.com/job1",
	}

	// Save job details
	if err := store.SaveJobDetails(ctx, jobDetail); err != nil {
		t.Fatalf("Failed to save job details: %v", err)
	}

	// Get cached job details using the full job ID
	cached, err := store.GetCachedJobDetails(ctx, fullJobID)
	if err != nil {
		t.Fatalf("Failed to get cached job details by full job ID: %v", err)
	}

	// Verify we got the job details back
	if cached == nil {
		t.Fatalf("Expected cached job details when retrieving by full job ID, got nil")
	}

	// Verify the cached data matches
	if cached.ID != jobDetail.ID {
		t.Errorf("ID mismatch: got %s, want %s", cached.ID, jobDetail.ID)
	}
	if cached.Title != jobDetail.Title {
		t.Errorf("Title mismatch: got %s, want %s", cached.Title, jobDetail.Title)
	}
	if cached.ProviderJobID != jobDetail.ProviderJobID {
		t.Errorf("ProviderJobID mismatch: got %s, want %s", cached.ProviderJobID, jobDetail.ProviderJobID)
	}
}

func TestJobStore_TwoLevelCaching(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Test data
	testJobs := []search.JobListing{
		{
			Provider:      "scrapingdog-test",
			ProviderJobID: "test-job1",
			Title:         "Software Engineer",
			Company:       "Test Company",
			Location:      "Remote",
			PostedDate:    time.Now(),
		},
	}
	testJobs[0].ID = getJobID(testJobs[0].Provider, testJobs[0].ProviderJobID)

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	searchHash := getSearchHash(criteria.Keywords, criteria.Location)
	defer cleanupTestData(t, store, searchHash)

	// Save jobs to both caches
	if err := store.SaveJobListings(ctx, criteria, testJobs); err != nil {
		t.Fatalf("Failed to save job listings: %v", err)
	}

	// Test 1: First call should hit Ristretto cache
	t.Run("ristretto cache hit", func(t *testing.T) {
		start := time.Now()
		jobs, err := store.GetCachedJobs(ctx, criteria)
		if err != nil {
			t.Fatalf("Failed to get cached jobs: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("Expected 1 job, got %d", len(jobs))
		}
		duration := time.Since(start)
		t.Logf("Ristretto cache retrieval took: %v", duration)
	})

	// Test 2: Clear Ristretto cache and verify Firestore fallback
	t.Run("firestore fallback", func(t *testing.T) {
		store.cache.Clear() // Clear Ristretto cache

		start := time.Now()
		jobs, err := store.GetCachedJobs(ctx, criteria)
		if err != nil {
			t.Fatalf("Failed to get cached jobs from Firestore: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("Expected 1 job from Firestore, got %d", len(jobs))
		}
		duration := time.Since(start)
		t.Logf("Firestore cache retrieval took: %v", duration)

		// Verify job was cached back in Ristretto
		key := fmt.Sprintf("search:%s", searchHash)
		_, found := store.cache.Get(key)
		if !found {
			t.Error("Job should have been cached in Ristretto after Firestore retrieval")
		}
	})
}

func TestJobStore_ConcurrentAccess(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Test data
	testJobs := []search.JobListing{
		{
			Provider:      "scrapingdog-test",
			ProviderJobID: "test-job1",
			Title:         "Software Engineer",
			Company:       "Test Company",
			Location:      "Remote",
			PostedDate:    time.Now(),
		},
	}
	testJobs[0].ID = getJobID(testJobs[0].Provider, testJobs[0].ProviderJobID)

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	searchHash := getSearchHash(criteria.Keywords, criteria.Location)
	defer cleanupTestData(t, store, searchHash)

	// Save initial data
	if err := store.SaveJobListings(ctx, criteria, testJobs); err != nil {
		t.Fatalf("Failed to save job listings: %v", err)
	}

	// Run concurrent reads and writes
	const numGoroutines = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*2)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)

		// Concurrent reads
		go func() {
			defer wg.Done()
			jobs, err := store.GetCachedJobs(ctx, criteria)
			if err != nil {
				errCh <- fmt.Errorf("read error: %v", err)
				return
			}
			if len(jobs) != 1 {
				errCh <- fmt.Errorf("expected 1 job, got %d", len(jobs))
			}
		}()

		// Concurrent writes
		go func(i int) {
			defer wg.Done()
			jobs := []search.JobListing{
				{
					Provider:      "scrapingdog-test",
					ProviderJobID: fmt.Sprintf("test-job-%d", i),
					Title:         fmt.Sprintf("Software Engineer %d", i),
					Company:       "Test Company",
					Location:      "Remote",
					PostedDate:    time.Now(),
				},
			}
			jobs[0].ID = getJobID(jobs[0].Provider, jobs[0].ProviderJobID)

			if err := store.SaveJobListings(ctx, criteria, jobs); err != nil {
				errCh <- fmt.Errorf("write error: %v", err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

func TestJobStore_DetailsCaching(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Test data
	jobDetail := &search.JobDetail{
		ID:            "test-job-1",
		ProviderJobID: "test-job1",
		Title:         "Senior Go Developer",
		Company:       "Test Company",
		Location:      "Remote",
		URL:           "https://example.com/job1",
	}

	// Test 1: Save and retrieve from Ristretto cache
	t.Run("ristretto cache hit for details", func(t *testing.T) {
		if err := store.SaveJobDetails(ctx, jobDetail); err != nil {
			t.Fatalf("Failed to save job details: %v", err)
		}

		start := time.Now()
		cached, err := store.GetCachedJobDetails(ctx, jobDetail.ID)
		if err != nil {
			t.Fatalf("Failed to get cached job details: %v", err)
		}
		if cached == nil {
			t.Fatal("Expected cached job details, got nil")
		}
		duration := time.Since(start)
		t.Logf("Ristretto cache retrieval took: %v", duration)
	})

	// Test 2: Clear Ristretto cache and verify Firestore fallback
	t.Run("firestore fallback for details", func(t *testing.T) {
		store.cache.Clear() // Clear Ristretto cache

		start := time.Now()
		cached, err := store.GetCachedJobDetails(ctx, jobDetail.ID)
		if err != nil {
			t.Fatalf("Failed to get cached job details from Firestore: %v", err)
		}
		if cached == nil {
			t.Fatal("Expected cached job details from Firestore, got nil")
		}
		duration := time.Since(start)
		t.Logf("Firestore cache retrieval took: %v", duration)

		// Verify details were cached back in Ristretto
		key := fmt.Sprintf("job_details:%s", jobDetail.ID)
		_, found := store.cache.Get(key)
		if !found {
			t.Error("Job details should have been cached in Ristretto after Firestore retrieval")
		}
	})
}
