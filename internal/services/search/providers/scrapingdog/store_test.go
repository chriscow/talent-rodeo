package scrapingdog

import (
	"context"
	"log"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/services/search"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load("../../../../../.env"); err != nil {
		log.Println("No .env file found, using system env")
	}
}

func TestJobStore_SaveAndGetCachedJobs(t *testing.T) {
	db := database.SetupTestDB(t)

	store := NewJobStore(db)
	ctx := context.Background()

	// Test data
	postedDate := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
	testJobs := []search.JobListing{
		{
			ID:             "job1",
			Provider:       "scrapingdog",
			ProviderJobID:  "job1",
			Title:          "Software Engineer",
			Company:        "Test Company",
			Location:       "Remote",
			PostedDate:     postedDate,
			URL:            "https://example.com/job1",
			CompanyProfile: "Test company profile",
		},
		{
			ID:             "job2",
			Provider:       "scrapingdog",
			ProviderJobID:  "job2",
			Title:          "Senior Developer",
			Company:        "Another Company",
			Location:       "Remote",
			PostedDate:     postedDate,
			URL:            "https://example.com/job2",
			CompanyProfile: "Another company profile",
		},
	}

	criteria := search.Criteria{
		Keywords: []string{"golang", "python"},
		Location: "Remote",
	}

	log.Printf("Saving test jobs: %+v", testJobs)

	// Test saving jobs
	err := store.SaveJobListings(ctx, criteria, testJobs)
	if err != nil {
		t.Fatalf("Failed to save job listings: %v", err)
	}

	// Test retrieving cached jobs
	cachedJobs, err := store.GetCachedJobs(criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	log.Printf("Retrieved cached jobs: %+v", cachedJobs)

	// Verify the number of cached jobs (should be 2 unique jobs after deduplication)
	if len(cachedJobs) != 2 {
		t.Errorf("Expected 2 unique cached jobs, got %d", len(cachedJobs))
		for i, job := range cachedJobs {
			t.Logf("Job %d: ID=%s, Title=%s", i+1, job.ID, job.Title)
		}
	}

	// Verify job details are preserved
	jobMap := make(map[string]search.JobListing)
	for _, job := range cachedJobs {
		jobMap[job.ID] = job
	}

	for _, expectedJob := range testJobs {
		cachedJob, exists := jobMap[expectedJob.ID]
		if !exists {
			t.Errorf("Job %s not found in cache", expectedJob.ID)
			continue
		}

		// Log the comparison for debugging
		log.Printf("Comparing job %s:", expectedJob.ID)
		log.Printf("  Expected: %+v", expectedJob)
		log.Printf("  Got:      %+v", cachedJob)

		if cachedJob.Title != expectedJob.Title {
			t.Errorf("Title mismatch for job %s: got %s, want %s",
				expectedJob.ID, cachedJob.Title, expectedJob.Title)
		}
		if cachedJob.Company != expectedJob.Company {
			t.Errorf("Company mismatch for job %s: got %s, want %s",
				expectedJob.ID, cachedJob.Company, expectedJob.Company)
		}
		if cachedJob.Location != expectedJob.Location {
			t.Errorf("Location mismatch for job %s: got %s, want %s",
				expectedJob.ID, cachedJob.Location, expectedJob.Location)
		}
		if cachedJob.URL != expectedJob.URL {
			t.Errorf("URL mismatch for job %s: got %s, want %s",
				expectedJob.ID, cachedJob.URL, expectedJob.URL)
		}
		if cachedJob.CompanyProfile != expectedJob.CompanyProfile {
			t.Errorf("CompanyProfile mismatch for job %s: got %s, want %s",
				expectedJob.ID, cachedJob.CompanyProfile, expectedJob.CompanyProfile)
		}
		if !cachedJob.PostedDate.Equal(expectedJob.PostedDate) {
			t.Errorf("PostedDate mismatch for job %s: got %v, want %v",
				expectedJob.ID, cachedJob.PostedDate, expectedJob.PostedDate)
		}
	}
}

func TestJobStore_CacheExpiration(t *testing.T) {
	db := database.SetupTestDB(t)

	store := NewJobStore(db)
	ctx := context.Background()

	// First create a company
	var companyID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO companies (name)
		VALUES ($1)
		RETURNING id`,
		"Old Company").Scan(&companyID)
	if err != nil {
		t.Fatalf("Failed to create company: %v", err)
	}

	// Insert a job with an old timestamp
	_, err = db.Exec(`
		INSERT INTO job_listings (
			job_id, position, link, company_name,
			location, posting_date, company_profile,
			last_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			NOW() - INTERVAL '5 hours'
		)`,
		"expired-job", "Old Position", "https://example.com/expired", "Old Company",
		"Remote", "2024-02-09", "Test job description")
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Try to retrieve the expired job
	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	cachedJobs, err := store.GetCachedJobs(criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	// Should not return expired jobs (older than 4 hours)
	if len(cachedJobs) != 0 {
		t.Errorf("Expected 0 cached jobs (due to expiration), got %d", len(cachedJobs))
	}
}

func TestJobStore_UpdateExistingJob(t *testing.T) {
	db := database.SetupTestDB(t)

	store := NewJobStore(db)
	ctx := context.Background()

	postedDate := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
	// Initial job data
	initialJob := []search.JobListing{{
		ID:             "job1",
		Provider:       "scrapingdog",
		ProviderJobID:  "job1",
		Title:          "Software Engineer",
		Company:        "Test Company",
		Location:       "Remote",
		PostedDate:     postedDate,
		URL:            "https://example.com/job1",
		CompanyProfile: "Test company profile",
	}}

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	// Save initial job
	err := store.SaveJobListings(ctx, criteria, initialJob)
	if err != nil {
		t.Fatalf("Failed to save initial job: %v", err)
	}

	// Updated job data
	updatedJob := []search.JobListing{{
		ID:             "job1",
		Provider:       "scrapingdog",
		ProviderJobID:  "job1",
		Title:          "Senior Software Engineer",
		Company:        "Test Company",
		Location:       "Remote",
		PostedDate:     postedDate,
		URL:            "https://example.com/job1",
		CompanyProfile: "Updated company profile",
	}}

	// Save updated job
	err = store.SaveJobListings(ctx, criteria, updatedJob)
	if err != nil {
		t.Fatalf("Failed to save updated job: %v", err)
	}

	// Retrieve the job
	cachedJobs, err := store.GetCachedJobs(criteria)
	if err != nil {
		t.Fatalf("Failed to get cached jobs: %v", err)
	}

	if len(cachedJobs) != 1 {
		t.Fatalf("Expected 1 cached job, got %d", len(cachedJobs))
	}

	// Verify the job was updated
	job := cachedJobs[0]
	if job.Title != "Senior Software Engineer" {
		t.Errorf("Title not updated: got %s, want Senior Software Engineer", job.Title)
	}
	if job.CompanyProfile != "Updated company profile" {
		t.Errorf("CompanyProfile not updated: got %s, want Updated company profile", job.CompanyProfile)
	}
}
