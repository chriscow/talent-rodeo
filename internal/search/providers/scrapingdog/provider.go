package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"talent-rodeo/internal/search"
	"time"
)

const (
	defaultBaseURL = "https://api.scrapingdog.com/linkedinjobs"
)

// Provider implements the JobProvider interface for ScrapingDog
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Option is a function that configures the Provider
type Option func(*Provider)

// WithBaseURL sets a custom base URL for the provider
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url + "/linkedinjobs"
	}
}

// WithHTTPClient sets a custom HTTP client for the provider
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// NewProvider creates a new ScrapingDog provider
func NewProvider(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// SearchJobs searches for jobs based on the provided criteria
func (p *Provider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.Job, error) {
	log.Printf("Searching jobs with criteria: %+v", criteria)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	if len(criteria.Keywords) > 0 {
		params.Set("field", strings.Join(criteria.Keywords, " "))
	}
	if criteria.Location != "" {
		params.Set("location", criteria.Location)
	}
	if criteria.PageSize > 0 {
		params.Set("page", fmt.Sprintf("%d", criteria.PageNumber))
	}

	// Make request
	url := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var listings jobListings
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to provider jobs
	jobs := make([]search.Job, len(listings))
	for i, listing := range listings {
		postedDate, _ := time.Parse("2006-01-02", listing.PostingDate)
		jobs[i] = search.Job{
			ID:            listing.ID,
			Title:         listing.Position,
			Company:       listing.CompanyName,
			Location:      listing.Location,
			PostedDate:    postedDate,
			Description:   listing.CompanyProfile,
			URL:           listing.Link,
			Provider:      "scrapingdog",
			ProviderJobID: listing.ID,
		}
	}

	log.Printf("Found %d jobs", len(jobs))
	return jobs, nil
}

// GetJobDetails retrieves detailed information about a specific job
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	log.Printf("Getting job details for ID: %s", jobID)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("job_id", jobID)

	// Make request
	url := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var details jobDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(details) == 0 {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	detail := details[0]
	postedTime, _ := time.Parse("2006-01-02", detail.PostingTime)

	// Parse salary range
	var salaryInfo *search.SalaryRange
	if detail.BasePay != "" {
		salaryInfo = parseSalaryRange(detail.BasePay)
	}

	// Convert to provider job detail
	jobDetail := &search.JobDetail{
		Job: search.Job{
			ID:            detail.CompanyLinkedInID,
			Title:         detail.Position,
			Company:       detail.CompanyName,
			Location:      detail.Location,
			PostedDate:    postedTime,
			Description:   detail.Description,
			Provider:      "scrapingdog",
			ProviderJobID: jobID,
		},
		FullDescription: detail.Description,
		EmploymentType:  detail.EmploymentType,
		Industry:        detail.Industries,
		SalaryInfo:      salaryInfo,
		CompanyInfo: &search.Company{
			Name:     detail.CompanyName,
			Industry: detail.Industries,
		},
	}

	log.Printf("Retrieved job details: %+v", jobDetail)
	return jobDetail, nil
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "scrapingdog"
}

// Validate verifies that the provider is properly configured
func (p *Provider) Validate(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("API key is required")
	}
	return nil
}

// parseSalaryRange attempts to parse a salary range string
func parseSalaryRange(salary string) *search.SalaryRange {
	// This is a very basic implementation
	// In a real system, we would need more sophisticated parsing
	if salary == "" {
		return nil
	}

	// Remove currency symbol and any whitespace
	salary = strings.TrimSpace(strings.TrimPrefix(salary, "$"))
	parts := strings.Split(salary, "-")
	if len(parts) != 2 {
		return nil
	}

	var min, max float64
	fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &min)
	fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &max)

	return &search.SalaryRange{
		Min:      min,
		Max:      max,
		Currency: "USD",
		Period:   "yearly",
	}
}
