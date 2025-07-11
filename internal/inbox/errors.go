package inbox

import "errors"

// Email response validation errors
var (
	// ErrMissingSubject is returned when an email response has no subject
	ErrMissingSubject = errors.New("email response missing subject")
	// ErrMissingBody is returned when an email response has no body content
	ErrMissingBody = errors.New("email response missing body content")
	// ErrInvalidResponseType is returned when an email response has an invalid type
	ErrInvalidResponseType = errors.New("email response has invalid type")
)

// Email processing errors
var (
	// ErrNoHandlers is returned when no handlers are registered
	ErrNoHandlers = errors.New("no handlers registered")
	// ErrInvalidEmail is returned when an email is invalid or malformed
	ErrInvalidEmail = errors.New("invalid email")
	// ErrResponseGenerationFailed is returned when response generation fails
	ErrResponseGenerationFailed = errors.New("response generation failed")
	// ErrEscalationFailed is returned when escalation to admin fails
	ErrEscalationFailed = errors.New("escalation failed")
)