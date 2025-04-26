package scrapingdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

/*
# ScrapingDog Points to Remember

  - Each request will be retried until it can be successfully completed (up to 60
    seconds). Remember to set your timeout to 60 seconds to ensure this process
    goes smoothly. In cases where every request fails in 60 seconds, our API will
    return a 410 error, you may retry the request and you will not be charged for
    the unsuccessful request (you are only charged for successful requests, 200
    and 404 status codes). Make sure to catch these errors! They will occur on
    roughly 1-2% of requests for hard-to-scrape websites.

  - There is no overage allowed on the free plan, if you exceed 1000 requests per
    month on the free plan, you will receive a 403 error.

  - Each request will return a string containing the raw HTML from the page
    requested, along with any headers and cookies.

  - If you exceed your plan concurrent connection limit, the API will respond with
    a 429 status code, this can be solved by slowing down your request rate or
    upgrading to a bigger plan.

# Error Codes

| Code | Description |
|------|-------------|
| 200 | Successfull Request |
| 410 | Request timeout |
| 404 | URL is wrong |
| 202 | Your request is accepted and the scraping is still going on. | **This status code only appears in the LinkedIn Scraping API**
| 403 | Request Limit Reached. |
| 429 | Concurrent connection limit reached. |
| 401 | API Key is wrong. |
| 400 | Request failed. |
*/

const (
	defaultBaseURL = "https://api.scrapingdog.com/linkedinjobs"
	maxRetries     = 3
	retryDelay     = 5 * time.Second
)

