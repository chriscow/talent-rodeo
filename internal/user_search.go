package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/search"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	searchResultsCollection = "search_results"
)

type UserSearch struct {
	// The time the search was queued
	QueuedAt *time.Time `json:"queued_at" firestore:"queued_at"`

	// The last time a successful search was completed
	// The time the search was completed
	CompletedAt *time.Time `json:"completed_at" firestore:"completed_at"`

	// The criteria for the search
	Criteria search.Criteria `json:"criteria" firestore:"criteria"`

	// The statement of the user's search
	Statement string `json:"statement" firestore:"statement"`

	// Holds error messages from transcript processing.
	Error string `json:"error" firestore:"error"`
}

// SearchResult represents a job that was found for a specific user
// type SearchResult struct {
// 	UserID     string     `firestore:"user_id"`
// 	JobID      string     `firestore:"job_id"`
// 	CreatedAt  time.Time  `firestore:"created_at"`
// 	Status     string     `firestore:"status"` // e.g., "new", "viewed", "applied", "rejected"
// 	Notes      string     `firestore:"notes,omitempty"`
// 	ViewedAt   *time.Time `firestore:"viewed_at,omitempty"`
// 	AppliedAt  *time.Time `firestore:"applied_at,omitempty"`
// 	RejectedAt *time.Time `firestore:"rejected_at,omitempty"`
// 	SearchHash string     `firestore:"search_hash"`
// }

// // SaveSearchResult saves a search result for a user
// func SaveSearchResult(ctx context.Context, fs *firestore.Client, result *SearchResult) error {
// 	if result.UserID == "" {
// 		return fmt.Errorf("user ID is required")
// 	}
// 	if result.JobID == "" {
// 		return fmt.Errorf("job ID is required")
// 	}

// 	result.CreatedAt = time.Now()
// 	if result.Status == "" {
// 		result.Status = "new"
// 	}

// 	// Save to the user's search_results subcollection
// 	_, err := fs.Collection(usersCollection).Doc(result.UserID).Collection(searchResultsCollection).Doc(result.JobID).Set(ctx, result)
// 	if err != nil {
// 		return fmt.Errorf("failed to save search result: %w", err)
// 	}

// 	return nil
// }

// // GetUserSearchResults retrieves search results for a user
// func GetUserSearchResults(ctx context.Context, fs *firestore.Client, userID string) ([]*SearchResult, error) {
// 	iter := fs.Collection("users").Doc(userID).Collection("search_results").Documents(ctx)
// 	var results []*SearchResult

// 	for {
// 		doc, err := iter.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to iterate search results: %w", err)
// 		}

// 		var result SearchResult
// 		if err := doc.DataTo(&result); err != nil {
// 			return nil, fmt.Errorf("failed to unmarshal search result: %w", err)
// 		}
// 		results = append(results, &result)
// 	}

// 	return results, nil
// }

// // UpdateSearchResultStatus updates the status of a search result
// func UpdateSearchResultStatus(ctx context.Context, fs *firestore.Client, userID, jobID, status string) error {
// 	now := time.Now()
// 	updates := []firestore.Update{
// 		{Path: "status", Value: status},
// 		{Path: "updated_at", Value: now},
// 	}

// 	// Add timestamp based on status
// 	switch status {
// 	case "viewed":
// 		updates = append(updates, firestore.Update{Path: "viewed_at", Value: now})
// 	case "applied":
// 		updates = append(updates, firestore.Update{Path: "applied_at", Value: now})
// 	case "rejected":
// 		updates = append(updates, firestore.Update{Path: "rejected_at", Value: now})
// 	}

// 	_, err := fs.Collection("users").Doc(userID).Collection("search_results").Doc(jobID).Update(ctx, updates)
// 	if err != nil {
// 		return fmt.Errorf("failed to update search result status: %w", err)
// 	}

// 	return nil
// }

func UpdateUserSearch(ctx context.Context, fs *firestore.Client, userID string, search *UserSearch) error {
	user, found, err := GetUser(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", userID)
	}

	user.Search = search

	return UpdateUser(ctx, fs, userID, map[string]any{
		"search": user.Search,
	})
}
