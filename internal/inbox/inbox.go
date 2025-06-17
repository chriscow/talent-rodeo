package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Email represents an email message from the inbox
type Email struct {
	ID          string            `json:"id"`
	ThreadID    string            `json:"thread_id"`
	From        string            `json:"from"`
	To          []string          `json:"to"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	HTML        string            `json:"html"`
	Date        time.Time         `json:"date"`
	Labels      []string          `json:"labels"`
	IsRead      bool              `json:"is_read"`
	Headers     map[string]string `json:"headers"`
	Attachments []Attachment      `json:"attachments"`
}

// Attachment represents an email attachment
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Data     []byte `json:"data,omitempty"`
}

// EmailHandler defines the interface for processing emails
type EmailHandler interface {
	HandleEmail(ctx context.Context, email *Email) error
}

// EmailHandlerFunc is a function type that implements EmailHandler
type EmailHandlerFunc func(ctx context.Context, email *Email) error

func (f EmailHandlerFunc) HandleEmail(ctx context.Context, email *Email) error {
	return f(ctx, email)
}

// InboxReader defines the interface for reading emails from an inbox
type InboxReader interface {
	// GetUnreadEmails retrieves unread emails from the inbox
	GetUnreadEmails(ctx context.Context, maxResults int) ([]*Email, error)
	
	// MarkAsRead marks an email as read
	MarkAsRead(ctx context.Context, emailID string) error
	
	// Close closes the inbox connection
	Close() error
}

// InboxConfig holds configuration for inbox readers
type InboxConfig struct {
	// For Gmail API
	CredentialsJSON []byte `json:"credentials_json,omitempty"`
	TokenPath       string `json:"token_path,omitempty"`
	
	// For IMAP
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	UseTLS   bool   `json:"use_tls,omitempty"`
}

// NewInboxReader creates a new inbox reader based on the configuration
func NewInboxReader(config InboxConfig, logger *slog.Logger) (InboxReader, error) {
	if config.CredentialsJSON != nil {
		return NewGmailReader(config, logger)
	}
	
	if config.Host != "" {
		return NewIMAPReader(config, logger)
	}
	
	return nil, fmt.Errorf("invalid inbox configuration: missing required fields")
}