/* DO NOT DELETE THIS
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

type GeoID string

const (
	providerName = "scrapingdog"
	remoteGEOID  = GeoID("103644278")
)

type QueuedSearch struct {
	UserID   string
	Criteria search.Criteria
}

// Custom error types for ScrapingDog API
type (
	ErrRequestTimeout   struct{ error }
	ErrInvalidURL       struct{ error }
	ErrRequestPending   struct{ error }
	ErrPlanLimitReached struct{ error }
	ErrConcurrentLimit  struct{ error }
	ErrInvalidAPIKey    struct{ error }
	ErrRequestFailed    struct{ error }
)

// Provider implements the JobProvider interface for ScrapingDog
type Provider struct {
	apiKey          string
	baseURL         string
	client          *http.Client
	fs              *internal.FirestoreClient
	store           *JobStore
	logger          *slog.Logger
	searchSemaphore chan struct{}          // Limits concurrent searches
	searchQueue     chan QueuedSearch      // Queue of searches to be performed
	shutdownCtx     context.Context        // Context used for shutdown signal
	shutdown        int32                  // Atomic flag: 0 = running, 1 = shutdown
	eventHandler    search.JobEventHandler // Handler for job-related events
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

func WithFirestore(fs *internal.FirestoreClient, logger *slog.Logger) Option {
	return func(p *Provider) {
		p.fs = fs
		p.logger = logger
		// Initialize store in SearchJobs since it needs context
	}
}

// WithJobStore sets the job store for the provider
func WithJobStore(store *JobStore) Option {
	return func(p *Provider) {
		p.store = store
	}
}

// WithEventHandler sets the job event handler for the provider
func WithEventHandler(handler search.JobEventHandler) Option {
	return func(p *Provider) {
		p.eventHandler = handler
	}
}

// NewProvider creates a new ScrapingDog provider
func NewProvider(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:          apiKey,
		baseURL:         defaultBaseURL,
		client:          &http.Client{Timeout: 60 * time.Second},
		logger:          slog.Default(),
		searchSemaphore: make(chan struct{}, 5),       // Limit to 5 concurrent searches
		searchQueue:     make(chan QueuedSearch, 100), // Buffer size of 100 searches
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

func (p *Provider) SetEventHandler(handler search.JobEventHandler) {
	p.eventHandler = handler
}

// QueueSearch adds a search to the queue
func (p *Provider) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {
	// If the shutdown context is cancelled, update the atomic shutdown flag
	if p.shutdownCtx != nil && p.shutdownCtx.Err() != nil {
		atomic.StoreInt32(&p.shutdown, 1)
	}
	if atomic.LoadInt32(&p.shutdown) == 1 {
		return fmt.Errorf("provider shutdown")
	}

	select {
	case p.searchQueue <- QueuedSearch{UserID: userID, Criteria: criteria}:
		if err := p.updateUserSearchQueued(ctx, userID); err != nil {
			return fmt.Errorf("failed to update user search queued: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("search queue is full")
	}
}

func (p *Provider) GetQueueDepth(ctx context.Context) (int, error) {
	return len(p.searchQueue), nil
}

// updateUserSearchCompletion updates the user's search completion status
func (p *Provider) updateUserSearchCompletion(ctx context.Context, userID string) error {
	user, found, err := internal.GetUser(ctx, p.fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found")
	}

	now := time.Now()
	user.Search.QueuedAt = nil
	user.Search.CompletedAt = &now

	if err := internal.UpdateUserSearch(ctx, p.fs, userID, user.Search); err != nil {
		return fmt.Errorf("failed to update user search completion: %w", err)
	}

	return nil
}

func (p *Provider) updateUserSearchQueued(ctx context.Context, userID string) error {
	user, found, err := internal.GetUser(ctx, p.fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found")
	}

	now := time.Now()
	user.Search.QueuedAt = &now

	if err := internal.UpdateUserSearch(ctx, p.fs, userID, user.Search); err != nil {
		return fmt.Errorf("failed to update user search queued: %w", err)
	}

	return nil
}

func convertJobListings(jobListings []jobListing) ([]search.JobListing, error) {
	const provider = "scrapingdog"
	providerJobs := make([]search.JobListing, len(jobListings))
	for i, listing := range jobListings {
		// If PostingDate is empty, assign a default date
		if listing.PostingDate == "" {
			listing.PostingDate = "2000-01-01"
		}
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

func handleAPIError(statusCode int) error {
	switch statusCode {
	case http.StatusOK:
		return nil
	case 410:
		return &ErrRequestTimeout{fmt.Errorf("request timed out after 60 seconds")}
	case http.StatusNotFound:
		return &ErrInvalidURL{fmt.Errorf("invalid URL or resource not found")}
	case 202:
		return &ErrRequestPending{fmt.Errorf("request accepted, scraping in progress")}
	case http.StatusForbidden:
		return &ErrPlanLimitReached{fmt.Errorf("monthly request limit reached on current plan")}
	case http.StatusTooManyRequests:
		return &ErrConcurrentLimit{fmt.Errorf("concurrent connection limit reached, please slow down")}
	case http.StatusUnauthorized:
		return &ErrInvalidAPIKey{fmt.Errorf("invalid API key")}
	case http.StatusBadRequest:
		return &ErrRequestFailed{fmt.Errorf("request failed, please check parameters")}
	default:
		return fmt.Errorf("unexpected status code: %d", statusCode)
	}
}

func (p *Provider) doRequestWithRetry(_ context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Handle specific status codes that warrant a retry
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 410 {
			resp.Body.Close()
			p.logger.Debug("rate limited or timeout, retrying",
				"attempt", attempt+1,
				"status", resp.StatusCode)

			// For rate limits, wait longer
			if resp.StatusCode == http.StatusTooManyRequests {
				time.Sleep(retryDelay * time.Duration(attempt+1))
			} else {
				time.Sleep(retryDelay)
			}
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("failed after %d retries", maxRetries)
}

func (p *Provider) StartSearchWorkers(ctx context.Context, numWorkers int) {
	p.logger.Info("starting search workers", "num_workers", numWorkers)
	p.shutdownCtx = ctx // store shutdown context
	// Spawn goroutine to set shutdown flag atomically when context is canceled
	go func() {
		<-ctx.Done()
		atomic.StoreInt32(&p.shutdown, 1)
	}()
	for i := range numWorkers {
		workerID := i
		p.initializeWorker(ctx, workerID)
	}
}

// processJobListings first checks the cache for job listings
// If no cached job listings are found, it performs a new search
// It then returns the job listings
func (p *Provider) processJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	var listings []search.JobListing
	var err error

	// First check cache
	if p.store != nil {
		p.logger.Debug("scrapingdog: checking cache for jobs")
		listings, err = p.store.GetCachedJobs(ctx, criteria)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, context.Canceled
			}
			return nil, fmt.Errorf("failed to get cached jobs: %w", err)
		}
		if len(listings) > 0 {
			p.logger.Debug("scrapingdog: found cached jobs", "num_jobs", len(listings))
			return listings, nil
		}
		p.logger.Debug("scrapingdog: no cached jobs found")
	}

	// If no cached job listings, perform the search
	if len(listings) == 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		p.logger.Debug("scrapingdog: performing job search", "keywords", criteria.Keywords)
		listings, err = p.SearchJobListings(ctx, criteria)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, context.Canceled
			}
			return nil, fmt.Errorf("failed to search jobs: %w", err)
		}
		p.logger.Debug("scrapingdog: job search completed", "num_jobs", len(listings))
	}

	return listings, nil
}
