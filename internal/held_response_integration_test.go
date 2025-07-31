package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"talent-rodeo/internal/inbox"
	"testing"
	"time"

	"github.com/alexedwards/flow"
)

// TestHeldResponseServiceWorkflow tests the held response service workflow.
func TestHeldResponseServiceWorkflow(t *testing.T) {
	ctx := context.Background()
	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Create held response service
	mockSender := &MockResponseSender{}
	heldResponseService := NewHeldResponseService(fs, nil, mockSender, nil)

	// Create a test held response
	testEmail := &inbox.Email{
		ID:      "test-email-001",
		From:    "user@example.com",
		Subject: "Test Subject",
		Body:    "Test email body",
		Date:    time.Now(),
	}

	testResponse := &inbox.EmailResponse{
		Subject: "Re: Test Subject",
		Body:    "Test response body",
	}

	holdDecision := HoldDecision{
		ShouldHold: true,
		Reason:     "Test hold reason",
		Criteria:   "test",
	}

	// Create held response
	heldResponse, err := heldResponseService.CreateHeldResponse(ctx, testEmail, testResponse, "recipient@example.com", holdDecision)
	if err != nil {
		t.Fatalf("Failed to create held response: %v", err)
	}
	defer DeleteHeldResponse(ctx, fs, heldResponse.ID)

	if heldResponse.Status != HeldResponseStatusPending {
		t.Errorf("Expected status 'pending', got %s", heldResponse.Status)
	}

	// Test approval workflow
	adminUserID := "admin-123"
	err = heldResponseService.ApproveResponse(ctx, heldResponse.ID, adminUserID)
	if err != nil {
		t.Fatalf("Failed to approve response: %v", err)
	}

	// Verify email was sent
	if mockSender.sentCount != 1 {
		t.Errorf("Expected 1 email to be sent after approval, got %d", mockSender.sentCount)
	}

	// Verify status updated
	updatedResponse, exists, err := GetHeldResponse(ctx, fs, heldResponse.ID)
	if err != nil {
		t.Fatalf("Failed to get updated response: %v", err)
	}
	if !exists {
		t.Fatal("Held response not found after approval")
	}

	if updatedResponse.Status != HeldResponseStatusApproved {
		t.Errorf("Expected status 'approved', got %s", updatedResponse.Status)
	}

	if updatedResponse.AdminUserID != adminUserID {
		t.Errorf("Expected admin user ID %s, got %s", adminUserID, updatedResponse.AdminUserID)
	}
}

// TestHeldResponseRevisionWorkflow tests the revision request workflow.
func TestHeldResponseRevisionWorkflow(t *testing.T) {
	ctx := context.Background()
	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Create a held response for revision testing
	heldResponse := &HeldResponse{
		ID:              "test-revision-001",
		Status:          HeldResponseStatusPending,
		OriginalEmailID: "email-001",
		OriginalFrom:    "user@example.com",
		OriginalSubject: "Test Subject",
		OriginalBody:    "Test email body",
		OriginalDate:    time.Now().Add(-1 * time.Hour),
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Re: Test Subject",
		ResponseBody:    "Original response body",
		ResponseType:    "job_search",
		CreatedAt:       time.Now().Add(-30 * time.Minute),
		ExpiresAt:       time.Now().Add(23 * time.Hour),
		HoldCriteria:    "keyword",
		HoldReason:      "Contains keyword: urgent",
		RevisionCount:   0,
	}

	err = CreateHeldResponse(ctx, fs, heldResponse)
	if err != nil {
		t.Fatalf("Failed to create test held response: %v", err)
	}
	defer DeleteHeldResponse(ctx, fs, heldResponse.ID)

	// Create service
	heldResponseService := NewHeldResponseService(fs, nil, nil, nil)

	// Request revision
	adminUserID := "admin-456"
	feedback := "Please make the response more personalized and include specific next steps."

	err = heldResponseService.RequestRevision(ctx, heldResponse.ID, adminUserID, feedback)
	if err != nil {
		t.Fatalf("Failed to request revision: %v", err)
	}

	// Verify held response was updated
	updatedResponse, exists, err := GetHeldResponse(ctx, fs, heldResponse.ID)
	if err != nil {
		t.Fatalf("Failed to get updated response: %v", err)
	}
	if !exists {
		t.Fatal("Held response not found after revision request")
	}

	if updatedResponse.Status != HeldResponseStatusRevision {
		t.Errorf("Expected status 'revision', got %s", updatedResponse.Status)
	}

	if updatedResponse.AdminFeedback != feedback {
		t.Errorf("Expected admin feedback %s, got %s", feedback, updatedResponse.AdminFeedback)
	}

	if updatedResponse.RevisionCount != 1 {
		t.Errorf("Expected revision count 1, got %d", updatedResponse.RevisionCount)
	}

	if updatedResponse.AdminUserID != adminUserID {
		t.Errorf("Expected admin user ID %s, got %s", adminUserID, updatedResponse.AdminUserID)
	}
}

