package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

type GeoID string

const (
	defaultBaseURL = "https://api.scrapingdog.com/linkedinjobs"
)

/*
France: 105015875
Belgium: 100565514
Spain: 105646813
England: 102299470
Germany: 101282230
Italy: 103350119
United States: 103644278
Canada: 101174742
Australia: 101452733
India: 102713980
China: 102890883
Japan: 101355337
Brazil: 106057199
Mexico: 103323778
Netherlands: 102890719
Singapore: 102454443
Switzerland: 106693272
Sweden: 105117694
South Korea: 105149562
Russia: 101728296
United Arab Emirates (UAE): 104305776
*/

// Could compare pricing to https://rapidapi.com/fantastic-jobs-fantastic-jobs-default/api/linkedin-jobs-api2/playground/apiendpoint_a6e7dfb5-61a7-4f5c-82e4-47b89a003e38

const (
	providerName = "scrapingdog"
	remoteGEOID  = GeoID("103644278")
)

var (
	geoIDMap = map[string]GeoID{
		"United States": "103644278",
	}
)

// Provider implements the JobProvider interface for ScrapingDog
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	fs      *firestore.Client
	store   *JobStore
	logger  *slog.Logger
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

func WithFirestore(fs *firestore.Client, logger *slog.Logger) Option {
	return func(p *Provider) {
		p.fs = fs
		p.logger = logger
		// Initialize store in SearchJobs since it needs context
	}
}

