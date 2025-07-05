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

	"github.com/getsentry/sentry-go"
)

type SortBy string

const (
	ScrapingDogConcurrency = 5

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
	apiKey       string
	baseURL      string
	client       *http.Client
	logger       *slog.Logger
	apiSemaphore chan struct{} // Controls concurrent API calls
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

// WithAPIConcurrency sets the maximum number of concurrent API calls
func WithAPIConcurrency(maxConcurrent int) APIOption {
	return func(c *APIClient) {
		c.apiSemaphore = make(chan struct{}, maxConcurrent)
	}
}

// NewAPIClient creates a new ScrapingDog API client
func NewAPIClient(apiKey string, opts ...APIOption) *APIClient {
	client := &APIClient{
		apiKey:       apiKey,
		baseURL:      DefaultBaseURL,
		client:       &http.Client{Timeout: 120 * time.Second},
		logger:       slog.Default(),
		apiSemaphore: make(chan struct{}, ScrapingDogConcurrency), // Default to 5
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
func (c *APIClient) SearchJobListings(ctx context.Context, query, geoid string, jobType internal.WorkType, expLevel internal.ExperienceLevel, page int, sortBy SortBy) ([]LinkedInJobListing, error) {
	// Acquire semaphore to limit concurrent API calls
	select {
	case c.apiSemaphore <- struct{}{}:
		c.logger.Debug("acquired API semaphore", "inUse", len(c.apiSemaphore), "capacity", cap(c.apiSemaphore))
		defer func() { <-c.apiSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	c.logger.Info("searching for job listings",
		"query", query,
		"location", geoid,
		"page", page)

	// Build URL
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::SearchJobListings: failed to parse base URL",
		})
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Add query parameters
	q := baseURL.Query()
	q.Set("api_key", c.apiKey)
	q.Set("field", query)
	q.Set("geoid", geoid)
	q.Set("sort_by", string(sortBy)) // day (last 24 hours), week (last 7 days), month (last 30 days)

	q.Set("job_type", string(jobType))   // optional, can be empty
	q.Set("exp_level", string(expLevel)) // optional, can be empty

	q.Set("page", strconv.Itoa(page))
	baseURL.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::SearchJobListings: failed to create request",
		})
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use retry mechanism
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::SearchJobListings: failed to send request",
		})
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors (this will handle specific error types)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := HandleAPIError(resp.StatusCode)
		internal.SentryReportError(ctx, apiErr, sentry.LevelError, map[string]string{
			"action":     "APIClient::SearchJobListings: API request failed",
			"statusCode": fmt.Sprintf("%d", resp.StatusCode),
			"response":   string(body),
		})
		return nil, fmt.Errorf("API request failed with status %d: %s: %w", resp.StatusCode, string(body), apiErr)
	}

	// Parse response
	var jobs []LinkedInJobListing
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::SearchJobListings: failed to decode response",
		})
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobs, nil
}

