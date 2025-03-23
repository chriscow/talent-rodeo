package scrapingdog

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load("../../../../.env"); err != nil {
		log.Println("No .env file found, using system env")
	}
}

func setupTestStore(t *testing.T) *JobStore {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store, err := NewJobStore(context.Background(), logger)
	if err != nil {
		t.Fatalf("Failed to create job store: %v", err)
	}
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
	docs, err := store.fs.Collection("job_listings").Where("search_hash", "==", searchHash).Documents(ctx).GetAll()
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
	ctx := context.Background()

	// Test data
	postedDate := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
	testJobs := []search.JobListing{
		{
			Provider:       testProvider,
			ProviderJobID:  "job1",
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

	// Manually update the last_searched_at to be 5 hours ago
	searchRef := store.fs.Collection("job_searches").Doc(searchHash)
	_, err := searchRef.Set(ctx, map[string]interface{}{
		"keywords":         criteria.Keywords,
		"location":         criteria.Location,
		"last_searched_at": time.Now().Add(-5 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Failed to set search timestamp: %v", err)
	}

	// Try to retrieve the expired job
	cachedJobs, err := store.GetCachedJobs(ctx, criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	// Should not return expired jobs (older than 4 hours)
	if len(cachedJobs) != 0 {
		t.Errorf("Expected 0 cached jobs (due to expiration), got %d", len(cachedJobs))
	}
}
