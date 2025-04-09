package internal

import (
	"context"
	"testing"

	"cloud.google.com/go/firestore"
)

func setupTestFirestore(t *testing.T) *firestore.Client {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create test firestore client: %v", err)
	}
	return fs
}

/*
func cleanupUserSearchResults(ctx context.Context, fs *firestore.Client, userID string) error {
	iter := fs.Collection("users").Doc(userID).Collection("search_results").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate search results: %w", err)
		}
		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete search result: %w", err)
		}
	}
	return nil
}

func TestSaveSearchResult(t *testing.T) {
	fs := setupTestFirestore(t)
	defer fs.Close()

	ctx := context.Background()
	tests := []struct {
		name        string
		result      *SearchResult
		wantErr     bool
		errContains string
	}{
		{
			name: "valid result",
			result: &SearchResult{
				UserID: "test-user",
				JobID:  "test-job",
				Status: "new",
			},
			wantErr: false,
		},
		{
			name: "missing user ID",
			result: &SearchResult{
				JobID:  "test-job",
				Status: "new",
			},
			wantErr:     true,
			errContains: "user ID is required",
		},
		{
			name: "missing job ID",
			result: &SearchResult{
				UserID: "test-user",
				Status: "new",
			},
			wantErr:     true,
			errContains: "job ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SaveSearchResult(ctx, fs, tt.result)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && err.Error() != tt.errContains {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify the result was saved
			if !tt.wantErr {
				doc, err := fs.Collection("users").Doc(tt.result.UserID).Collection("search_results").Doc(tt.result.JobID).Get(ctx)
				if err != nil {
					t.Errorf("failed to get saved result: %v", err)
					return
				}

				var saved SearchResult
				if err := doc.DataTo(&saved); err != nil {
					t.Errorf("failed to unmarshal saved result: %v", err)
					return
				}

				if saved.UserID != tt.result.UserID {
					t.Errorf("got user ID %q, want %q", saved.UserID, tt.result.UserID)
				}
				if saved.JobID != tt.result.JobID {
					t.Errorf("got job ID %q, want %q", saved.JobID, tt.result.JobID)
				}
				if saved.Status != tt.result.Status {
					t.Errorf("got status %q, want %q", saved.Status, tt.result.Status)
				}
			}
		})
	}
}

func TestGetUserSearchResults(t *testing.T) {
	fs := setupTestFirestore(t)
	defer fs.Close()

	ctx := context.Background()
	userID := "test-user"

	// Clean up any existing search results
	if err := cleanupUserSearchResults(ctx, fs, userID); err != nil {
		t.Fatalf("failed to cleanup existing search results: %v", err)
	}

	// Create some test results
	testResults := []*SearchResult{
		{
			UserID: userID,
			JobID:  "job1",
			Status: "new",
		},
		{
			UserID: userID,
			JobID:  "job2",
			Status: "viewed",
		},
	}

	// Save test results
	for _, result := range testResults {
		if err := SaveSearchResult(ctx, fs, result); err != nil {
			t.Fatalf("failed to save test result: %v", err)
		}
	}

	// Test getting results
	results, err := GetUserSearchResults(ctx, fs, userID)
	if err != nil {
		t.Fatalf("failed to get search results: %v", err)
	}

	if len(results) != len(testResults) {
		t.Errorf("got %d results, want %d", len(results), len(testResults))
	}

	// Test getting results for non-existent user
	results, err = GetUserSearchResults(ctx, fs, "non-existent")
	if err != nil {
		t.Errorf("unexpected error for non-existent user: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results for non-existent user, want 0", len(results))
	}
}

func TestUpdateSearchResultStatus(t *testing.T) {
	fs := setupTestFirestore(t)
	defer fs.Close()

	ctx := context.Background()
	userID := "test-user"
	jobID := "test-job"

	// Create initial result
	result := &SearchResult{
		UserID: userID,
		JobID:  jobID,
		Status: "new",
	}

	if err := SaveSearchResult(ctx, fs, result); err != nil {
		t.Fatalf("failed to save initial result: %v", err)
	}

	tests := []struct {
		name     string
		status   string
		wantTime bool // whether we expect a timestamp for this status
	}{
		{
			name:     "mark as viewed",
			status:   "viewed",
			wantTime: true,
		},
		{
			name:     "mark as applied",
			status:   "applied",
			wantTime: true,
		},
		{
			name:     "mark as rejected",
			status:   "rejected",
			wantTime: true,
		},
		{
			name:     "mark as other",
			status:   "other",
			wantTime: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateSearchResultStatus(ctx, fs, userID, jobID, tt.status)
			if err != nil {
				t.Errorf("failed to update status: %v", err)
				return
			}

			// Verify the update
			doc, err := fs.Collection("users").Doc(userID).Collection("search_results").Doc(jobID).Get(ctx)
			if err != nil {
				t.Errorf("failed to get updated result: %v", err)
				return
			}

			var updated SearchResult
			if err := doc.DataTo(&updated); err != nil {
				t.Errorf("failed to unmarshal updated result: %v", err)
				return
			}

			if updated.Status != tt.status {
				t.Errorf("got status %q, want %q", updated.Status, tt.status)
			}

			switch tt.status {
			case "viewed":
				if tt.wantTime && updated.ViewedAt == nil {
					t.Error("expected ViewedAt timestamp, got nil")
				}
			case "applied":
				if tt.wantTime && updated.AppliedAt == nil {
					t.Error("expected AppliedAt timestamp, got nil")
				}
			case "rejected":
				if tt.wantTime && updated.RejectedAt == nil {
					t.Error("expected RejectedAt timestamp, got nil")
				}
			}
		})
	}
}
*/
