package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal/inbox"
	"testing"
	"time"
)

// TestUserVerificationFilter tests the UserVerificationFilter functionality
func TestUserVerificationFilter(t *testing.T) {
	ctx := context.Background()
	fs := getTestFirestoreClient(t)
	defer fs.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	filter := NewUserVerificationFilter(fs, logger)

	// Create a test user with unique ID
	testUserID := fmt.Sprintf("test-user-%d", time.Now().UnixNano())
	testUser := &User{
		ID:        testUserID,
		Email:     "registered@example.com",
		FirstName: "Test",
		LastName:  "User",
		Status:    UserStatusPendingResumeUpload,
		CreatedAt: time.Now(),
	}

	err := CreateUser(ctx, fs, testUser)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	tests := []struct {
		name           string
		email          *inbox.Email
		expectedResult bool
		expectError    bool
	}{
		{
			name: "registered user should be processed",
			email: &inbox.Email{
				ID:      "email-001",
				From:    "registered@example.com",
				Subject: "Test from registered user",
				Body:    "Hello from registered user",
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "non-registered user should be filtered out",
			email: &inbox.Email{
				ID:      "email-002",
				From:    "noreply@google.com",
				Subject: "Security alert",
				Body:    "Your account was accessed",
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "empty From field should be filtered out",
			email: &inbox.Email{
				ID:      "email-003",
				From:    "",
				Subject: "Empty from field",
				Body:    "No sender specified",
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "nil email should return error",
			email: nil,
			expectedResult: false,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.ShouldProcess(ctx, tt.email)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

// TestEmailFilterChain tests the EmailFilterChain functionality
func TestEmailFilterChain(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	testEmail := &inbox.Email{
		ID:      "test-email-001",
		From:    "test@example.com",
		Subject: "Test email",
		Body:    "Test email body",
	}

	t.Run("empty chain should allow processing", func(t *testing.T) {
		chain := NewEmailFilterChain(logger)
		result, err := chain.ShouldProcess(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !result {
			t.Error("Empty chain should allow processing")
		}
	})

	t.Run("chain with passing filters should allow processing", func(t *testing.T) {
		chain := NewEmailFilterChain(logger)
		
		// Add filters that always pass
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		
		result, err := chain.ShouldProcess(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !result {
			t.Error("Chain with passing filters should allow processing")
		}
	})

	t.Run("chain with rejecting filter should reject processing", func(t *testing.T) {
		chain := NewEmailFilterChain(logger)
		
		// Add one passing filter and one rejecting filter
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return false, nil
		}))
		
		result, err := chain.ShouldProcess(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if result {
			t.Error("Chain with rejecting filter should reject processing")
		}
	})

	t.Run("chain with error should continue processing", func(t *testing.T) {
		chain := NewEmailFilterChain(logger)
		
		// Add filter that returns error and then passing filter
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return false, errors.New("test error")
		}))
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		
		result, err := chain.ShouldProcess(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !result {
			t.Error("Chain should continue processing after filter error")
		}
	})

	t.Run("filter count should be correct", func(t *testing.T) {
		chain := NewEmailFilterChain(logger)
		
		if chain.GetFilterCount() != 0 {
			t.Errorf("Expected filter count 0, got %d", chain.GetFilterCount())
		}
		
		chain.AddFilter(EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		
		if chain.GetFilterCount() != 1 {
			t.Errorf("Expected filter count 1, got %d", chain.GetFilterCount())
		}
	})
}

// TestInboxDispatchFilterIntegration tests the integration of filters with InboxDispatch
func TestInboxDispatchFilterIntegration(t *testing.T) {
	ctx := context.Background()
	
	testEmail := &inbox.Email{
		ID:      "test-email-001",
		From:    "test@example.com",
		Subject: "Test email",
		Body:    "Test email body",
	}

	t.Run("dispatch with no filters should process email", func(t *testing.T) {
		dispatch := NewInboxDispatch()
		
		// Add a test handler to verify processing
		handlerCalled := false
		dispatch.Register("test-handler", EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			handlerCalled = true
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler",
			}, nil
		}))
		
		err := dispatch.HandleEmail(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !handlerCalled {
			t.Error("Handler should have been called when no filters are registered")
		}
	})

	t.Run("dispatch with passing filter should process email", func(t *testing.T) {
		dispatch := NewInboxDispatch()
		
		// Add a filter that always passes
		dispatch.RegisterFilter("allow-filter", EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return true, nil
		}))
		
		// Add a test handler to verify processing
		handlerCalled := false
		dispatch.Register("test-handler", EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			handlerCalled = true
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler",
			}, nil
		}))
		
		err := dispatch.HandleEmail(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !handlerCalled {
			t.Error("Handler should have been called when filter passes")
		}
	})

	t.Run("dispatch with rejecting filter should not process email", func(t *testing.T) {
		dispatch := NewInboxDispatch()
		
		// Add a filter that always rejects
		dispatch.RegisterFilter("reject-filter", EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return false, nil
		}))
		
		// Add a test handler to verify it's not called
		handlerCalled := false
		dispatch.Register("test-handler", EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			handlerCalled = true
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler",
			}, nil
		}))
		
		err := dispatch.HandleEmail(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if handlerCalled {
			t.Error("Handler should not have been called when filter rejects")
		}
	})

	t.Run("dispatch with filter error should continue processing", func(t *testing.T) {
		dispatch := NewInboxDispatch()
		
		// Add a filter that returns error
		dispatch.RegisterFilter("error-filter", EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return false, errors.New("test filter error")
		}))
		
		// Add a test handler to verify processing continues
		handlerCalled := false
		dispatch.Register("test-handler", EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			handlerCalled = true
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler",
			}, nil
		}))
		
		err := dispatch.HandleEmail(ctx, testEmail)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if !handlerCalled {
			t.Error("Handler should have been called when filter has error (fail open)")
		}
	})
}

