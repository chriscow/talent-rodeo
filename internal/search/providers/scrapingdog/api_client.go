package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"time"
)

type SortBy string

const (
	SortByDay   SortBy = "day"   // Last 24 hours
	SortByWeek  SortBy = "week"  // Last 7 days
	SortByMonth SortBy = "month" // Last 30 days

	DefaultSortBy SortBy = SortByDay // Default sort order
)

// APIClientInterface defines the interface for the API client
type APIClientInterface interface {
	SearchJobListings(ctx context.Context, query string, location string, page int) ([]LinkedInJobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (*LinkedInJobDetail, error)
}

// APIClient implements the APIClientInterface
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

	client.logger = client.logger.With("pkg", "scrapingdog.APIClient")

	// Trim trailing slash from baseURL to avoid issues with endpoint path construction
	client.baseURL = strings.TrimSuffix(client.baseURL, "/")

	return client
}

// SearchJobListings searches for jobs with the given criteria
func (c *APIClient) SearchJobListings(ctx context.Context, query, location string, jobType internal.WorkType, expLevel internal.ExperienceLevel, page int, sortBy SortBy) ([]LinkedInJobListing, error) {
	c.logger.Info("searching for job listings",
		"query", query,
		"location", location,
		"page", page)

	// Build URL
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Add query parameters
	q := baseURL.Query()
	q.Set("api_key", c.apiKey)
	q.Set("field", query)
	q.Set("geoid", location)
	q.Set("sort_by", string(sortBy)) // day (last 24 hours), week (last 7 days), month (last 30 days)

	q.Set("job_type", string(jobType))   // optional, can be empty
	q.Set("exp_level", string(expLevel)) // optional, can be empty

	q.Set("page", strconv.Itoa(page))
	baseURL.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use retry mechanism
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors (this will handle specific error types)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := HandleAPIError(resp.StatusCode)
		return nil, fmt.Errorf("API request failed with status %d: %s: %w", resp.StatusCode, string(body), apiErr)
	}

	// Parse response
	var jobs []LinkedInJobListing
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobs, nil
}

// GetJobDetails retrieves detailed information for a specific job
func (c *APIClient) GetJobDetails(ctx context.Context, jobID string) (LinkedInJobDetail, error) {
	c.logger.Debug("getting job details from scrapingdog", "jobID", jobID)

	// Construct URL with jobid parameter
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return LinkedInJobDetail{}, fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Update the path to include the jobid endpoint
	baseURL.Path = "/linkedinjobs"

	params := url.Values{}
	params.Add("api_key", c.apiKey)
	params.Add("job_id", jobID)

	baseURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return LinkedInJobDetail{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request with retry
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		return LinkedInJobDetail{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		return LinkedInJobDetail{}, HandleAPIError(resp.StatusCode)
	}

	// Check content type for debugging
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		c.logger.Warn("unexpected content type in response",
			"content-type", contentType,
			"jobID", jobID)
	}

	// Read the entire body for potential error debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LinkedInJobDetail{}, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTML content that would indicate an error page
	if len(body) > 0 && body[0] == '<' {
		// Log the first 200 chars of the response for diagnosis
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		c.logger.Error("received HTML instead of JSON",
			"jobID", jobID,
			"preview", preview)
		return LinkedInJobDetail{}, fmt.Errorf("received HTML response instead of JSON")
	}

	// Parse the JSON response
	items := make([]LinkedInJobDetail, 0)
	if err := json.Unmarshal(body, &items); err != nil {
		return LinkedInJobDetail{}, fmt.Errorf("failed to decode response: %w", err)
	}

	jobDetail := items[0]

	// Set the ID from the input parameter
	jobDetail.ID = jobID

	c.logger.Debug("successfully fetched job details",
		"jobID", jobID,
		"title", jobDetail.Position)

	return jobDetail, nil
}

// doRequestWithRetry executes an HTTP request with retry logic and exponential backoff
func (c *APIClient) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	var backoffDelay time.Duration = RetryDelay
	var statusCode int

	for i := 0; i < MaxRetries; i++ {
		// Log retry attempt
		if i > 0 {
			c.logger.Info("retrying request",
				"attempt", i+1,
				"max_retries", MaxRetries,
				"backoff", backoffDelay,
				"url", req.URL.String())
		}

		// Execute the request
		resp, err = c.client.Do(req)

		// If we encountered a network error, apply exponential backoff
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			// Wait before retrying with exponential backoff
			select {
			case <-time.After(backoffDelay):
				// Double the backoff for next retry (exponential)
				backoffDelay *= 2
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		// Check if it's a retryable status code (429, 410, 500+)
		statusCode = resp.StatusCode
		if statusCode == http.StatusTooManyRequests {
			// This is a 429 error - rate limiting
			body, bodyErr := io.ReadAll(resp.Body)
			resp.Body.Close() // Important: close this response before retrying

			c.logger.Warn("rate limit (429) encountered, backing off",
				"attempt", i+1,
				"backoff", backoffDelay,
				"response", string(body))

			// Apply exponential backoff for rate limiting
			select {
			case <-time.After(backoffDelay):
				// Increase backoff duration for next retry
				backoffDelay *= 2
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Log body error if we had trouble reading it
			if bodyErr != nil {
				c.logger.Warn("failed to read error response body", "error", bodyErr)
			}

			continue
		} else if statusCode == http.StatusRequestTimeout || statusCode == http.StatusGone || statusCode >= 500 {
			// This is a 410 (request timeout) or 500+ (server-side) error - retry
			resp.Body.Close() // Important: close this response before retrying

			c.logger.Warn("retryable status code encountered",
				"status", statusCode,
				"attempt", i+1,
				"backoff", backoffDelay)

			// Apply backoff for server errors
			select {
			case <-time.After(backoffDelay):
				// Increase backoff duration for next retry, but less aggressively
				backoffDelay = backoffDelay + (RetryDelay / 2)
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			continue
		}

		// For all other status codes (success or non-retryable errors), return the response
		return resp, nil
	}

	// If we've exhausted all retries, return the last error
	if err != nil {
		return nil, fmt.Errorf("exhausted all retries (%d): %w", MaxRetries, err)
	}

	// If we reach here, we've exhausted retries due to status codes
	return nil, fmt.Errorf("exhausted all retries (%d): last status code: %d", MaxRetries, statusCode)
}
