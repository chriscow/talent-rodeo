package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"talent-rodeo/internal/search"

	"github.com/getsentry/sentry-go"
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
	WorkerSettingsKey = "scrapingdog"
)

// Provider implements the search.JobProvider and search.JobQueue interfaces
type Provider struct {
	apiClient     *APIClient
	cache         search.JobCache
	workerManager *WorkerManager
	eventHandler  internal.JobEventHandler
	logger        *slog.Logger
	apiKey        string
	kvClient      *cloudflare.KV
}

// ProviderOption configures the Provider
type ProviderOption func(*Provider)

// WithBaseURL sets a custom base URL for the provider
func WithBaseURL(url string) ProviderOption {
	return func(p *Provider) {
		if p.apiClient == nil {
			p.apiClient = NewAPIClient(p.apiKey, WithAPIBaseURL(url))
			return
		}
		p.apiClient.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client for the provider
func WithHTTPClient(client *http.Client) ProviderOption {
	return func(p *Provider) {
		// Update HTTP client on existing APIClient without recreating
		p.apiClient.client = client
	}
}

func WithCache(cache search.JobCache) ProviderOption {
	return func(p *Provider) {
		p.cache = cache
	}
}

// WithJobEventHandler sets the job event handler for the provider
func WithJobEventHandler(handler internal.JobEventHandler) ProviderOption {
	return func(p *Provider) {
		p.eventHandler = handler
	}
}

// WithProviderAPIConcurrency sets the maximum number of concurrent API calls
func WithProviderAPIConcurrency(maxConcurrent int) ProviderOption {
	return func(p *Provider) {
		// Re-create the API client with the new concurrency setting
		if p.apiClient != nil {
			p.apiClient = NewAPIClient(p.apiKey,
				WithAPILogger(p.logger),
				WithAPIConcurrency(maxConcurrent))
		}
	}
}

// NewProvider creates a new ScrapingDog provider
func NewProvider(apiKey string, logger *slog.Logger, opts ...ProviderOption) *Provider {
	logger = logger.With("pkg", "scrapingdog.Provider")

	// Create provider with required configuration
	provider := &Provider{
		logger: logger,
		apiKey: apiKey,
	}

	// Create API client with the logger
	provider.apiClient = NewAPIClient(apiKey, WithAPILogger(logger))

	// Apply all options
	for _, opt := range opts {
		opt(provider)
	}

	kv, err := cloudflare.NewKV()
	if err != nil {
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "NewProvider: failed to create KV client",
		})
		logger.Error("failed to create KV client", "error", err)
		panic(fmt.Sprintf("failed to create KV client: %v", err))
	}

	// Set KV client and settings key
	provider.kvClient = kv

	// Create worker manager if not already set
	if provider.workerManager == nil {
		// Use a larger semaphore for search tasks since API concurrency is controlled at the client level
		provider.workerManager = NewWorkerManager(
			provider,
			DefaultConcurrency*4, // Allow more search tasks since API calls are rate-limited
			DefaultQueueSize,
			withWorkerLogger(logger),
			withEventHandler(provider.eventHandler),
		)
	}

	return provider
}

func (p *Provider) SetEventHandler(handler internal.JobEventHandler) {
	p.eventHandler = handler
	if p.workerManager != nil {
		p.workerManager.eventHandler = handler
	}
}

// Name returns the provider's name
func (p *Provider) Name() string {
	return SearchProviderName
}

// Validate checks if the provider is properly configured
func (p *Provider) Validate(ctx context.Context) error {
	p.logger.Info("validating ScrapingDog provider")

	// Try a test search with minimal parameters
	criteria := internal.SearchCriteria{
		Query: "software engineer",
		GeoID: "remote",
	}

	_, err := p.SearchJobListings(ctx, criteria, 1)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	p.logger.Info("ScrapingDog provider validation successful")
	return nil
}

// StartWorkers starts background worker goroutines
func (p *Provider) Start(ctx context.Context) error {
	settings, err := p.getWorkerSettings(ctx)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "NewProvider::Start: failed to get worker settings",
		})
		return fmt.Errorf("failed to get worker settings: %w", err)
	}

	currentPaused := p.workerManager.IsPaused()
	if currentPaused != settings.Paused {
		p.workerManager.Pause(settings.Paused)
	}

	// Start the settings updater
	go p.startSettingsUpdater(ctx)

	return p.workerManager.Start(ctx, DefaultConcurrency*4) // More workers since API calls are rate-limited
}