// TestFilterRegistration tests the filter registration and deregistration functionality
func TestFilterRegistration(t *testing.T) {
	dispatch := NewInboxDispatch()
	
	testFilter := EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
		return true, nil
	})

	t.Run("register valid filter should succeed", func(t *testing.T) {
		err := dispatch.RegisterFilter("test-filter", testFilter)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if dispatch.GetFilterCount() != 1 {
			t.Errorf("Expected filter count 1, got %d", dispatch.GetFilterCount())
		}
	})

	t.Run("register filter with empty ID should fail", func(t *testing.T) {
		err := dispatch.RegisterFilter("", testFilter)
		
		if err == nil {
			t.Error("Expected error for empty filter ID")
		}
	})

	t.Run("register nil filter should fail", func(t *testing.T) {
		err := dispatch.RegisterFilter("nil-filter", nil)
		
		if err == nil {
			t.Error("Expected error for nil filter")
		}
	})

	t.Run("deregister existing filter should succeed", func(t *testing.T) {
		// First register a filter
		dispatch.RegisterFilter("temp-filter", testFilter)
		
		result := dispatch.DeregisterFilter("temp-filter")
		
		if !result {
			t.Error("Expected deregister to return true for existing filter")
		}
	})

	t.Run("deregister non-existing filter should return false", func(t *testing.T) {
		result := dispatch.DeregisterFilter("non-existing-filter")
		
		if result {
			t.Error("Expected deregister to return false for non-existing filter")
		}
	})

	t.Run("replace existing filter should work", func(t *testing.T) {
		// Register first filter
		dispatch.RegisterFilter("replaceable-filter", testFilter)
		originalCount := dispatch.GetFilterCount()
		
		// Replace with new filter
		newFilter := EmailFiltererFunc(func(ctx context.Context, email *inbox.Email) (bool, error) {
			return false, nil
		})
		
		err := dispatch.RegisterFilter("replaceable-filter", newFilter)
		
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		if dispatch.GetFilterCount() != originalCount {
			t.Errorf("Expected filter count to remain %d, got %d", originalCount, dispatch.GetFilterCount())
		}
	})
}

// getTestFirestoreClient creates a test Firestore client
func getTestFirestoreClient(t *testing.T) *FirestoreClient {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	return fs
}