// GetJobDetails retrieves detailed information for a specific job
func (c *APIClient) GetJobDetails(ctx context.Context, jobID string) (LinkedInJobDetail, error) {
	// Acquire semaphore to limit concurrent API calls
	select {
	case c.apiSemaphore <- struct{}{}:
		c.logger.Debug("acquired API semaphore", "inUse", len(c.apiSemaphore), "capacity", cap(c.apiSemaphore))
		defer func() { <-c.apiSemaphore }()
	case <-ctx.Done():
		return LinkedInJobDetail{}, ctx.Err()
	}

	c.logger.Debug("getting job details from scrapingdog", "jobID", jobID)

	// Construct URL with jobid parameter
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::GetJobDetails: failed to parse base URL",
		})
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
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::GetJobDetails: failed to create request",
		})
		return LinkedInJobDetail{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request with retry
	resp, err := c.doRequestWithRetry(ctx, req)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::GetJobDetails: failed to execute request",
			"jobID":  jobID,
		})
		return LinkedInJobDetail{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		apiErr := HandleAPIError(resp.StatusCode)
		internal.SentryReportError(ctx, apiErr, sentry.LevelError, map[string]string{
			"action":     "APIClient::GetJobDetails: API request failed",
			"statusCode": fmt.Sprintf("%d", resp.StatusCode),
			"jobID":      jobID,
		})
		return LinkedInJobDetail{}, apiErr
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
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::GetJobDetails: failed to read response body",
			"jobID":  jobID,
		})
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
		htmlErr := fmt.Errorf("received HTML response instead of JSON")
		internal.SentryReportError(ctx, htmlErr, sentry.LevelError, map[string]string{
			"action":  "APIClient::GetJobDetails: received HTML instead of JSON",
			"jobID":   jobID,
			"preview": preview,
		})
		return LinkedInJobDetail{}, htmlErr
	}

	// Parse the JSON response
	items := make([]LinkedInJobDetail, 0)
	if err := json.Unmarshal(body, &items); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "APIClient::GetJobDetails: failed to decode response",
			"jobID":  jobID,
		})
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

// IsRetryable400Error checks if a 400 error response contains messages indicating a temporary failure
func IsRetryable400Error(body []byte) bool {
	bodyStr := string(body)
	// Check for specific temporary failure messages from ScrapingDog API
	retryableMessages := []string{
		"Something went wrong, please try again",
		"try again",
		"temporary",
		"temporarily unavailable",
	}

	bodyLower := strings.ToLower(bodyStr)
	for _, msg := range retryableMessages {
		if strings.Contains(bodyLower, strings.ToLower(msg)) {
			return true
		}
	}
	return false
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
		} else if statusCode == http.StatusBadRequest {
			// This is a 400 error - check if it's retryable based on the message
			body, bodyErr := io.ReadAll(resp.Body)
			resp.Body.Close() // Important: close this response before retrying

			if bodyErr != nil {
				c.logger.Warn("failed to read 400 error response body", "error", bodyErr)
				// If we can't read the body, treat as non-retryable
				return resp, nil
			}

			if IsRetryable400Error(body) {
				c.logger.Warn("retryable 400 error encountered, backing off",
					"attempt", i+1,
					"backoff", backoffDelay,
					"response", string(body))

				// Apply backoff for retryable 400 errors
				select {
				case <-time.After(backoffDelay):
					// Increase backoff duration for next retry
					backoffDelay = backoffDelay + (RetryDelay / 2)
				case <-ctx.Done():
					return nil, ctx.Err()
				}

				continue
			} else {
				// Not a retryable 400 error, reconstruct response and return
				c.logger.Debug("non-retryable 400 error, not retrying", "response", string(body))
				// Create a new response with the body we read
				resp = &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     resp.Header,
					Body:       io.NopCloser(strings.NewReader(string(body))),
				}
				return resp, nil
			}
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
		retryErr := fmt.Errorf("exhausted all retries (%d): %w", MaxRetries, err)
		internal.SentryReportError(ctx, retryErr, sentry.LevelError, map[string]string{
			"action":     "APIClient::doRequestWithRetry: exhausted all retries with error",
			"maxRetries": fmt.Sprintf("%d", MaxRetries),
			"url":        req.URL.String(),
		})
		return nil, retryErr
	}

	// If we reach here, we've exhausted retries due to status codes
	retryErr := fmt.Errorf("exhausted all retries (%d): last status code: %d", MaxRetries, statusCode)
	internal.SentryReportError(ctx, retryErr, sentry.LevelError, map[string]string{
		"action":     "APIClient::doRequestWithRetry: exhausted all retries with status code",
		"maxRetries": fmt.Sprintf("%d", MaxRetries),
		"statusCode": fmt.Sprintf("%d", statusCode),
		"url":        req.URL.String(),
	})
	return nil, retryErr
}
