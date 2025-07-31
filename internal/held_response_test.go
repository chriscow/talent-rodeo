package internal

import (
	"context"
	"testing"
	"time"
)

func TestHeldResponseStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status HeldResponseStatus
		want   bool
	}{
		{"valid pending", HeldResponseStatusPending, true},
		{"valid approved", HeldResponseStatusApproved, true},
		{"valid revision", HeldResponseStatusRevision, true},
		{"valid expired", HeldResponseStatusExpired, true},
		{"invalid empty", HeldResponseStatus(""), false},
		{"invalid unknown", HeldResponseStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("HeldResponseStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeldResponse_IsValid(t *testing.T) {
	validHeldResponse := &HeldResponse{
		OriginalEmailID: "test-email-id",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Test Subject",
		ResponseBody:    "Test Body",
		Status:          HeldResponseStatusPending,
	}

	tests := []struct {
		name    string
		hr      *HeldResponse
		wantErr bool
	}{
		{"valid held response", validHeldResponse, false},
		{"missing original email ID", &HeldResponse{
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, true},
		{"missing original from", &HeldResponse{
			OriginalEmailID: "test-email-id",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, true},
		{"missing recipient email", &HeldResponse{
			OriginalEmailID: "test-email-id",
			OriginalFrom:    "test@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, true},
		{"missing response subject", &HeldResponse{
			OriginalEmailID: "test-email-id",
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, true},
		{"missing response body", &HeldResponse{
			OriginalEmailID: "test-email-id",
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			Status:          HeldResponseStatusPending,
		}, true},
		{"valid with HTML body only", &HeldResponse{
			OriginalEmailID:  "test-email-id",
			OriginalFrom:     "test@example.com",
			RecipientEmail:   "recipient@example.com",
			ResponseSubject:  "Test Subject",
			ResponseHTMLBody: "<p>Test HTML Body</p>",
			Status:           HeldResponseStatusPending,
		}, false},
		{"invalid status", &HeldResponse{
			OriginalEmailID: "test-email-id",
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatus("invalid"),
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hr.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("HeldResponse.IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHeldResponse_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"expired - 1 hour ago", now.Add(-1 * time.Hour), true},
		{"not expired - 1 hour from now", now.Add(1 * time.Hour), false},
		{"expired - exactly now", now, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := &HeldResponse{
				ExpiresAt: tt.expiresAt,
			}
			if got := hr.IsExpired(); got != tt.want {
				t.Errorf("HeldResponse.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeldResponse_StatusChecks(t *testing.T) {
	tests := []struct {
		name       string
		status     HeldResponseStatus
		isPending  bool
		isApproved bool
		needsRev   bool
	}{
		{"pending status", HeldResponseStatusPending, true, false, false},
		{"approved status", HeldResponseStatusApproved, false, true, false},
		{"revision status", HeldResponseStatusRevision, false, false, true},
		{"expired status", HeldResponseStatusExpired, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := &HeldResponse{Status: tt.status}

			if got := hr.IsPending(); got != tt.isPending {
				t.Errorf("HeldResponse.IsPending() = %v, want %v", got, tt.isPending)
			}
			if got := hr.IsApproved(); got != tt.isApproved {
				t.Errorf("HeldResponse.IsApproved() = %v, want %v", got, tt.isApproved)
			}
			if got := hr.NeedsRevision(); got != tt.needsRev {
				t.Errorf("HeldResponse.NeedsRevision() = %v, want %v", got, tt.needsRev)
			}
		})
	}
}

func TestHeldResponse_GetPreviewText(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		htmlBody     string
		want         string
	}{
		{"short text", "Short text", "", "Short text"},
		{"long text", "This is a very long text that should be truncated because it exceeds the maximum length of 200 characters. This is a very long text that should be truncated because it exceeds the maximum length of 200 characters.", "", "This is a very long text that should be truncated because it exceeds the maximum length of 200 characters. This is a very long text that should be truncated because it exceeds the maximum length of 20..."},
		{"empty body uses HTML", "", "<p>HTML content</p>", "<p>HTML content</p>"},
		{"both present uses body", "Plain text", "<p>HTML content</p>", "Plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := &HeldResponse{
				ResponseBody:     tt.responseBody,
				ResponseHTMLBody: tt.htmlBody,
			}
			if got := hr.GetPreviewText(); got != tt.want {
				t.Errorf("HeldResponse.GetPreviewText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateHeldResponse(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	tests := []struct {
		name    string
		hr      *HeldResponse
		wantErr bool
	}{
		{"valid held response", &HeldResponse{
			OriginalEmailID: "test-email-id",
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, false},
		{"nil held response", nil, true},
		{"invalid held response", &HeldResponse{
			OriginalEmailID: "", // Invalid - empty
			OriginalFrom:    "test@example.com",
			RecipientEmail:  "recipient@example.com",
			ResponseSubject: "Test Subject",
			ResponseBody:    "Test Body",
			Status:          HeldResponseStatusPending,
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateHeldResponse(ctx, fs, tt.hr)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateHeldResponse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil && tt.hr != nil {
				// Verify the held response was created with proper defaults
				if tt.hr.ID == "" {
					t.Error("Expected ID to be generated")
				}
				if tt.hr.CreatedAt.IsZero() {
					t.Error("Expected CreatedAt to be set")
				}
				if tt.hr.UpdatedAt.IsZero() {
					t.Error("Expected UpdatedAt to be set")
				}
				if tt.hr.Status == "" {
					t.Error("Expected Status to be set to default")
				}
				if tt.hr.ExpiresAt.IsZero() {
					t.Error("Expected ExpiresAt to be set")
				}

				// Clean up
				_ = DeleteHeldResponse(ctx, fs, tt.hr.ID)
			}
		})
	}
}

func TestGetHeldResponse(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Create a test held response
	hr := &HeldResponse{
		OriginalEmailID: "test-email-id",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Test Subject",
		ResponseBody:    "Test Body",
		Status:          HeldResponseStatusPending,
	}

	err2 := CreateHeldResponse(ctx, fs, hr)
	if err2 != nil {
		t.Fatalf("Failed to create test held response: %v", err2)
	}
	defer DeleteHeldResponse(ctx, fs, hr.ID)

	tests := []struct {
		name    string
		id      string
		want    bool
		wantErr bool
	}{
		{"existing held response", hr.ID, true, false},
		{"non-existent held response", "non-existent-id", false, false},
		{"empty id", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := GetHeldResponse(ctx, fs, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHeldResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if found != tt.want {
				t.Errorf("GetHeldResponse() found = %v, want %v", found, tt.want)
			}
			if found && got.ID != tt.id {
				t.Errorf("GetHeldResponse() ID = %v, want %v", got.ID, tt.id)
			}
		})
	}
}

func TestUpdateHeldResponseStatus(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Create a test held response
	hr := &HeldResponse{
		OriginalEmailID: "test-email-id",
		OriginalFrom:    "test@example.com",
		RecipientEmail:  "recipient@example.com",
		ResponseSubject: "Test Subject",
		ResponseBody:    "Test Body",
		Status:          HeldResponseStatusPending,
	}

	err2 := CreateHeldResponse(ctx, fs, hr)
	if err2 != nil {
		t.Fatalf("Failed to create test held response: %v", err2)
	}
	defer DeleteHeldResponse(ctx, fs, hr.ID)

	tests := []struct {
		name    string
		id      string
		status  HeldResponseStatus
		wantErr bool
	}{
		{"valid status update", hr.ID, HeldResponseStatusApproved, false},
		{"invalid status", hr.ID, HeldResponseStatus("invalid"), true},
		{"empty id", "", HeldResponseStatusApproved, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateHeldResponseStatus(ctx, fs, tt.id, tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateHeldResponseStatus() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil {
				// Verify the status was updated
				updated, found, err := GetHeldResponse(ctx, fs, tt.id)
				if err != nil {
					t.Errorf("Failed to get updated held response: %v", err)
				}
				if !found {
					t.Error("Expected to find updated held response")
				}
				if updated.Status != tt.status {
					t.Errorf("Expected status %v, got %v", tt.status, updated.Status)
				}
			}
		})
	}
}

func TestListHeldResponses(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Clean up any existing held responses to ensure test isolation
	existingResponses, err := ListHeldResponses(ctx, fs, "", 100, 0)
	if err != nil {
		t.Fatalf("Failed to list existing held responses: %v", err)
	}
	for _, hr := range existingResponses {
		_ = DeleteHeldResponse(ctx, fs, hr.ID)
	}

	// Create test held responses
	hr1 := &HeldResponse{
		OriginalEmailID: "test-email-id-1",
		OriginalFrom:    "test1@example.com",
		RecipientEmail:  "recipient1@example.com",
		ResponseSubject: "Test Subject 1",
		ResponseBody:    "Test Body 1",
		Status:          HeldResponseStatusPending,
	}
	hr2 := &HeldResponse{
		OriginalEmailID: "test-email-id-2",
		OriginalFrom:    "test2@example.com",
		RecipientEmail:  "recipient2@example.com",
		ResponseSubject: "Test Subject 2",
		ResponseBody:    "Test Body 2",
		Status:          HeldResponseStatusApproved,
	}

	err1 := CreateHeldResponse(ctx, fs, hr1)
	if err1 != nil {
		t.Fatalf("Failed to create test held response 1: %v", err1)
	}
	defer DeleteHeldResponse(ctx, fs, hr1.ID)

	err2 := CreateHeldResponse(ctx, fs, hr2)
	if err2 != nil {
		t.Fatalf("Failed to create test held response 2: %v", err2)
	}
	defer DeleteHeldResponse(ctx, fs, hr2.ID)

	// Add small delay to ensure ordering
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		name      string
		status    HeldResponseStatus
		limit     int
		offset    int
		wantCount int
		wantErr   bool
	}{
		{"all responses", "", 10, 0, 2, false},
		{"pending only", HeldResponseStatusPending, 10, 0, 1, false},
		{"approved only", HeldResponseStatusApproved, 10, 0, 1, false},
		{"with limit", "", 1, 0, 1, false},
		{"with offset", "", 10, 1, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ListHeldResponses(ctx, fs, tt.status, tt.limit, tt.offset)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListHeldResponses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("ListHeldResponses() count = %v, want %v", len(got), tt.wantCount)
			}
		})
	}
}

func TestGetPendingHeldResponses(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Clean up any existing held responses to ensure test isolation
	existingResponses, err := ListHeldResponses(ctx, fs, "", 100, 0)
	if err != nil {
		t.Fatalf("Failed to list existing held responses: %v", err)
	}
	for _, hr := range existingResponses {
		_ = DeleteHeldResponse(ctx, fs, hr.ID)
	}

	// Create test held responses
	hr1 := &HeldResponse{
		OriginalEmailID: "test-email-id-1",
		OriginalFrom:    "test1@example.com",
		RecipientEmail:  "recipient1@example.com",
		ResponseSubject: "Test Subject 1",
		ResponseBody:    "Test Body 1",
		Status:          HeldResponseStatusPending,
	}
	hr2 := &HeldResponse{
		OriginalEmailID: "test-email-id-2",
		OriginalFrom:    "test2@example.com",
		RecipientEmail:  "recipient2@example.com",
		ResponseSubject: "Test Subject 2",
		ResponseBody:    "Test Body 2",
		Status:          HeldResponseStatusApproved,
	}

	err1 := CreateHeldResponse(ctx, fs, hr1)
	if err1 != nil {
		t.Fatalf("Failed to create test held response 1: %v", err1)
	}
	defer DeleteHeldResponse(ctx, fs, hr1.ID)

	err2 := CreateHeldResponse(ctx, fs, hr2)
	if err2 != nil {
		t.Fatalf("Failed to create test held response 2: %v", err2)
	}
	defer DeleteHeldResponse(ctx, fs, hr2.ID)

	// Get pending responses
	pending, err3 := GetPendingHeldResponses(ctx, fs)
	if err3 != nil {
		t.Fatalf("GetPendingHeldResponses() error = %v", err3)
	}

	// Should only return the pending response
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending response, got %v", len(pending))
	}

	if len(pending) > 0 && pending[0].Status != HeldResponseStatusPending {
		t.Errorf("Expected pending status, got %v", pending[0].Status)
	}
}