// TestHeldResponseExpiration tests the expiration workflow.
func TestHeldResponseExpiration(t *testing.T) {
	ctx := context.Background()
	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Create an expired held response
	heldResponse := &HeldResponse{
		ID:              "test-expired-001",
		Status:          HeldResponseStatusPending,
		OriginalEmailID: "email-002",
		OriginalFrom:    "user@example.com",
		OriginalSubject: "Test Expired Subject",
		OriginalBody:    "Test expired email body",
		OriginalDate:    time.Now().Add(-2 * time.Hour),
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Re: Test Expired Subject",
		ResponseBody:    "Expired response body",
		ResponseType:    "job_search",
		CreatedAt:       time.Now().Add(-25 * time.Hour),
		ExpiresAt:       time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		HoldCriteria:    "keyword",
		HoldReason:      "Contains keyword: urgent",
		RevisionCount:   0,
	}

	err = CreateHeldResponse(ctx, fs, heldResponse)
	if err != nil {
		t.Fatalf("Failed to create test held response: %v", err)
	}
	defer DeleteHeldResponse(ctx, fs, heldResponse.ID)

	// Create service and mark expired
	heldResponseService := NewHeldResponseService(fs, nil, nil, nil)

	count, err := heldResponseService.MarkExpired(ctx)
	if err != nil {
		t.Fatalf("Failed to mark expired responses: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected to mark 1 response as expired, got %d", count)
	}

	// Verify status was updated
	updatedResponse, exists, err := GetHeldResponse(ctx, fs, heldResponse.ID)
	if err != nil {
		t.Fatalf("Failed to get updated response: %v", err)
	}
	if !exists {
		t.Fatal("Held response not found after expiration")
	}

	if updatedResponse.Status != HeldResponseStatusExpired {
		t.Errorf("Expected status 'expired', got %s", updatedResponse.Status)
	}
}

// TestHeldResponseAdminHandlers tests the admin HTTP handlers.
func TestHeldResponseAdminHandlers(t *testing.T) {
	ctx := context.Background()
	fs := getTestFirestoreClient(t)
	defer fs.Close()

	// Create test held response
	heldResponse := &HeldResponse{
		ID:              "test-admin-001",
		Status:          HeldResponseStatusPending,
		OriginalEmailID: "email-003",
		OriginalFrom:    "user@example.com",
		OriginalSubject: "Test Admin Subject",
		OriginalBody:    "Test admin email body",
		OriginalDate:    time.Now().Add(-1 * time.Hour),
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Re: Test Admin Subject",
		ResponseBody:    "Admin test response body",
		ResponseType:    "confirmation",
		CreatedAt:       time.Now().Add(-30 * time.Minute),
		ExpiresAt:       time.Now().Add(23 * time.Hour),
		HoldCriteria:    "keyword",
		HoldReason:      "Contains keyword: urgent",
		RevisionCount:   0,
	}

	err := CreateHeldResponse(ctx, fs, heldResponse)
	if err != nil {
		t.Fatalf("Failed to create test held response: %v", err)
	}
	defer DeleteHeldResponse(ctx, fs, heldResponse.ID)

	// Mock application with held response service
	mockApp := &MockApplication{
		fs:                  fs,
		heldResponseService: NewHeldResponseService(fs, nil, &MockResponseSender{}, nil),
	}

	// Test list handler
	req := httptest.NewRequest("GET", "/admin/held-responses", nil)
	w := httptest.NewRecorder()

	mockApp.adminHeldResponses(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test detail handler
	mux := flow.New()
	mux.HandleFunc("/admin/held-responses/:id", mockApp.adminHeldResponseDetail, "GET", "POST")

	req = httptest.NewRequest("GET", "/admin/held-responses/"+heldResponse.ID, nil)
	w = httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test approval via POST
	form := url.Values{}
	form.Add("action", "approve")

	req = httptest.NewRequest("POST", "/admin/held-responses/"+heldResponse.ID, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Response body: %s", w.Code, w.Body.String())
	}

	// Verify response was approved
	updatedResponse, exists, err := GetHeldResponse(ctx, fs, heldResponse.ID)
	if err != nil {
		t.Fatalf("Failed to get updated response: %v", err)
	}
	if !exists {
		t.Fatal("Held response not found after approval")
	}

	if updatedResponse.Status != HeldResponseStatusApproved {
		t.Errorf("Expected status 'approved', got %s", updatedResponse.Status)
	}
}

// MockApplication provides a minimal mock of the web application for testing.
type MockApplication struct {
	fs     *FirestoreClient
	logger *slog.Logger

	heldResponseService *HeldResponseService
}

func (app *MockApplication) newTemplateData(r *http.Request) map[string]interface{} {
	return map[string]interface{}{}
}

func (app *MockApplication) serverError(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (app *MockApplication) adminHeldResponses(w http.ResponseWriter, r *http.Request) {
	// Simplified implementation for testing
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (app *MockApplication) adminHeldResponseDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	responseID := flow.Param(ctx, "id")

	if responseID == "" {
		app.serverError(w, r, fmt.Errorf("response ID is required"))
		return
	}

	// Handle POST actions (approve/revision) - similar to real handler
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		action := r.PostForm.Get("action")
		switch action {
		case "approve":
			err := app.heldResponseService.ApproveResponse(ctx, responseID, "test-admin")
			if err != nil {
				app.serverError(w, r, fmt.Errorf("failed to approve response: %w", err))
				return
			}
		case "revision":
			feedback := r.PostForm.Get("feedback")
			if feedback == "" {
				app.serverError(w, r, fmt.Errorf("feedback is required for revision requests"))
				return
			}
			err := app.heldResponseService.RequestRevision(ctx, responseID, "test-admin", feedback)
			if err != nil {
				app.serverError(w, r, fmt.Errorf("failed to request revision: %w", err))
				return
			}
		default:
			app.serverError(w, r, fmt.Errorf("invalid action: %s", action))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// MockEmailHandler provides a mock email handler for testing.
type MockEmailHandler struct {
	responses []*inbox.EmailResponse
}

func (h *MockEmailHandler) HandleEmail(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
	return &inbox.HandlingResult{
		Handled: true,
		Summary: "Mock handling",
		Metadata: map[string]interface{}{
			"responses": h.responses,
		},
	}, nil
}
