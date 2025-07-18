package internal

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal/inbox"
)

// EmailFilterer defines the interface for filtering emails before they reach handlers.
// Filters can examine emails and decide whether they should be processed by handlers.
type EmailFilterer interface {
	// ShouldProcess determines whether an email should be processed by handlers.
	// Returns true if the email should be processed, false to skip processing.
	ShouldProcess(ctx context.Context, email *inbox.Email) (bool, error)
}

// EmailFiltererFunc is a function type that implements EmailFilterer
type EmailFiltererFunc func(ctx context.Context, email *inbox.Email) (bool, error)

func (f EmailFiltererFunc) ShouldProcess(ctx context.Context, email *inbox.Email) (bool, error) {
	return f(ctx, email)
}

// UserVerificationFilter filters emails to only process those from registered users.
// This prevents the system from responding to automated emails and spam.
type UserVerificationFilter struct {
	firestoreClient *FirestoreClient
	logger          *slog.Logger
}

// NewUserVerificationFilter creates a new user verification filter.
func NewUserVerificationFilter(firestoreClient *FirestoreClient, logger *slog.Logger) *UserVerificationFilter {
	return &UserVerificationFilter{
		firestoreClient: firestoreClient,
		logger:          logger,
	}
}

// ShouldProcess checks if the email sender is a registered user.
// Returns true if the sender is a registered user, false otherwise.
func (f *UserVerificationFilter) ShouldProcess(ctx context.Context, email *inbox.Email) (bool, error) {
	if email == nil {
		return false, fmt.Errorf("email cannot be nil")
	}

	if email.From == "" {
		if f.logger != nil {
			f.logger.Warn("email has empty From field, skipping processing",
				"email_id", email.ID,
				"subject", email.Subject,
			)
		}
		return false, nil
	}

	// Check if sender is a registered user
	user, exists, err := GetUserByEmail(ctx, f.firestoreClient, email.From)
	if err != nil {
		// Log error but don't fail the processing - allow email through on database errors
		if f.logger != nil {
			f.logger.Error("failed to check user existence, allowing email through",
				"email_from", email.From,
				"email_id", email.ID,
				"error", err,
			)
		}
		return true, nil
	}

	if !exists {
		if f.logger != nil {
			f.logger.Info("filtering out email from non-registered user",
				"email_from", email.From,
				"email_id", email.ID,
				"subject", email.Subject,
			)
		}
		return false, nil
	}

	if f.logger != nil {
		f.logger.Debug("email from registered user, allowing processing",
			"email_from", email.From,
			"email_id", email.ID,
			"user_id", user.ID,
			"subject", email.Subject,
		)
	}

	return true, nil
}

// FilterResult represents the result of email filtering
type FilterResult struct {
	ShouldProcess bool
	FilteredBy    string
	Reason        string
}

// EmailFilterChain manages multiple filters and executes them in sequence.
type EmailFilterChain struct {
	filters []EmailFilterer
	logger  *slog.Logger
}

// NewEmailFilterChain creates a new filter chain.
func NewEmailFilterChain(logger *slog.Logger) *EmailFilterChain {
	return &EmailFilterChain{
		filters: make([]EmailFilterer, 0),
		logger:  logger,
	}
}

// AddFilter adds a filter to the chain.
func (chain *EmailFilterChain) AddFilter(filter EmailFilterer) {
	chain.filters = append(chain.filters, filter)
}

// ShouldProcess runs all filters in the chain and returns false if any filter rejects the email.
func (chain *EmailFilterChain) ShouldProcess(ctx context.Context, email *inbox.Email) (bool, error) {
	for i, filter := range chain.filters {
		shouldProcess, err := filter.ShouldProcess(ctx, email)
		if err != nil {
			if chain.logger != nil {
				chain.logger.Error("filter error, allowing email through",
					"filter_index", i,
					"email_id", email.ID,
					"error", err,
				)
			}
			// Continue processing on filter errors
			continue
		}

		if !shouldProcess {
			if chain.logger != nil {
				chain.logger.Info("email filtered out by filter chain",
					"filter_index", i,
					"email_id", email.ID,
					"email_from", email.From,
					"subject", email.Subject,
				)
			}
			return false, nil
		}
	}

	return true, nil
}

// GetFilterCount returns the number of filters in the chain.
func (chain *EmailFilterChain) GetFilterCount() int {
	return len(chain.filters)
}