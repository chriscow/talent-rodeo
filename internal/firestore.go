package internal

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/dgraph-io/ristretto/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// FirestoreClient implements FirestoreClientInterface
type FirestoreClient struct {
	*firestore.Client

	jobListingCache   *ristretto.Cache[string, *JobListing]
	jobDetailCache    *ristretto.Cache[string, *JobDetail]
	semanticJobsCache *ristretto.Cache[string, []ScoredJobDetail]
}

// Track if this is the first call to NewFirestoreClient
var initOnce sync.Once

// NewFirestoreClient creates a new Firestore client using Google Cloud credential chain:
// 1. GOOGLE_APPLICATION_CREDENTIALS environment variable pointing to a JSON credentials file
// 2. FIREBASE_CREDENTIALS environment variable containing the JSON credentials directly
// 3. Application Default Credentials (ADC) if running in Google Cloud
func NewFirestoreClient(ctx context.Context) (*FirestoreClient, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID must be set")
	}

	// Log project ID and first 20 chars of credentials on first call
	initOnce.Do(func() {
		logger := slog.Default()
		credentialsPreview := ""
		if credentials := os.Getenv("FIREBASE_CREDENTIALS"); credentials != "" {
			if len(credentials) > 20 {
				credentialsPreview = credentials[:20] + "..."
			} else {
				credentialsPreview = credentials + "..."
			}
		}

		usingEmulator := os.Getenv("FIRESTORE_EMULATOR_HOST") != ""
		if usingEmulator {
			logger.Info(BGMagenta("using Firestore emulator"),
				"projectID", projectID,
				"credentials_preview", credentialsPreview)
		}
	})

	config := &firebase.Config{ProjectID: projectID}
	var options []option.ClientOption

	// If FIREBASE_CREDENTIALS is set, use it directly
	if credentials := os.Getenv("FIREBASE_CREDENTIALS"); credentials != "" {
		decoded, err := base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			return nil, fmt.Errorf("failed to decode FIREBASE_CREDENTIALS: %w", err)
		}
		options = append(options, option.WithCredentialsJSON(decoded))
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

	jobListingCache, err := ristretto.NewCache(&ristretto.Config[string, *JobListing]{
		NumCounters: 1e6,     // number of keys to track frequency of
		MaxCost:     1 << 30, // maximum cost of cache (1GB)
		BufferItems: 64,      // number of keys per Get buffer
		Metrics:     false,   // no metrics collection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create job listing cache: %w", err)
	}

	jobDetailCache, err := ristretto.NewCache(&ristretto.Config[string, *JobDetail]{
		NumCounters: 1e6,     // number of keys to track frequency of
		MaxCost:     1 << 30, // maximum cost of cache (1GB)
		BufferItems: 64,      // number of keys per Get buffer
		Metrics:     false,   // no metrics collection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create job detail cache: %w", err)
	}

	semanticJobsCache, err := ristretto.NewCache(&ristretto.Config[string, []ScoredJobDetail]{
		NumCounters: 1e6,     // number of keys to track frequency of
		MaxCost:     1 << 28, // maximum cost of cache (256MB)
		BufferItems: 64,      // number of keys per Get buffer
		Metrics:     false,   // no metrics collection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create semantic jobs cache: %w", err)
	}

	return &FirestoreClient{
		Client:            fs,
		jobListingCache:   jobListingCache,
		jobDetailCache:    jobDetailCache,
		semanticJobsCache: semanticJobsCache,
	}, nil
}

func (fs *FirestoreClient) Close() error {
	if fs.jobListingCache != nil {
		fs.jobListingCache.Close()
	}

	if fs.jobDetailCache != nil {
		fs.jobDetailCache.Close()
	}

	if fs.semanticJobsCache != nil {
		fs.semanticJobsCache.Close()
	}

	if fs.Client != nil {
		return fs.Client.Close()
	}
	return nil
}

// DeleteCollection deletes all documents in a collection in batches using BulkWriter
// If recursive is true, it will also delete all documents in all subcollections
func DeleteCollection(ctx context.Context, fs *FirestoreClient, collectionPath string, batchSize int, recursive bool) error {
	if batchSize <= 0 {
		batchSize = 100 // Default batch size if not specified
	}

	bulkWriter := fs.BulkWriter(ctx)
	colRef := fs.Collection(collectionPath)

	// Keep deleting documents until the collection is empty
	for {
		// Get a batch of documents
		iter := colRef.Limit(batchSize).Documents(ctx)
		numDeleted := 0

		// Iterate through the documents, adding delete operations to the BulkWriter
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to get document in collection %s: %w", collectionPath, err)
			}

			// If recursive, delete all subcollections for this document first
			if recursive {
				subcollections := doc.Ref.Collections(ctx)
				for {
					subCol, err := subcollections.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						return fmt.Errorf("failed to get subcollection: %w", err)
					}

					// Recursively delete the subcollection
					err = DeleteCollection(ctx, fs, subCol.Path, batchSize, recursive)
					if err != nil {
						return fmt.Errorf("failed to delete subcollection %s: %w", subCol.Path, err)
					}
				}
			}

			bulkWriter.Delete(doc.Ref)
			numDeleted++
		}

		// If no documents were deleted, the collection is empty
		if numDeleted == 0 {
			bulkWriter.End()
			break
		}

		// Flush the current batch of delete operations
		bulkWriter.Flush()
	}

	return nil
}
