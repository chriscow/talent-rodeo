package search

import (
	"errors"
)

// Error types for common provider operations
var (
	ErrProviderNotFound    = errors.New("provider not found")
	ErrProviderDisabled    = errors.New("provider is disabled")
	ErrInvalidCredentials  = errors.New("invalid provider credentials")
	ErrRateLimitExceeded   = errors.New("provider rate limit exceeded")
	ErrProviderUnavailable = errors.New("provider is temporarily unavailable")
)