// NewProvider creates a new ScrapingDog provider
func NewProvider(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// SearchJobs searches for jobs based on the provided criteria
func (p *Provider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	// Initialize store if needed
	if p.fs != nil && p.store == nil {
		store, err := NewJobStore(ctx, p.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize job store: %w", err)
		}
		p.store = store
	}

	if p.store != nil {
		cachedJobs, err := p.store.GetCachedJobs(ctx, criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to get cached jobs: %w", err)
		}
		if len(cachedJobs) > 0 {
			return cachedJobs, nil
		}
	}

	jobs, err := p.searchJobs(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs: %w", err)
	}

	if p.store != nil {
		err := p.store.SaveJobListings(ctx, criteria, jobs)
		if err != nil {
			return nil, fmt.Errorf("failed to save jobs: %w", err)
		}
	}
	return jobs, nil
}

// searchJobs searches for jobs based on the provided criteria
func (p *Provider) searchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	log.Printf("Searching jobs with criteria: %+v", criteria)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)

	// We need to verify that "field" is not empty
	if len(criteria.Keywords) == 0 {
		return nil, fmt.Errorf("keywords are required")
	}

	// Handle keywords

	// We need to verify that "field" is not empty
	if len(criteria.Keywords) == 0 {
		return nil, fmt.Errorf("keywords are required")
	}

	// Handle keywords
	if len(criteria.Keywords) > 0 {
		params.Set("field", strings.Join(criteria.Keywords, " "))
	}

	// Handle location

	// Handle location
	if criteria.Location != "" {
		params.Set("geoid", criteria.Location)
	} else {
		params.Set("geoid", string(remoteGEOID))
	}

	// Handle pagination
	if criteria.PageNumber > 0 {
		params.Set("page", fmt.Sprintf("%d", criteria.PageNumber))
	} else {
		params.Set("page", "1")
	}

	// Handle sort by date posted
	if criteria.SortBy != "" {
		params.Set("sort_by", criteria.SortBy)
	}

	// Handle job type - take first one if multiple are specified
	if len(criteria.JobType) > 0 {
		params.Set("job_type", criteria.JobType[0])
	}

	// Handle experience level - take first one if multiple are specified
	if len(criteria.Experience) > 0 {
		params.Set("exp_level", criteria.Experience[0])
	}

	// Handle work type (remote/office/hybrid)
	if criteria.WorkType != "" {
		params.Set("work_type", criteria.WorkType)
	}

	// Handle company filter
	if criteria.CompanyID != "" {
		params.Set("company_id", criteria.CompanyID)
	} else {
		params.Set("page", "1")
	}

	// Handle sort by date posted
	if criteria.SortBy != "" {
		params.Set("sort_by", criteria.SortBy)
	}

	// Handle job type - take first one if multiple are specified
	if len(criteria.JobType) > 0 {
		params.Set("job_type", criteria.JobType[0])
	}

	// Handle experience level - take first one if multiple are specified
	if len(criteria.Experience) > 0 {
		params.Set("exp_level", criteria.Experience[0])
	}

	// Handle work type (remote/office/hybrid)
	if criteria.WorkType != "" {
		params.Set("work_type", criteria.WorkType)
	}

	// Handle company filter
	if criteria.CompanyID != "" {
		params.Set("company_id", criteria.CompanyID)
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

	log.Printf("ScrapingDog response received with status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var listings jobListings
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("ScrapingDog response decoded successfully, found %d listings", len(listings))

	// Convert to provider jobs
	jobs, err := convertJobListings(listings)
	if err != nil {
		return nil, fmt.Errorf("failed to convert job listings: %w", err)
	}

	log.Printf("Successfully converted %d job listings to search.JobListing format", len(jobs))
	return jobs, nil
}

func convertJobListings(jobListings []jobListing) ([]search.JobListing, error) {
	const provider = "scrapingdog"
	providerJobs := make([]search.JobListing, len(jobListings))
	for i, listing := range jobListings {
		postedDate, err := time.Parse("2006-01-02", listing.PostingDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse posting date: %w", err)
		}
		providerJobs[i] = search.JobListing{
			ID:             funcs.Slugify(fmt.Sprintf("%s %s", provider, listing.LinkedInJobID)),
			Title:          listing.Position,
			Company:        listing.CompanyName,
			Location:       listing.Location,
			PostedDate:     postedDate,
			URL:            listing.Link,
			Provider:       provider,
			ProviderJobID:  listing.LinkedInJobID,
			Description:    listing.CompanyProfile,
			CompanyProfile: listing.CompanyProfile,
			CompanyLogo:    listing.CompanyLogo,
		}
	}
	return providerJobs, nil
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

	var details []jobDetail
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
		salaryInfo, err = parseSalaryRange(ctx, detail.BasePay)
		if err != nil {
			return nil, fmt.Errorf("failed to parse salary range: %w", err)
		}
	}

	// Convert recruiters
	recruiters := make([]search.Recruiter, len(detail.Recruiters))
	for i, r := range detail.Recruiters {
		recruiters[i] = search.Recruiter{
			Photo:      r.Photo,
			Name:       r.Name,
			Title:      r.Title,
			ProfileURL: r.ProfileURL,
		}
	}

	// Convert similar jobs
	similarJobs := make([]search.RelatedJob, len(detail.SimilarJobs))
	for i, j := range detail.SimilarJobs {
		similarJobs[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert people also viewed
	peopleAlsoViewed := make([]search.RelatedJob, len(detail.PeopleAlsoViewed))
	for i, j := range detail.PeopleAlsoViewed {
		peopleAlsoViewed[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert to provider job detail
	jobDetail := &search.JobDetail{
		// Basic job information
		ID:            detail.CompanyLinkedInID,
		Provider:      providerName,
		ProviderJobID: jobID,
		Title:         detail.Position,
		Company:       detail.CompanyName,
		Location:      detail.Location,
		PostedDate:    postedTime,
		URL:           "", // Not available in detail response

		// Detailed information
		FullDescription: detail.Description,
		SalaryInfo:      salaryInfo,
		EmploymentType:  detail.EmploymentType,
		Industry:        detail.Industries,
		CompanyInfo: &search.Company{
			Name:     detail.CompanyName,
			Industry: detail.Industries,
		},

		// Additional fields from ScrapingDog
		Status:           detail.Status,
		SeniorityLevel:   detail.SeniorityLevel,
		JobFunction:      detail.JobFunction,
		Recruiters:       recruiters,
		SimilarJobs:      similarJobs,
		PeopleAlsoViewed: peopleAlsoViewed,
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
func parseSalaryRange(ctx context.Context, salary string) (*search.SalaryRange, error) {
	// This is a very basic implementation
	// In a real system, we would need more sophisticated parsing
	if salary == "" {
		return nil, nil
	}

	llm, err := gemini.NewProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	prompt := minds.Messages{{
		Role: minds.RoleUser,
		Content: `
		You are a helpful assistant that parses salary ranges from a given string.
		Please return the salary range in JSON format.

		Parse the following string:
		` + salary,
	}}

	schema, err := minds.NewResponseSchema("salary_range", "A salary range with a minimum and maximum value, currency, and period", search.SalaryRange{})
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	req := minds.NewRequest(prompt, minds.WithResponseSchema(*schema))

	response, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var salaryRange search.SalaryRange
	if err := json.Unmarshal([]byte(response.String()), &salaryRange); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &salaryRange, nil
}
