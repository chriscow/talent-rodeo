package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"talent-rodeo/internal/funcs"
)

func TestUserCriteriaSubscription(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client with emulator
	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewTestFirestoreClient() error = %v", err)
	}

	// Create a test user with unique email
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("test-criteria-%d@example.com", timestamp)
	testUser := CreateTestUser(t, fs, "Test User", email, "+14251234567", false)

	// Create a test criteria
	testCriteriaID := fmt.Sprintf("test-criteria-%d", timestamp)
	testCriteria := &SearchCriteria{
		ID:       testCriteriaID,
		Query:    "software engineer",
		GeoID:    "us-wa-seattle",
		MaxPages: 3,
	}

	// Save the criteria to Firestore
	_, err = fs.Collection(searchCriteriaCollection).Doc(testCriteria.ID).Create(ctx, testCriteria)
	if err != nil {
		t.Fatalf("Failed to create test criteria: %v", err)
	}

	// Setup cleanup
	t.Cleanup(func() {
		// Clean up test data
		DeleteTestData(t, fs, "test-user-")
		DeleteTestSearchCriteria(ctx, t, fs, "test-criteria-")
		DeleteTestUserCriteriaByPrefix(ctx, t, fs, "test-user-")
		fs.Close()
	})

	// Run tests
	t.Run("SubscribeUserToCriteria", func(t *testing.T) {
		// Test with valid inputs
		err := SubscribeToCriteria(ctx, fs, testUser.ID, testCriteria.ID)
		if err != nil {
			t.Fatalf("SubscribeUserToCriteria() error = %v", err)
		}

		// Verify the subscription was created
		subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", testUser.ID, testCriteria.ID))
		docRef := fs.Collection(userCriteriaCollection).Doc(subscriptionID)
		docSnap, err := docRef.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get subscription: %v", err)
		}

		var subscription SearchCriteriaSubscription
		if err := docSnap.DataTo(&subscription); err != nil {
			t.Fatalf("Failed to parse subscription data: %v", err)
		}

		if subscription.UserID != testUser.ID {
			t.Errorf("UserID = %v, want %v", subscription.UserID, testUser.ID)
		}
		if subscription.CriteriaID != testCriteria.ID {
			t.Errorf("CriteriaID = %v, want %v", subscription.CriteriaID, testCriteria.ID)
		}
		if subscription.SubscribedAt.IsZero() {
			t.Error("SubscribedAt should not be zero")
		}

		// Verify subscriber count was incremented
		criteriaRef := fs.Collection(searchCriteriaCollection).Doc(testCriteria.ID)
		criteriaDoc, err := criteriaRef.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get criteria: %v", err)
		}

		var updatedCriteria SearchCriteria
		if err := criteriaDoc.DataTo(&updatedCriteria); err != nil {
			t.Fatalf("Failed to parse criteria data: %v", err)
		}

		if updatedCriteria.SubscriberCount != 1 {
			t.Errorf("SubscriberCount = %v, want %v", updatedCriteria.SubscriberCount, 1)
		}
	})

	t.Run("SubscribeUserToCriteria_EmptyUserID", func(t *testing.T) {
		err := SubscribeToCriteria(ctx, fs, "", testCriteria.ID)
		if err == nil {
			t.Fatal("SubscribeUserToCriteria() with empty userID should return error")
		}
	})

	t.Run("SubscribeUserToCriteria_EmptyCriteriaID", func(t *testing.T) {
		err := SubscribeToCriteria(ctx, fs, testUser.ID, "")
		if err == nil {
			t.Fatal("SubscribeUserToCriteria() with empty criteriaID should return error")
		}
	})

	t.Run("SubscribeUserToCriteria_NonExistentCriteria", func(t *testing.T) {
		err := SubscribeToCriteria(ctx, fs, testUser.ID, "non-existent-criteria")
		if err == nil {
			t.Fatal("SubscribeUserToCriteria() with non-existent criteria should return error")
		}
	})

	t.Run("GetUserCriteriaSubscriptions", func(t *testing.T) {
		// Get subscriptions for the user
		subscriptions, err := GetSubscriptionsForUser(ctx, fs, testUser.ID)
		if err != nil {
			t.Fatalf("GetUserCriteriaSubscriptions() error = %v", err)
		}

		// Should have at least one subscription
		if len(subscriptions) == 0 {
			t.Fatal("GetUserCriteriaSubscriptions() returned no subscriptions, expected at least one")
		}

		// Verify the subscription details
		found := false
		for _, sub := range subscriptions {
			if sub.CriteriaID == testCriteria.ID {
				found = true
				if sub.UserID != testUser.ID {
					t.Errorf("UserID = %v, want %v", sub.UserID, testUser.ID)
				}
				if sub.SubscribedAt.IsZero() {
					t.Error("SubscribedAt should not be zero")
				}
				break
			}
		}
		if !found {
			t.Errorf("Subscription for criteria %s not found", testCriteria.ID)
		}
	})

	t.Run("GetUserCriteriaSubscriptions_EmptyUserID", func(t *testing.T) {
		_, err := GetSubscriptionsForUser(ctx, fs, "")
		if err == nil {
			t.Fatal("GetUserCriteriaSubscriptions() with empty userID should return error")
		}
	})

	t.Run("UnsubscribeFromCriteria_EmptyUserID", func(t *testing.T) {
		err := UnsubscribeFromCriteria(ctx, fs, "", testCriteria.ID)
		if err == nil {
			t.Fatal("UnsubscribeFromCriteria() with empty userID should return error")
		}
	})

	t.Run("UnsubscribeUserFromCriteria_EmptyCriteriaID", func(t *testing.T) {
		err := UnsubscribeFromCriteria(ctx, fs, testUser.ID, "")
		if err == nil {
			t.Fatal("UnsubscribeFromCriteria() with empty criteriaID should return error")
		}
	})
}

