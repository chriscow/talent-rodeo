package internal

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/patrickmn/go-cache"
)

type FirestoreClient struct {
	*firestore.Client
	UsersCache *cache.Cache // *ristretto.Cache[string, *User]
}

// NewFirestoreClient creates a new Firestore client using Google Cloud credential chain:
// 1. GOOGLE_APPLICATION_CREDENTIALS environment variable pointing to a JSON credentials file
// 2. FIREBASE_CREDENTIALS environment variable containing the JSON credentials directly
// 3. Application Default Credentials (ADC) if running in Google Cloud
func NewFirestoreClient(ctx context.Context) (*FirestoreClient, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID must be set")
	}

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

	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	return &FirestoreClient{
		Client:     fs,
		UsersCache: userCache,
	}, nil
}

func (fs *FirestoreClient) Close() error {
	if fs.Client != nil {
		return fs.Client.Close()
	}
	return nil
}

// DeleteCollection deletes all documents in a collection in batches using BulkWriter
// If recursive is true, it will also delete all documents in all subcollections
func (fs *FirestoreClient) DeleteCollection(ctx context.Context, collectionPath string, batchSize int, recursive bool) error {
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
					err = fs.DeleteCollection(ctx, subCol.Path, batchSize, recursive)
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
