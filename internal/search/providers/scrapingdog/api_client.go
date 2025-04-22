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
	"time"
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

	// Trim trailing slash from baseURL to avoid issues with endpoint path construction
	client.baseURL = strings.TrimSuffix(client.baseURL, "/")

	return client
}

// SearchJobListings searches for jobs with the given criteria
func (c *APIClient) SearchJobListings(ctx context.Context, query string, location string, page int) ([]LinkedInJobListing, error) {
	c.logger.InfoContext(ctx, "Searching for jobs",
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
	q.Set("page", strconv.Itoa(page))
	baseURL.RawQuery = q.Encode()

	fmt.Printf("SearchJobListings URL: %s\n", baseURL.String())

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var jobs []LinkedInJobListing
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.InfoContext(ctx, "Search completed",
		"query", query,
		"location", location,
		"page", page,
		"jobs_found", len(jobs))

	return jobs, nil
}

// GetJobDetails retrieves detailed information for a specific job
func (c *APIClient) GetJobDetails(ctx context.Context, jobID string) (LinkedInJobDetail, error) {
	c.logger.InfoContext(ctx, "Getting job details", "jobID", jobID)

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

	// Log constructed URL at info level so it's visible in test output
	c.logger.InfoContext(ctx, "Constructed URL for job details",
		"job_id", jobID,
		"url", baseURL.String())

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
		c.logger.WarnContext(ctx, "Unexpected content type in response",
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
		c.logger.ErrorContext(ctx, "Received HTML instead of JSON",
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
