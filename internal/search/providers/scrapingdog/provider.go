package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
)

// Provider implements the search.JobProvider and search.JobQueue interfaces
type Provider struct {
	apiClient     *APIClient
	store         JobStore
	workerManager *WorkerManager
	eventHandler  search.JobEventHandler
	logger        *slog.Logger
}

// ProviderOption configures the Provider
type ProviderOption func(*Provider)

// WithBaseURL sets a custom base URL for the provider
func WithBaseURL(url string) ProviderOption {
	return func(p *Provider) {
		// Update baseURL on existing APIClient without recreating
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

// WithFirestore sets a Firestore client for the provider
func WithFirestore(fs *internal.FirestoreClient) ProviderOption {
	return func(p *Provider) {
		// Initialize store with Firestore
		p.store = NewFirestoreStore(fs, WithStoreLogger(p.logger))
	}
}

// WithJobEventHandler sets the job event handler for the provider
func WithJobEventHandler(handler search.JobEventHandler) ProviderOption {
	return func(p *Provider) {
		p.eventHandler = handler
	}
}

// WithProviderLogger sets a custom logger for the provider
func WithProviderLogger(logger *slog.Logger) ProviderOption {
	return func(p *Provider) {
		p.logger = logger

		// Update loggers for components if they exist
		if p.apiClient != nil {
			p.apiClient.logger = logger
		}

		if store, ok := p.store.(*FirestoreStore); ok {
			store.logger = logger
		}

		if p.workerManager != nil {
			p.workerManager.logger = logger
		}
	}
}

// WithWorkerCount sets the number of worker goroutines
func WithWorkerCount(count int) ProviderOption {
	return func(p *Provider) {
		// This will be used when starting workers
	}
}

// NewProvider creates a new ScrapingDog provider
func NewProvider(apiKey string, opts ...ProviderOption) *Provider {
	logger := slog.Default()

	// Create provider with default configuration
	provider := &Provider{
		logger: logger,
	}

	// Create API client with default configuration
	provider.apiClient = NewAPIClient(apiKey, WithAPILogger(logger))

	// Apply options
	for _, opt := range opts {
		opt(provider)
	}

	// Create worker manager if not already set
	if provider.workerManager == nil {
		provider.workerManager = NewWorkerManager(
			provider,
			DefaultConcurrency,
			DefaultQueueSize,
			WithWorkerLogger(logger),
			WithJobStore(provider.store),
			WithEventHandler(provider.eventHandler),
		)
	}

	return provider
}

func (p *Provider) SetEventHandler(handler search.JobEventHandler) {
	p.eventHandler = handler
	if p.workerManager != nil {
		p.workerManager.eventHandler = handler
	}
}

// Name returns the provider's name
func (p *Provider) Name() string {
	return ProviderName
}

// Validate checks if the provider is properly configured
func (p *Provider) Validate(ctx context.Context) error {
	p.logger.InfoContext(ctx, "Validating ScrapingDog provider")

	// Try a test search with minimal parameters
	criteria := search.Criteria{
		Query:    "software engineer",
		Location: "remote",
	}

	_, err := p.SearchJobListings(ctx, criteria)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	p.logger.InfoContext(ctx, "ScrapingDog provider validation successful")
	return nil
}

// StartWorkers starts background worker goroutines
func (p *Provider) StartWorkers(ctx context.Context, numWorkers int) {
	p.workerManager.Start(ctx, numWorkers)
}

// StopWorkers gracefully stops the background workers
func (p *Provider) StopWorkers(ctx context.Context) error {
	return p.workerManager.Shutdown(ctx)
}

// SearchJobListings searches for jobs matching the given criteria
func (p *Provider) SearchJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	p.logger.InfoContext(ctx, "Searching for jobs",
		"keywords", criteria.Query,
		"location", criteria.Location)

	// Check if we have cached results
	if p.store != nil {
		cachedJobs, err := p.store.GetCachedJobs(ctx, criteria)
		if err != nil {
			p.logger.WarnContext(ctx, "Failed to check job cache", "error", err)
			// Continue with live search despite cache error
		} else if len(cachedJobs) > 0 {
			p.logger.InfoContext(ctx, "Using cached job results", "count", len(cachedJobs))
			return cachedJobs, nil
		}
	}

	// Perform live search
	p.logger.InfoContext(ctx, "Performing live search")

	// Set default page number if not specified
	page := criteria.MaxPages
	if page <= 0 {
		page = 1
	}

	// Call the API
	jobListings, err := p.apiClient.SearchJobListings(ctx, criteria.Query, criteria.Location, page)
	if err != nil {
		return nil, fmt.Errorf("API search failed: %w", err)
	}

	// Convert to common format
	jobs, err := ConvertJobListings(jobListings)
	if err != nil {
		return nil, fmt.Errorf("failed to convert job listings: %w", err)
	}

	// Cache the results
	if p.store != nil && len(jobs) > 0 {
		if err := p.store.SaveJobs(ctx, criteria, jobs); err != nil {
			p.logger.WarnContext(ctx, "Failed to cache job results", "error", err)
			// Continue despite cache error
		}
	}

	p.logger.InfoContext(ctx, "Search completed", "count", len(jobs))
	return jobs, nil
}

// GetJobDetails retrieves detailed information for a job
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (search.JobDetail, error) {
	p.logger.InfoContext(ctx, "Getting job details", "jobID", jobID)

	// Check if we have cached results
	if p.store != nil {
		cachedDetails, err := p.store.GetJobDetails(ctx, jobID)
		if err != nil {
			p.logger.WarnContext(ctx, "Failed to check job details cache", "error", err)
			// Continue with live fetch despite cache error
		} else if cachedDetails != nil {
			p.logger.InfoContext(ctx, "Using cached job details", "jobID", jobID)
			return *cachedDetails, nil
		}
	}

	// Perform live fetch
	p.logger.InfoContext(ctx, "Fetching job details from API", "jobID", jobID)

	// Call the API
	jobDetail, err := p.apiClient.GetJobDetails(ctx, jobID)
	if err != nil {
		return search.JobDetail{}, fmt.Errorf("API job details fetch failed: %w", err)
	}

	// Convert to common format
	details, err := ConvertJobDetail(jobDetail)
	if err != nil {
		return search.JobDetail{}, fmt.Errorf("failed to convert job details: %w", err)
	}

	// Cache the results
	if p.store != nil {
		if err := p.store.SaveJobDetails(ctx, details); err != nil {
			p.logger.WarnContext(ctx, "Failed to cache job details", "error", err)
			// Continue despite cache error
		}
	}

	p.logger.InfoContext(ctx, "Job details fetched successfully", "jobID", jobID)
	return details, nil
}

// QueueSearch adds a search to the background processing queue
func (p *Provider) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {
	p.logger.InfoContext(ctx, "Queueing search",
		"userID", userID,
		"keywords", criteria.Query)

	return p.workerManager.QueueSearch(ctx, userID, criteria)
}

// GetQueueDepth returns the current number of items in the queue
func (p *Provider) GetQueueDepth(ctx context.Context) (int, error) {
	depth := p.workerManager.GetQueueDepth()
	return depth, nil
}
