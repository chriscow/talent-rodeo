package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"talent-rodeo/internal/inbox"
)

func TestHeldResponseService_ShouldHoldResponse(t *testing.T) {
	tests := []struct {
		name         string
		criteria     HoldCriteria
		originalEmail *inbox.Email
		response     *inbox.EmailResponse
		userEmail    string
		want         bool
	}{
		{
			name: "disabled criteria returns false",
			criteria: HoldCriteria{
				Enabled: false,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "test@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeConfirmation,
				Subject:  "RE: Test Subject",
				Body:     "Test response",
				HTMLBody: "<p>Test response</p>",
			},
			userEmail: "test@example.com",
			want:      false,
		},
		{
			name: "hold all enabled returns true",
			criteria: HoldCriteria{
				Enabled:                true,
				HoldAll:                true,
				DefaultExpirationHours: 24,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "test@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeConfirmation,
				Subject:  "RE: Test Subject",
				Body:     "Test response",
				HTMLBody: "<p>Test response</p>",
			},
			userEmail: "test@example.com",
			want:      true,
		},
		{
			name: "keyword match in response body",
			criteria: HoldCriteria{
				Enabled:                true,
				HoldKeywords:           []string{"urgent", "complaint"},
				DefaultExpirationHours: 24,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "test@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeConfirmation,
				Subject:  "RE: Test Subject",
				Body:     "This is urgent and needs attention",
				HTMLBody: "<p>This is urgent and needs attention</p>",
			},
			userEmail: "test@example.com",
			want:      true,
		},
		{
			name: "user email match",
			criteria: HoldCriteria{
				Enabled:                true,
				HoldUserEmails:         []string{"vip@example.com", "important@example.com"},
				DefaultExpirationHours: 24,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "vip@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeConfirmation,
				Subject:  "RE: Test Subject",
				Body:     "Normal response",
				HTMLBody: "<p>Normal response</p>",
			},
			userEmail: "vip@example.com",
			want:      true,
		},
		{
			name: "escalation response type match",
			criteria: HoldCriteria{
				Enabled:                 true,
				HoldEscalationResponses: true,
				DefaultExpirationHours:  24,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "test@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeEscalation,
				Subject:  "RE: Test Subject",
				Body:     "This requires escalation",
				HTMLBody: "<p>This requires escalation</p>",
			},
			userEmail: "test@example.com",
			want:      true,
		},
		{
			name: "no match",
			criteria: HoldCriteria{
				Enabled:                true,
				HoldKeywords:           []string{"urgent", "complaint"},
				HoldUserEmails:         []string{"vip@example.com", "important@example.com"},
				HoldEscalationResponses: true,
				DefaultExpirationHours: 24,
			},
			originalEmail: &inbox.Email{
				ID:      "test-email-1",
				From:    "test@example.com",
				Subject: "Test Subject",
				Body:    "Test body",
			},
			response: &inbox.EmailResponse{
				Type:     inbox.ResponseTypeConfirmation,
				Subject:  "RE: Test Subject",
				Body:     "Normal response",
				HTMLBody: "<p>Normal response</p>",
			},
			userEmail: "test@example.com",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := NewHoldDecisionEvaluator(tt.criteria)
			service := NewHeldResponseService(nil, evaluator, nil, nil)

			got := service.ShouldHoldResponse(context.Background(), tt.originalEmail, tt.response, tt.userEmail)
			if got != tt.want {
				t.Errorf("ShouldHoldResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeldResponseService_CreateHeldResponse(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	criteria := HoldCriteria{
		Enabled:                true,
		HoldAll:                true,
		DefaultExpirationHours: 24,
	}
	evaluator := NewHoldDecisionEvaluator(criteria)
	service := NewHeldResponseService(client, evaluator, nil, nil)

	originalEmail := &inbox.Email{
		ID:      "test-email-1",
		From:    "test@example.com",
		Subject: "Test Subject",
		Body:    "Test body",
	}

	response := &inbox.EmailResponse{
		Type:     inbox.ResponseTypeConfirmation,
		Subject:  "RE: Test Subject",
		Body:     "Test response",
		HTMLBody: "<p>Test response</p>",
	}

	holdDecision := HoldDecision{
		ShouldHold: true,
		Reason:     "test reason",
		Criteria:   "test_criteria",
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	t.Run("successful creation", func(t *testing.T) {
		heldResponse, err := service.CreateHeldResponse(ctx, originalEmail, response, "recipient@example.com", holdDecision)
		if err != nil {
			t.Fatalf("CreateHeldResponse failed: %v", err)
		}

		if heldResponse.ID == "" {
			t.Error("Expected non-empty ID")
		}
		if heldResponse.OriginalEmailID != originalEmail.ID {
			t.Errorf("Expected OriginalEmailID %s, got %s", originalEmail.ID, heldResponse.OriginalEmailID)
		}
		if heldResponse.Status != HeldResponseStatusPending {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusPending, heldResponse.Status)
		}
		if heldResponse.RecipientEmail != "recipient@example.com" {
			t.Errorf("Expected recipient %s, got %s", "recipient@example.com", heldResponse.RecipientEmail)
		}
		if heldResponse.ExpiresAt.Before(time.Now()) {
			t.Error("Expected expiration time in the future")
		}
		if heldResponse.HoldReason != holdDecision.Reason {
			t.Errorf("Expected HoldReason %s, got %s", holdDecision.Reason, heldResponse.HoldReason)
		}
		if heldResponse.HoldCriteria != holdDecision.Criteria {
			t.Errorf("Expected HoldCriteria %s, got %s", holdDecision.Criteria, heldResponse.HoldCriteria)
		}
	})

	t.Run("nil original email", func(t *testing.T) {
		_, err := service.CreateHeldResponse(ctx, nil, response, "recipient@example.com", holdDecision)
		if err == nil {
			t.Error("Expected error for nil original email")
		}
	})

	t.Run("nil response", func(t *testing.T) {
		_, err := service.CreateHeldResponse(ctx, originalEmail, nil, "recipient@example.com", holdDecision)
		if err == nil {
			t.Error("Expected error for nil response")
		}
	})

	t.Run("empty recipient email", func(t *testing.T) {
		_, err := service.CreateHeldResponse(ctx, originalEmail, response, "", holdDecision)
		if err == nil {
			t.Error("Expected error for empty recipient email")
		}
	})
}

func TestHeldResponseService_ApproveResponse(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	// Mock response sender
	mockSender := &MockResponseSender{}
	service := NewHeldResponseService(client, nil, mockSender, nil)

	// Create a test held response
	heldResponse := &HeldResponse{
		OriginalEmailID:    "test-email-1",
		OriginalFrom:       "test@example.com",
		OriginalSubject:    "Test Subject",
		OriginalBody:       "Test body",
		RecipientEmail:     "recipient@example.com",
		ResponseSubject:    "RE: Test Subject",
		ResponseBody:       "Test response",
		ResponseHTMLBody:   "<p>Test response</p>",
		ResponseType:       "confirmation",
		Status:             HeldResponseStatusPending,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		ExpiresAt:          time.Now().Add(24 * time.Hour),
		AdminFeedback:      "",
		RevisionCount:      0,
	}

	if err := CreateHeldResponse(ctx, client, heldResponse); err != nil {
		t.Fatalf("Failed to create test held response: %v", err)
	}

	t.Run("successful approval", func(t *testing.T) {
		err := service.ApproveResponse(ctx, heldResponse.ID, "admin-user-1")
		if err != nil {
			t.Fatalf("ApproveResponse failed: %v", err)
		}

		// Check that response was sent
		if !mockSender.SendResponseCalled {
			t.Error("Expected SendResponse to be called")
		}

		// Check that status was updated
		updated, exists, err := GetHeldResponse(ctx, client, heldResponse.ID)
		if err != nil {
			t.Fatalf("Failed to get updated held response: %v", err)
		}
		if !exists {
			t.Fatal("Held response not found after approval")
		}
		if updated.Status != HeldResponseStatusApproved {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusApproved, updated.Status)
		}
		if updated.AdminUserID != "admin-user-1" {
			t.Errorf("Expected AdminUserID %s, got %s", "admin-user-1", updated.AdminUserID)
		}
		if updated.ApprovedAt.IsZero() {
			t.Error("Expected ApprovedAt to be set")
		}
	})

	t.Run("response not found", func(t *testing.T) {
		err := service.ApproveResponse(ctx, "non-existent-id", "admin-user-1")
		if err == nil {
			t.Error("Expected error for non-existent response")
		}
	})
}

func TestHeldResponseService_RequestRevision(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	service := NewHeldResponseService(client, nil, nil, nil)

	// Create a test held response
	heldResponse := &HeldResponse{
		OriginalEmailID:    "test-email-2",
		OriginalFrom:       "test@example.com",
		OriginalSubject:    "Test Subject",
		OriginalBody:       "Test body",
		RecipientEmail:     "recipient@example.com",
		ResponseSubject:    "RE: Test Subject",
		ResponseBody:       "Test response",
		ResponseHTMLBody:   "<p>Test response</p>",
		ResponseType:       "confirmation",
		Status:             HeldResponseStatusPending,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		ExpiresAt:          time.Now().Add(24 * time.Hour),
		AdminFeedback:      "",
		RevisionCount:      0,
	}

	if err := CreateHeldResponse(ctx, client, heldResponse); err != nil {
		t.Fatalf("Failed to create test held response: %v", err)
	}

	t.Run("successful revision request", func(t *testing.T) {
		feedback := "Please make the tone more professional and add more details about the process."
		err := service.RequestRevision(ctx, heldResponse.ID, "admin-user-1", feedback)
		if err != nil {
			t.Fatalf("RequestRevision failed: %v", err)
		}

		// Check that status was updated
		updated, exists, err := GetHeldResponse(ctx, client, heldResponse.ID)
		if err != nil {
			t.Fatalf("Failed to get updated held response: %v", err)
		}
		if !exists {
			t.Fatal("Held response not found after revision request")
		}
		if updated.Status != HeldResponseStatusRevision {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusRevision, updated.Status)
		}
		if updated.AdminFeedback != feedback {
			t.Errorf("Expected AdminFeedback %s, got %s", feedback, updated.AdminFeedback)
		}
		if updated.RevisionCount != 1 {
			t.Errorf("Expected RevisionCount 1, got %d", updated.RevisionCount)
		}
		if updated.AdminUserID != "admin-user-1" {
			t.Errorf("Expected AdminUserID %s, got %s", "admin-user-1", updated.AdminUserID)
		}
	})

	t.Run("empty feedback", func(t *testing.T) {
		err := service.RequestRevision(ctx, heldResponse.ID, "admin-user-1", "")
		if err == nil {
			t.Error("Expected error for empty feedback")
		}
	})
}

func TestHeldResponseService_MarkExpired(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	service := NewHeldResponseService(client, nil, nil, nil)

	// Create expired held responses with unique IDs
	now := time.Now()
	testPrefix := fmt.Sprintf("test-expired-%d", now.UnixNano())
	
	expiredResponse1 := &HeldResponse{
		OriginalEmailID: testPrefix + "-email-1",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "RE: Test",
		ResponseBody:    "Test response",
		ResponseType:    "confirmation",
		Status:          HeldResponseStatusPending,
		CreatedAt:       now.Add(-25 * time.Hour),
		UpdatedAt:       now.Add(-25 * time.Hour),
		ExpiresAt:       now.Add(-1 * time.Hour),
	}

	expiredResponse2 := &HeldResponse{
		OriginalEmailID: testPrefix + "-email-2",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "RE: Test",
		ResponseBody:    "Test response",
		ResponseType:    "confirmation",
		Status:          HeldResponseStatusRevision,
		CreatedAt:       now.Add(-25 * time.Hour),
		UpdatedAt:       now.Add(-25 * time.Hour),
		ExpiresAt:       now.Add(-1 * time.Hour),
	}

	// Create non-expired response
	validResponse := &HeldResponse{
		OriginalEmailID: testPrefix + "-email-3",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "RE: Test",
		ResponseBody:    "Test response",
		ResponseType:    "confirmation",
		Status:          HeldResponseStatusPending,
		CreatedAt:       now.Add(-1 * time.Hour),
		UpdatedAt:       now.Add(-1 * time.Hour),
		ExpiresAt:       now.Add(23 * time.Hour),
	}

	responses := []*HeldResponse{expiredResponse1, expiredResponse2, validResponse}
	for _, response := range responses {
		if err := CreateHeldResponse(ctx, client, response); err != nil {
			t.Fatalf("Failed to create test held response: %v", err)
		}
	}

	t.Run("mark expired responses", func(t *testing.T) {
		count, err := service.MarkExpired(ctx)
		if err != nil {
			t.Fatalf("MarkExpired failed: %v", err)
		}

		if count != 2 {
			t.Errorf("Expected 2 expired responses, got %d", count)
		}

		// Check that expired responses were marked
		updated1, exists1, err := GetHeldResponse(ctx, client, expiredResponse1.ID)
		if err != nil {
			t.Fatalf("Failed to get updated response: %v", err)
		}
		if !exists1 {
			t.Fatal("Response 1 not found")
		}
		if updated1.Status != HeldResponseStatusExpired {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusExpired, updated1.Status)
		}

		updated2, exists2, err := GetHeldResponse(ctx, client, expiredResponse2.ID)
		if err != nil {
			t.Fatalf("Failed to get updated response: %v", err)
		}
		if !exists2 {
			t.Fatal("Response 2 not found")
		}
		if updated2.Status != HeldResponseStatusExpired {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusExpired, updated2.Status)
		}

		// Check that valid response was not changed
		updatedValid, existsValid, err := GetHeldResponse(ctx, client, validResponse.ID)
		if err != nil {
			t.Fatalf("Failed to get valid response: %v", err)
		}
		if !existsValid {
			t.Fatal("Valid response not found")
		}
		if updatedValid.Status != HeldResponseStatusPending {
			t.Errorf("Expected status %s, got %s", HeldResponseStatusPending, updatedValid.Status)
		}
	})
}

// MockResponseSender for testing
type MockResponseSender struct {
	SendResponseCalled bool
	SendResponseError  error
}

func (m *MockResponseSender) SendResponse(ctx context.Context, response *inbox.EmailResponse, recipientEmail string) error {
	m.SendResponseCalled = true
	return m.SendResponseError
}

func (m *MockResponseSender) Reset() {
	m.SendResponseCalled = false
	m.SendResponseError = nil
}