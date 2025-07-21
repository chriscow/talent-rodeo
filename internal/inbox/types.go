package inbox

import (
	"context"
	"time"
)

// HandlingResult represents the outcome of email processing by a handler.
// It provides information about whether the email was handled and what actions were taken.
type HandlingResult struct {
	// Handled indicates whether this handler processed the email
	Handled bool
	// Summary provides a description of actions taken (empty if not handled)
	Summary string
	// Metadata contains additional context for response generation
	Metadata map[string]interface{}
}

// Email represents an email message from the inbox
type Email struct {
	ID          string
	ThreadID    string
	From        string
	To          []string
	Subject     string
	Body        string
	HTML        string
	Date        time.Time
	Labels      []string
	IsRead      bool
	Headers     map[string]string
	Attachments []Attachment
}

// Attachment represents an email attachment
type Attachment struct {
	ID       string
	Filename string
	MimeType string
	Size     int64
	Data     []byte
}

// EmailHandler defines the interface for processing emails and returning handling results.
// Implementations should return a HandlingResult indicating whether they processed
// the email and what actions were taken.
type EmailHandler interface {
	HandleEmail(ctx context.Context, email *Email) (*HandlingResult, error)
}

// EmailHandlerFunc is a function type that implements EmailHandler
type EmailHandlerFunc func(ctx context.Context, email *Email) (*HandlingResult, error)

func (f EmailHandlerFunc) HandleEmail(ctx context.Context, email *Email) (*HandlingResult, error) {
	return f(ctx, email)
}
