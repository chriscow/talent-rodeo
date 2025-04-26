package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/search"
	"time"
)

type UserSearch struct {
	// The time the search was queued
	QueuedAt *time.Time

	// The last time a successful search was completed
	// The time the search was completed
	CompletedAt *time.Time

	// The criteria for the search
	Criteria search.Criteria

	// A concise first-person statement of the user's preferences. This is
	// used to further generate the search.Criteria above.
	Statement string

	// Holds error messages from transcript processing.
	Error string
}

func UpdateUserSearch(ctx context.Context, fs *FirestoreClient, userID string, search *UserSearch) error {
	user, found, err := GetUser(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", userID)
	}

	user.Search = search

	return UpdateUser(ctx, fs, userID, map[string]any{
		"Search": user.Search,
	})
}
