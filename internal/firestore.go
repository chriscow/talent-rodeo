package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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

// NewFirestoreClient creates a new Firestore client using Google Cloud credential chain:
// 1. GOOGLE_APPLICATION_CREDENTIALS environment variable pointing to a JSON credentials file
// 2. FIREBASE_CREDENTIALS environment variable containing the JSON credentials directly
// 3. Application Default Credentials (ADC) if running in Google Cloud
func NewFirestoreClient(ctx context.Context, opts ...Option) (*firestore.Client, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID must be set")
	}

	config := &firebase.Config{ProjectID: projectID}
	var options []option.ClientOption

	// If FIREBASE_CREDENTIALS is set, use it directly
	if credentials := os.Getenv("FIREBASE_CREDENTIALS"); credentials != "" {
		options = append(options, option.WithCredentialsJSON([]byte(credentials)))
	}

	// Initialize Firebase app with options (or default credentials if no options provided)
	app, err := firebase.NewApp(ctx, config, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	return fs, nil
}
