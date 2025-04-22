package scrapingdog

import (
	"errors"
	"fmt"
)

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

// NewRequestTimeoutError creates a new request timeout error
func NewRequestTimeoutError(msg string) *ErrRequestTimeout {
	return &ErrRequestTimeout{errors.New(msg)}
}

// NewInvalidURLError creates a new invalid URL error
func NewInvalidURLError(msg string) *ErrInvalidURL {
	return &ErrInvalidURL{errors.New(msg)}
}

// NewRequestPendingError creates a new request pending error
func NewRequestPendingError(msg string) *ErrRequestPending {
	return &ErrRequestPending{errors.New(msg)}
}

// NewPlanLimitReachedError creates a new plan limit reached error
func NewPlanLimitReachedError(msg string) *ErrPlanLimitReached {
	return &ErrPlanLimitReached{errors.New(msg)}
}

// NewConcurrentLimitError creates a new concurrent limit error
func NewConcurrentLimitError(msg string) *ErrConcurrentLimit {
	return &ErrConcurrentLimit{errors.New(msg)}
}

// NewInvalidAPIKeyError creates a new invalid API key error
func NewInvalidAPIKeyError(msg string) *ErrInvalidAPIKey {
	return &ErrInvalidAPIKey{errors.New(msg)}
}

// NewRequestFailedError creates a new request failed error
func NewRequestFailedError(msg string) *ErrRequestFailed {
	return &ErrRequestFailed{errors.New(msg)}
}

// IsRetryableError determines if an error can be retried
func IsRetryableError(err error) bool {
	switch err.(type) {
	case *ErrRequestTimeout, *ErrConcurrentLimit, *ErrRequestPending:
		return true
	default:
		return false
	}
}

// HandleAPIError converts an HTTP status code to a specific error type
func HandleAPIError(statusCode int) error {
	switch statusCode {
	case 200:
		return nil // Successful request
	case 410:
		return NewRequestTimeoutError("request timed out")
	case 404:
		return NewInvalidURLError("invalid URL")
	case 202:
		return NewRequestPendingError("request pending")
	case 403:
		return NewPlanLimitReachedError("plan limit reached")
	case 429:
		return NewConcurrentLimitError("concurrent connection limit reached")
	case 401:
		return NewInvalidAPIKeyError("invalid API key")
	case 400:
		return NewRequestFailedError("request failed")
	default:
		return fmt.Errorf("unknown API error with status code: %d", statusCode)
	}
}
