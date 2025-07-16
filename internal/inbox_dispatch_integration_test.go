package internal

import (
	"context"
	"testing"

	"talent-rodeo/internal/inbox"
)

func TestInboxDispatch_HeldResponseIntegration(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	// Create mock components
	mockResponder := &MockInboxEmailResponder{}
	mockSender := &MockResponseSender{}
	mockEscalation := &MockInboxEscalationService{}

	// Create test email
	testEmail := &inbox.Email{
		ID:      "test-integration-email",
		From:    "test@example.com",
		Subject: "Test Subject",
		Body:    "Test email body",
	}

	t.Run("response sent when holding disabled", func(t *testing.T) {
		// Setup held response service with disabled criteria
		holdCriteria := DefaultHoldCriteria()
		holdCriteria.Enabled = false
		holdEvaluator := NewHoldDecisionEvaluator(holdCriteria)
		heldResponseService := NewHeldResponseService(client, holdEvaluator, mockSender, nil)

		// Create inbox dispatch
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, heldResponseService, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "Test response body",
			HTMLBody: "<p>Test response body</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was sent
		if !mockSender.SendResponseCalled {
			t.Error("Expected SendResponse to be called")
		}
		if mockResponder.GenerateResponseCallCount != 1 {
			t.Errorf("Expected GenerateResponse to be called once, got %d", mockResponder.GenerateResponseCallCount)
		}
	})

	t.Run("response held when criteria match", func(t *testing.T) {
		// Setup held response service with enabled criteria
		holdCriteria := DefaultHoldCriteria()
		holdCriteria.Enabled = true
		holdCriteria.HoldAll = true // Hold all responses
		holdEvaluator := NewHoldDecisionEvaluator(holdCriteria)
		heldResponseService := NewHeldResponseService(client, holdEvaluator, mockSender, nil)

		// Create inbox dispatch
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, heldResponseService, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "Test response body",
			HTMLBody: "<p>Test response body</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was NOT sent
		if mockSender.SendResponseCalled {
			t.Error("Expected SendResponse NOT to be called when response is held")
		}
		if mockResponder.GenerateResponseCallCount != 1 {
			t.Errorf("Expected GenerateResponse to be called once, got %d", mockResponder.GenerateResponseCallCount)
		}

		// Verify held response was created
		pendingResponses, err := heldResponseService.GetPendingResponses(ctx)
		if err != nil {
			t.Fatalf("Failed to get pending responses: %v", err)
		}
		if len(pendingResponses) == 0 {
			t.Error("Expected at least one pending response")
		}
	})

	t.Run("response held based on keyword criteria", func(t *testing.T) {
		// Setup held response service with keyword criteria
		holdCriteria := DefaultHoldCriteria()
		holdCriteria.Enabled = true
		holdCriteria.HoldKeywords = []string{"urgent", "complaint"}
		holdEvaluator := NewHoldDecisionEvaluator(holdCriteria)
		heldResponseService := NewHeldResponseService(client, holdEvaluator, mockSender, nil)

		// Create inbox dispatch
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, heldResponseService, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response with urgent keyword
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "This is urgent and needs immediate attention",
			HTMLBody: "<p>This is urgent and needs immediate attention</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was NOT sent
		if mockSender.SendResponseCalled {
			t.Error("Expected SendResponse NOT to be called when response contains urgent keyword")
		}

		// Verify held response was created
		pendingResponses, err := heldResponseService.GetPendingResponses(ctx)
		if err != nil {
			t.Fatalf("Failed to get pending responses: %v", err)
		}
		
		foundUrgentResponse := false
		for _, resp := range pendingResponses {
			if resp.HoldCriteria == "keyword" {
				foundUrgentResponse = true
				break
			}
		}
		if !foundUrgentResponse {
			t.Error("Expected to find held response with keyword criteria")
		}
	})

	t.Run("response sent when no criteria match", func(t *testing.T) {
		// Setup held response service with specific criteria that won't match
		holdCriteria := DefaultHoldCriteria()
		holdCriteria.Enabled = true
		holdCriteria.HoldKeywords = []string{"refund", "cancel"}
		holdEvaluator := NewHoldDecisionEvaluator(holdCriteria)
		heldResponseService := NewHeldResponseService(client, holdEvaluator, mockSender, nil)

		// Create inbox dispatch
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, heldResponseService, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response without matching keywords
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "Thank you for your inquiry",
			HTMLBody: "<p>Thank you for your inquiry</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was sent
		if !mockSender.SendResponseCalled {
			t.Error("Expected SendResponse to be called when no criteria match")
		}
	})
}

