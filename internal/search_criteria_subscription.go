package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/samber/lo"
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

// SearchCriteriaSubscription represents a user's subscription to a search criteria
type SearchCriteriaSubscription struct {
	UserID       string    `firestore:"UserID"`
	CriteriaID   string    `firestore:"CriteriaID"`
	SubscribedAt time.Time `firestore:"SubscribedAt"`
	LastViewedAt time.Time `firestore:"LastViewedAt"`
	DisplayName  string    `firestore:"DisplayName"`
}

// SubscribeToCriteria creates a subscription for a user to a search criteria
func SubscribeToCriteria(ctx context.Context, fs *FirestoreClient, userID, criteriaID string) error {
	if fs == nil {
		return fmt.Errorf("FirestoreClient is nil")
	}
	// Validate inputs
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if criteriaID == "" {
		return fmt.Errorf("criteria ID is required")
	}

	// Check if the criteria exists
	_, found, err := GetSearchCriteria(ctx, fs, criteriaID)
	if err != nil {
		return fmt.Errorf("failed to check existence of criteria ID '%s': %w", criteriaID, err)
	}
	if !found {
		return fmt.Errorf("criteria with ID '%s' not found", criteriaID)
	}

	// Create subscription document ID by combining user and criteria IDs
	subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", userID, criteriaID))

	// Create the subscription
	subscription := SearchCriteriaSubscription{
		UserID:       userID,
		CriteriaID:   criteriaID,
		SubscribedAt: time.Now(),
	}

	// Use a transaction to update both the subscription and increment the subscriber count
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Create/update the subscription
		subscriptionRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
		err := tx.Set(subscriptionRef, subscription)
		if err != nil {
			return fmt.Errorf("SubscribeToCriteria: firestore.Transaction.Set: %w", err)
		}

		// Increment the subscriber count for the criteria
		criteriaRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID)
		err = tx.Update(criteriaRef, []firestore.Update{
			{Path: "SubscriberCount", Value: firestore.Increment(1)},
		})
		if err != nil {
			return fmt.Errorf("SubscribeToCriteria: firestore.Transaction.Update: %w", err)
		}

		return nil
	})

}

// UnsubscribeFromCriteria removes a user's subscription to a criteria
func UnsubscribeFromCriteria(ctx context.Context, fs *FirestoreClient, userID, criteriaID string) error {
	if fs == nil {
		return fmt.Errorf("FirestoreClient is nil")
	}

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
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// First get the current count to ensure we don't go below zero
		criteriaRef := fs.Collection(searchCriteriaCollection).Doc(criteriaID)
		doc, err := tx.Get(criteriaRef)
		if err != nil {
			return fmt.Errorf("UnsubscribeFromCriteria: firestore.Transaction.Get: %w", err)
		}

		var criteria struct {
			SubscriberCount int `firestore:"SubscriberCount"`
		}
		if err := doc.DataTo(&criteria); err != nil {
			return fmt.Errorf("UnsubscribeFromCriteria: firestore.Transaction.DataTo: %w", err)
		}

		// Delete the subscription
		subscriptionRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
		err = tx.Delete(subscriptionRef)
		if err != nil {
			return fmt.Errorf("UnsubscribeFromCriteria: firestore.Transaction.Delete: %w", err)
		}

		// Only decrement if count is > 0
		if criteria.SubscriberCount > 0 {
			err = tx.Update(criteriaRef, []firestore.Update{
				{Path: "SubscriberCount", Value: firestore.Increment(-1)},
			})
			if err != nil {
				return fmt.Errorf("UnsubscribeFromCriteria: firestore.Transaction.Update: %w", err)
			}
		}

		return nil
	})
}

func UnsubscribeFromCriteriaTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, userID, criteriaID string) error {
	if fs == nil {
		return fmt.Errorf("FirestoreClient is nil")
	}
	if tx == nil {
		return fmt.Errorf("Transaction is nil")
	}
	// Validate inputs
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if criteriaID == "" {
		return fmt.Errorf("criteria ID is required")
	}

	// Create subscription document ID
	subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", userID, criteriaID))

	// Delete the subscription
	subscriptionRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
	err := tx.Delete(subscriptionRef)
	if err != nil {
		return fmt.Errorf("UnsubscribeFromCriteriaTx: firestore.Transaction.Delete: %w", err)
	}
	return nil
}

// GetSubscriptionsForUser retrieves all criteria a user is subscribed to
func GetSubscriptionsForUser(ctx context.Context, fs *FirestoreClient, userID string) ([]SearchCriteriaSubscription, error) {
	if fs == nil {
		return nil, fmt.Errorf("FirestoreClient is nil")
	}

	// Validate input
	if userID == "" {
		return nil, fmt.Errorf("GetSubscriptionsForUser: user ID is required")
	}

	// Query all subscriptions for the user
	query := fs.Collection(userCriteriaCollection).Where("UserID", "==", userID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var subscriptions []SearchCriteriaSubscription
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GetSubscriptionsForUser: error iterating user criteria: %w", err)
		}

		var subscription SearchCriteriaSubscription
		if err := doc.DataTo(&subscription); err != nil {
			return nil, fmt.Errorf("GetSubscriptionsForUser: error parsing subscription data: %w", err)
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

// GetCriteriaSubscribers retrieves all users subscribed to a specific criteria
func GetCriteriaSubscribers(ctx context.Context, fs *FirestoreClient, criteriaID string) ([]SearchCriteriaSubscription, error) {
	if fs == nil {
		return nil, fmt.Errorf("FirestoreClient is nil")
	}

	// Validate input
	if criteriaID == "" {
		return nil, fmt.Errorf("GetCriteriaSubscribers: criteria ID is required")
	}

	// Query all subscriptions for the criteria
	query := fs.Collection(userCriteriaCollection).Where("CriteriaID", "==", criteriaID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var subscribers []SearchCriteriaSubscription
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GetCriteriaSubscribers: error iterating user criteria: %w", err)
		}

		var subscription SearchCriteriaSubscription
		if err := doc.DataTo(&subscription); err != nil {
			return nil, fmt.Errorf("error parsing subscription data: %w", err)
		}

		subscribers = append(subscribers, subscription)
	}

	return subscribers, nil
}

// ListSubscribedCriteriaIDs retrieves all criteria that has a user subscribed to it.
func ListSubscribedCriteriaIDs(ctx context.Context, fs *FirestoreClient) ([]string, error) {
	if fs == nil {
		return nil, fmt.Errorf("FirestoreClient is nil")
	}
	// Query all subscriptions for the user
	query := fs.Collection(userCriteriaCollection)
	iter := query.Documents(ctx)
	defer iter.Stop()

	criteria := map[string]string{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating user criteria: %w", err)
		}

		var subscription SearchCriteriaSubscription
		if err := doc.DataTo(&subscription); err != nil {
			return nil, fmt.Errorf("error parsing subscription data: %w", err)
		}

		criteria[subscription.CriteriaID] = subscription.CriteriaID
	}

	keys := lo.Keys(criteria)

	return keys, nil
}
