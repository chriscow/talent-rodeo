package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

//
// `user_criteria` is a top-level collection that joins users to search criteria in
// a many-to-many relationship.
//
// Each document in this collection represents a user's subscription to a search
// criteria and contains the following fields:
//

const userCriteriaCollection = "user_criteria"

// UserCriteriaSubscription represents a user's subscription to a search criteria
type UserCriteriaSubscription struct {
	UserID       string
	CriteriaID   string
	SubscribedAt time.Time
	LastViewedAt time.Time
	DisplayName  string
}

// SubscribeUserToCriteria creates a subscription for a user to a search criteria
func SubscribeUserToCriteria(ctx context.Context, fs *FirestoreClient, userID, criteriaID, displayName string) error {
	// Validate inputs
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if criteriaID == "" {
		return fmt.Errorf("criteria ID is required")
	}

	// Check if the criteria exists
	_, found, err := GetSearchCriteria(fs, criteriaID)
	if err != nil {
		return fmt.Errorf("criteria not found: %w", err)
	}
	if !found {
		return fmt.Errorf("criteria not found")
	}

	// Create subscription document ID by combining user and criteria IDs
	subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", userID, criteriaID))

	// Create the subscription
	subscription := UserCriteriaSubscription{
		UserID:       userID,
		CriteriaID:   criteriaID,
		SubscribedAt: time.Now(),
		DisplayName:  displayName,
	}

	// Use a transaction to update both the subscription and increment the subscriber count
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Create/update the subscription
		subscriptionRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
		err := tx.Set(subscriptionRef, subscription)
		if err != nil {
			return err
		}

		// Increment the subscriber count for the criteria
		criteriaRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID)
		err = tx.Update(criteriaRef, []firestore.Update{
			{Path: "SubscriberCount", Value: firestore.Increment(1)},
		})
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

// UnsubscribeUserFromCriteria removes a user's subscription to a criteria
func UnsubscribeUserFromCriteria(fs *FirestoreClient, userID, criteriaID string) error {
	ctx := context.Background()

	// Validate inputs
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if criteriaID == "" {
		return fmt.Errorf("criteria ID is required")
	}

	// Create subscription document ID
	subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", userID, criteriaID))

	// Use a transaction to update both the subscription and decrement the subscriber count
	err := fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// First get the current count to ensure we don't go below zero
		criteriaRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID)
		doc, err := tx.Get(criteriaRef)
		if err != nil {
			return err
		}

		var criteria struct {
			SubscriberCount int `firestore:"SubscriberCount"`
		}
		if err := doc.DataTo(&criteria); err != nil {
			return err
		}

		// Delete the subscription
		subscriptionRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
		err = tx.Delete(subscriptionRef)
		if err != nil {
			return err
		}

		// Only decrement if count is > 0
		if criteria.SubscriberCount > 0 {
			err = tx.Update(criteriaRef, []firestore.Update{
				{Path: "SubscriberCount", Value: firestore.Increment(-1)},
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

// GetUserCriteriaSubscriptions retrieves all criteria a user is subscribed to
func GetUserCriteriaSubscriptions(fs *FirestoreClient, userID string) ([]UserCriteriaSubscription, error) {
	ctx := context.Background()

	// Validate input
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Query all subscriptions for the user
	query := fs.Collection(userCriteriaCollection).Where("UserID", "==", userID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var subscriptions []UserCriteriaSubscription
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating user criteria: %w", err)
		}

		var subscription UserCriteriaSubscription
		if err := doc.DataTo(&subscription); err != nil {
			return nil, fmt.Errorf("error parsing subscription data: %w", err)
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

// GetCriteriaSubscribers retrieves all users subscribed to a specific criteria
func GetCriteriaSubscribers(fs *FirestoreClient, criteriaID string) ([]UserCriteriaSubscription, error) {
	ctx := context.Background()

	// Validate input
	if criteriaID == "" {
		return nil, fmt.Errorf("criteria ID is required")
	}

	// Query all subscriptions for the criteria
	query := fs.Collection(userCriteriaCollection).Where("CriteriaID", "==", criteriaID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var subscribers []UserCriteriaSubscription
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating user criteria: %w", err)
		}

		var subscription UserCriteriaSubscription
		if err := doc.DataTo(&subscription); err != nil {
			return nil, fmt.Errorf("error parsing subscription data: %w", err)
		}

		subscribers = append(subscribers, subscription)
	}

	return subscribers, nil
}