// Close gracefully stops the background workers
func (p *Provider) Close(ctx context.Context) error {
	return p.workerManager.Shutdown(ctx)
}

// SearchJobListings searches for jobs matching the given criteria
func (p *Provider) SearchJobListings(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error) {
	p.logger.Debug("searching for job listings",
		"keywords", criteria.Query,
		"location", criteria.GeoID)

	// Check if we have cached results
	if p.cache != nil {
		cachedJobs, found := p.cache.GetJobListings(ctx, criteria.GetSearchHash())
		if found {
			p.logger.Debug("using cached job results", "count", len(cachedJobs))
			return cachedJobs, nil
		}
	}

	if string(criteria.ListingAge) == "" {
		criteria.ListingAge = internal.DefaultListingAge
	}

	// Call the API
	jobListings, err := p.apiClient.SearchJobListings(ctx, criteria.Query, criteria.GeoID, criteria.WorkType, criteria.ExperienceLevel, page, SortBy(criteria.ListingAge))
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "NewProvider::SearchJobListings: API search failed",
		})
		return nil, fmt.Errorf("API search failed: %w", err)
	}

	// Convert to common format
	jobs, err := convertJobListings(JobProviderName, jobListings, &criteria)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "NewProvider::SearchJobListings: failed to convert job listings",
		})
		return nil, fmt.Errorf("failed to convert job listings: %w", err)
	}

	p.logger.Info("search job listings completed", "count", len(jobs))
	return jobs, nil
}

// GetJobDetails retrieves detailed information for a job
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (*internal.JobDetail, error) {
	p.logger.Debug("fetching job details", "jobID", jobID)

	// Check if we have cached results
	if p.cache != nil {
		cachedDetails, found := p.cache.GetJobDetail(ctx, jobID)
		if found {
			p.logger.Debug("using cached job details", "jobID", jobID)
			return cachedDetails, nil
		}
	}

	jobDetail, err := p.apiClient.GetJobDetails(ctx, jobID)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "NewProvider::GetJobDetails: API job details fetch failed",
		})
		return nil, fmt.Errorf("API job details fetch failed: %w", err)
	}

	// Convert to common format
	details, err := convertJobDetail(jobDetail)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "NewProvider::GetJobDetails: failed to convert job details",
		})
		return nil, fmt.Errorf("failed to convert job details: %w", err)
	}

	return details, nil
}

// QueueSearch adds a search to the background processing queue
func (p *Provider) QueueSearch(ctx context.Context, searchJob internal.SearchTask) error {
	p.logger.Info("queueing search",
		"query", searchJob.Criteria.Query,
		"geoID", searchJob.Criteria.GeoID,
		"maxPages", searchJob.Criteria.MaxPages)

	return p.workerManager.QueueTask(ctx, WorkerTask{Type: TaskTypeSearch, SearchTask: &searchJob})
}

// GetQueueDepth returns the current number of items in the queue
func (p *Provider) GetQueueDepth(ctx context.Context) (int, error) {
	depth := p.workerManager.GetQueueDepth()
	return depth, nil
}

// getWorkerSettings retrieves worker settings from CloudFlare KV
func (p *Provider) getWorkerSettings(_ context.Context) (*WorkerSettings, error) {
	// If no KV client configured, return default settings (not paused)
	if p.kvClient == nil {
		return &WorkerSettings{Paused: false}, nil
	}

	// Read the scrapingdog flag from KV - when disabled, workers should be paused
	var settings WorkerSettings
	if err := p.kvClient.Get("scrapingdog", &settings); err != nil {
		if err == cloudflare.ErrNotFound {
			// Return default settings if key doesn't exist (enabled by default)
			return &WorkerSettings{Paused: false}, nil
		}
		return &WorkerSettings{Paused: false}, err // Default to not paused on error
	}

	// If scrapingdog is disabled in feature flags, pause the workers
	return &settings, nil
}

// startSettingsUpdater runs a background goroutine to check settings every 30 seconds
func (p *Provider) startSettingsUpdater(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	p.logger.Info("starting settings updater")

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("shutting down settings updater")
			return
		case <-ticker.C:
			settings, err := p.getWorkerSettings(ctx)
			if err != nil {
				p.logger.Error("failed to get worker settings", "error", err)
				continue
			}

			currentPaused := p.workerManager.IsPaused()
			if currentPaused != settings.Paused {
				p.logger.Info("updating worker pause state", "paused", settings.Paused)
				p.workerManager.Pause(settings.Paused)
			}
		}
	}
}