func TestUserCriteriaUnsubscribe(t *testing.T) {
	ctx := context.Background()

	var fs *FirestoreClient
	if testing.Short() {
		// Initialize Firestore client with emulator
		f, err := NewTestFirestoreClient(ctx)
		if err != nil {
			t.Fatalf("NewTestFirestoreClient() error = %v", err)
		}
		fs = f
	} else {
		f, err := NewFirestoreClient(ctx)
		if err != nil {
			t.Fatalf("NewFirestoreClient() error = %v", err)
		}
		fs = f
	}

	// Create a test user with unique email
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("test-criteria-%d@example.com", timestamp)
	testUser := CreateTestUser(t, fs, "Test User", email, "+14251234567", false)

	// Create a test criteria
	testCriteriaID := fmt.Sprintf("test-criteria-%d", timestamp)
	testCriteria := &SearchCriteria{
		ID:       testCriteriaID,
		Query:    "software engineer",
		GeoID:    "us-wa-seattle",
		MaxPages: 3,
	}

	// Save the criteria to Firestore
	_, err := fs.Collection(searchCriteriaCollection).Doc(testCriteria.ID).Create(ctx, testCriteria)
	if err != nil {
		t.Fatalf("Failed to create test criteria: %v", err)
	}

	// Setup cleanup
	t.Cleanup(func() {
		// Clean up test data
		DeleteTestData(t, fs, "test-user-")
		DeleteTestSearchCriteria(ctx, t, fs, "test-criteria-")
		DeleteTestUserCriteriaByPrefix(ctx, t, fs, "test-user-")
		fs.Close()
	})

	// Test unsubscribe functionality
	t.Run("UnsubscribeUserFromCriteria", func(t *testing.T) {
		// First subscribe the user
		err := SubscribeToCriteria(ctx, fs, testUser.ID, testCriteria.ID)
		if err != nil {
			t.Fatalf("Failed to subscribe user: %v", err)
		}

		// Then unsubscribe
		err = UnsubscribeFromCriteria(ctx, fs, testUser.ID, testCriteria.ID)
		if err != nil {
			t.Fatalf("UnsubscribeUserFromCriteria() error = %v", err)
		}

		// Verify subscription was removed
		subscriptionID := funcs.Slugify(fmt.Sprintf("%s_%s", testUser.ID, testCriteria.ID))
		_, err = fs.Collection(userCriteriaCollection).Doc(subscriptionID).Get(ctx)
		if err == nil {
			t.Error("Subscription document still exists after unsubscribe")
		}

		// Verify subscriber count was decremented
		criteriaDoc, err := fs.Collection(searchCriteriaCollection).Doc(testCriteria.ID).Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get criteria: %v", err)
		}

		var updatedCriteria SearchCriteria
		if err := criteriaDoc.DataTo(&updatedCriteria); err != nil {
			t.Fatalf("Failed to parse criteria data: %v", err)
		}

		if updatedCriteria.SubscriberCount != 0 {
			t.Errorf("SubscriberCount = %v, want %v", updatedCriteria.SubscriberCount, 0)
		}
	})
}
