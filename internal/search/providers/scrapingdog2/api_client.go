package scrapingdog2

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIClient handles all direct interactions with the ScrapingDog API
type APIClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
	logger  *slog.Logger
}

// APIOption is a function type for configuring the APIClient
type APIOption func(*APIClient)

// WithAPIBaseURL sets a custom base URL for the API client
func WithAPIBaseURL(baseURL string) APIOption {
	return func(c *APIClient) {
		c.baseURL = baseURL
	}
}

// WithAPILogger sets a custom logger for the API client
func WithAPILogger(logger *slog.Logger) APIOption {
	return func(c *APIClient) {
		c.logger = logger
	}
}

// WithAPIHTTPClient sets a custom HTTP client for the API client
func WithAPIHTTPClient(client *http.Client) APIOption {
	return func(c *APIClient) {
		c.client = client
	}
}

// NewAPIClient creates a new ScrapingDog API client
func NewAPIClient(apiKey string, opts ...APIOption) *APIClient {
	client := &APIClient{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// SearchJobs searches for jobs with the given criteria
func (c *APIClient) SearchJobs(ctx context.Context, keywords []string, location string, page int) ([]JobListing, error) {
	c.logger.InfoContext(ctx, "Searching for jobs",
		"keywords", keywords,
		"location", location,
		"page", page)

	// Construct URL with querystring parameters
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	params := url.Values{}
	params.Add("api_key", c.apiKey)

	// Add keywords as space-separated string
	if len(keywords) > 0 {
		params.Add("q", keywords[0]) // ScrapingDog only supports one keyword
	}

	// Set location parameter if provided
	if location != "" {
		params.Add("location", location)
	}

	// Set page number if greater than 0
	if page > 0 {
		params.Add("page", strconv.Itoa(page))
	}

	baseURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request with retry
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		return nil, HandleAPIError(resp.StatusCode)
	}

	// Parse response
	var jobListings []JobListing
	if err := json.NewDecoder(resp.Body).Decode(&jobListings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.InfoContext(ctx, "Successfully fetched jobs",
		"count", len(jobListings))

	return jobListings, nil
}

// GetJobDetails retrieves detailed information for a specific job
func (c *APIClient) GetJobDetails(ctx context.Context, jobID string) (JobDetail, error) {
	c.logger.InfoContext(ctx, "Getting job details", "jobID", jobID)

	var jobDetail JobDetail

	// Construct URL with jobid parameter
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return JobDetail{}, fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Update the path to include the jobid endpoint
	baseURL.Path = fmt.Sprintf("%s/%s", baseURL.Path, "jobid")

	params := url.Values{}
	params.Add("api_key", c.apiKey)
	params.Add("jobid", jobID)

	baseURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return JobDetail{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request with retry
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		return JobDetail{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		return JobDetail{}, HandleAPIError(resp.StatusCode)
	}

	// Parse response
	if err := json.NewDecoder(resp.Body).Decode(&jobDetail); err != nil {
		return JobDetail{}, fmt.Errorf("failed to decode response: %w", err)
	}

	// Set the ID from the input parameter
	jobDetail.ID = jobID

	c.logger.InfoContext(ctx, "Successfully fetched job details",
		"jobID", jobID,
		"title", jobDetail.Position)

	return jobDetail, nil
}

// doRequestWithRetry executes an HTTP request with retry logic
func (c *APIClient) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := 0; i < MaxRetries; i++ {
		// Log retry attempt
		if i > 0 {
			c.logger.InfoContext(ctx, "Retrying request",
				"attempt", i+1,
				"max_retries", MaxRetries,
				"url", req.URL.String())
		}

		// Execute the request
		resp, err = c.client.Do(req)

		// If no error, return the response
		if err == nil {
			return resp, nil
		}

		// If context is canceled, return immediately
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Wait before retrying
		select {
		case <-time.After(RetryDelay):
			// Continue with retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// If we've exhausted all retries, return the last error
	return nil, fmt.Errorf("exhausted all retries (%d): %w", MaxRetries, err)
}