func TestInboxDispatch_HeldResponseFailureScenarios(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	// Create mock components
	mockResponder := &MockInboxEmailResponder{}
	mockSender := &MockResponseSender{}
	mockEscalation := &MockInboxEscalationService{}

	// Create test email
	testEmail := &inbox.Email{
		ID:      "test-failure-email",
		From:    "test@example.com",
		Subject: "Test Subject",
		Body:    "Test email body",
	}

	t.Run("response sent when held response service is nil", func(t *testing.T) {
		// Create inbox dispatch without held response service
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, nil, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "Test response body",
			HTMLBody: "<p>Test response body</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was sent
		if !mockSender.SendResponseCalled {
			t.Error("Expected SendResponse to be called when held response service is nil")
		}
	})

	t.Run("response sent when held response creation fails", func(t *testing.T) {
		// Create held response service with invalid configuration to cause failure
		holdCriteria := DefaultHoldCriteria()
		holdCriteria.Enabled = true
		holdCriteria.HoldAll = true
		holdEvaluator := NewHoldDecisionEvaluator(holdCriteria)
		
		// Use a nil FirestoreClient to cause CreateHeldResponse to fail
		heldResponseService := NewHeldResponseService(nil, holdEvaluator, mockSender, nil)

		// Create inbox dispatch
		dispatch := NewEnhancedInboxDispatch(mockResponder, mockSender, mockEscalation, heldResponseService, nil)

		// Register a dummy handler to generate results
		dummyHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
			return &inbox.HandlingResult{
				Handled: true,
				Summary: "Test handler processed email",
			}, nil
		})
		dispatch.Register("test-handler", dummyHandler)

		// Reset mock state
		mockResponder.Reset()
		mockSender.Reset()

		// Set up mock response
		mockResponder.NextResponse = &inbox.EmailResponse{
			Type:     inbox.ResponseTypeConfirmation,
			Subject:  "RE: Test Subject",
			Body:     "Test response body",
			HTMLBody: "<p>Test response body</p>",
		}

		// Process email
		err := dispatch.HandleEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("HandleEmail failed: %v", err)
		}

		// Verify response was sent even though hold creation failed
		if !mockSender.SendResponseCalled {
			t.Error("Expected SendResponse to be called when held response creation fails")
		}
	})
}

// Mock implementations for testing

type MockInboxEmailResponder struct {
	NextResponse             *inbox.EmailResponse
	NextError                error
	GenerateResponseCallCount int
	LastEmail                *inbox.Email
	LastResults              []*inbox.HandlingResult
}

func (m *MockInboxEmailResponder) GenerateResponse(ctx context.Context, email *inbox.Email, results []*inbox.HandlingResult) (*inbox.EmailResponse, error) {
	m.GenerateResponseCallCount++
	m.LastEmail = email
	m.LastResults = results
	return m.NextResponse, m.NextError
}

func (m *MockInboxEmailResponder) Reset() {
	m.NextResponse = nil
	m.NextError = nil
	m.GenerateResponseCallCount = 0
	m.LastEmail = nil
	m.LastResults = nil
}

// MockResponseSender is already defined in held_response_service_test.go
// Use the existing one and add Reset method if needed

type MockInboxEscalationService struct {
	SendEscalationCalled bool
	SendEscalationError  error
	LastEmail           *inbox.Email
	LastUnhandledItems  []string
}

func (m *MockInboxEscalationService) SendEscalation(ctx context.Context, email *inbox.Email, unhandledItems []string) error {
	m.SendEscalationCalled = true
	m.LastEmail = email
	m.LastUnhandledItems = unhandledItems
	return m.SendEscalationError
}

func (m *MockInboxEscalationService) IsConfigured() bool {
	return true
}

func (m *MockInboxEscalationService) Reset() {
	m.SendEscalationCalled = false
	m.SendEscalationError = nil
	m.LastEmail = nil
	m.LastUnhandledItems = nil
}