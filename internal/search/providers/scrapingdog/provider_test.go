package scrapingdog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"talent-rodeo/internal/search"
	"testing"
)

func TestScrapingDogProvider_SearchJobs(t *testing.T) {
	// Mock server to simulate ScrapingDog API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		if got := r.Method; got != "GET" {
			t.Errorf("request method = %q; want %q", got, "GET")
		}
		if got := r.URL.Path; got != "/linkedinjobs" {
			t.Errorf("request path = %q; want %q", got, "/linkedinjobs")
		}
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Errorf("api_key = %q; want %q", got, "test-key")
		}
		if got := r.URL.Query().Get("field"); got != "golang" {
			t.Errorf("field = %q; want %q", got, "golang")
		}

		// Return mock response
		jobs := []jobListing{
			{
				Position:    "Senior Go Developer",
				Link:        "https://linkedin.com/jobs/1",
				ID:          "job1",
				CompanyName: "Test Company",
				Location:    "Remote",
				PostingDate: "2024-02-06",
				CompanyLogo: "https://example.com/logo.png",
			},
		}
		if err := json.NewEncoder(w).Encode(jobs); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewProvider("test-key", WithBaseURL(server.URL))
	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	ctx := context.Background()
	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	got, err := provider.SearchJobs(ctx, criteria)
	if err != nil {
		t.Fatalf("SearchJobs() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SearchJobs() returned %d jobs; want 1", len(got))
	}

	job := got[0]
	want := struct {
		Title         string
		Company       string
		Location      string
		ProviderJobID string
	}{
		Title:         "Senior Go Developer",
		Company:       "Test Company",
		Location:      "Remote",
		ProviderJobID: "job1",
	}

	if job.Title != want.Title {
		t.Errorf("job.Title = %q; want %q", job.Title, want.Title)
	}
	if job.Company != want.Company {
		t.Errorf("job.Company = %q; want %q", job.Company, want.Company)
	}
	if job.Location != want.Location {
		t.Errorf("job.Location = %q; want %q", job.Location, want.Location)
	}
	if job.ProviderJobID != want.ProviderJobID {
		t.Errorf("job.ProviderJobID = %q; want %q", job.ProviderJobID, want.ProviderJobID)
	}
}

func TestScrapingDogProvider_GetJobDetails(t *testing.T) {
	// Mock server to simulate ScrapingDog API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		if got := r.Method; got != "GET" {
			t.Errorf("request method = %q; want %q", got, "GET")
		}
		if got := r.URL.Path; got != "/linkedinjobs" {
			t.Errorf("request path = %q; want %q", got, "/linkedinjobs")
		}
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Errorf("api_key = %q; want %q", got, "test-key")
		}
		if got := r.URL.Query().Get("job_id"); got != "job1" {
			t.Errorf("job_id = %q; want %q", got, "job1")
		}

		// Return mock response
		details := []jobDetail{
			{
				Position:       "Senior Go Developer",
				Status:         "active",
				Location:       "Remote",
				CompanyName:    "Test Company",
				PostingTime:    "2024-02-06",
				BasePay:        "$150,000 - $200,000",
				Description:    "Looking for an experienced Go developer...",
				SeniorityLevel: "Senior",
				EmploymentType: "Full-time",
				JobFunction:    "Software Development",
				Industries:     "Technology",
				Recruiters: []recruiter{
					{
						Name:       "John Doe",
						Title:      "Technical Recruiter",
						ProfileURL: "https://linkedin.com/in/johndoe",
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(details); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewProvider("test-key", WithBaseURL(server.URL))
	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	ctx := context.Background()
	got, err := provider.GetJobDetails(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJobDetails() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetJobDetails() returned nil")
	}

	want := struct {
		Title          string
		Company        string
		Location       string
		EmploymentType string
		Industry       string
	}{
		Title:          "Senior Go Developer",
		Company:        "Test Company",
		Location:       "Remote",
		EmploymentType: "Full-time",
		Industry:       "Technology",
	}

	if got.Title != want.Title {
		t.Errorf("job.Title = %q; want %q", got.Title, want.Title)
	}
	if got.Company != want.Company {
		t.Errorf("job.Company = %q; want %q", got.Company, want.Company)
	}
	if got.Location != want.Location {
		t.Errorf("job.Location = %q; want %q", got.Location, want.Location)
	}
	if got.EmploymentType != want.EmploymentType {
		t.Errorf("job.EmploymentType = %q; want %q", got.EmploymentType, want.EmploymentType)
	}
	if got.Industry != want.Industry {
		t.Errorf("job.Industry = %q; want %q", got.Industry, want.Industry)
	}
	if got.SalaryInfo == nil {
		t.Error("job.SalaryInfo is nil, want non-nil")
	}
}

func TestScrapingDogProvider_Validate(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid api key",
			apiKey:  "test-key",
			wantErr: false,
		},
		{
			name:    "empty api key",
			apiKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.apiKey)
			err := provider.Validate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScrapingDogProvider_Name(t *testing.T) {
	provider := NewProvider("test-key")
	got := provider.Name()
	want := "scrapingdog"
	if got != want {
		t.Errorf("Name() = %q; want %q", got, want)
	}
}
