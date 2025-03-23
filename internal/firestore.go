package internal

import (
	"context"
	"fmt"
	"log/slog"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

// UserManager handles user-related operations including users, resumes, and appointments
type UserManager struct {
	fs     *firestore.Client
	logger *slog.Logger
}

// Option is a function that configures a UserManager instance
type Option func(*UserManager) error

// WithLogger sets the logger for the UserManager instance
func WithLogger(logger *slog.Logger) Option {
	return func(c *UserManager) error {
		c.logger = logger
		return nil
	}
}

// New creates a new UserManager instance
func NewFirestoreClient(ctx context.Context, projectID string, credentialsFile string, opts ...Option) (*firestore.Client, error) {
	opt := option.WithCredentialsFile(credentialsFile)
	config := &firebase.Config{ProjectID: projectID}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	return fs, nil
}
