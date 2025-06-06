package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal"
)

// InboxReader defines the interface for reading emails from an inbox
type InboxReader interface {
	// GetUnreadEmails retrieves unread emails from the inbox
	GetUnreadEmails(ctx context.Context, maxResults int) ([]*internal.Email, error)

	// MarkAsRead marks an email as read
	MarkAsRead(ctx context.Context, emailID string) error

	// Close closes the inbox connection
	Close() error
}

// InboxConfig holds configuration for inbox readers
type InboxConfig struct {
	// For Gmail API
	CredentialsJSON []byte
	TokenPath       string

	// For IMAP
	Host     string
	Port     int
	Username string
	Password string
	UseTLS   bool
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
