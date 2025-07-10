package internal

import "errors"

// Common error definitions
var (
	ErrUserNotFound           = errors.New("user not found")
	ErrInvalidAppointmentType = errors.New("invalid appointment type")
	ErrNoMessagesFound        = errors.New("no messages found")
	ErrAppointmentNotFound    = errors.New("appointment not found")
	ErrInvalidCSRFToken       = errors.New("invalid CSRF token")
	ErrUnauthorized           = errors.New("unauthorized")
	ErrSessionExpired         = errors.New("session expired")
)
