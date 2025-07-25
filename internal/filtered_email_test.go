package internal

import (
	"context"
	"talent-rodeo/internal/inbox"
	"testing"
)

func TestFilteredEmailTracking(t *testing.T) {
	ctx := context.Background()
	fs := getTestFirestoreClient(t)
	defer fs.Close()

	service := NewFilteredEmailService(fs)

	// Create a test email
	testEmail := &inbox.Email{
		ID:      "test-email-123",
		From:    "Chris Cowherd <madcow@gmail.com>",
		Subject: "Test email that should be filtered",
		Body:    "This is a test email body",
	}

	// Record a filtered email
	err := service.RecordFilteredEmail(
		ctx,
		testEmail,
		"user-verification",
		FilteredEmailReasonUserNotFound,
		"User not found in database",
		"madcow@gmail.com",
	)
	if err != nil {
		t.Fatalf("Failed to record filtered email: %v", err)
	}

	// Get statistics
	stats, err := service.GetStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Verify statistics
	totalFiltered, ok := stats["total_filtered"].(int)
	if !ok || totalFiltered != 1 {
		t.Errorf("Expected total_filtered to be 1, got %v", stats["total_filtered"])
	}

	filterCounts, ok := stats["by_filter"].(map[string]int)
	if !ok {
		t.Fatal("Expected by_filter to be map[string]int")
	}

	if filterCounts["user-verification"] != 1 {
		t.Errorf("Expected user-verification filter count to be 1, got %d", filterCounts["user-verification"])
	}

	reasonCounts, ok := stats["by_reason"].(map[string]int)
	if !ok {
		t.Fatal("Expected by_reason to be map[string]int")
	}

	if reasonCounts["user_not_found"] != 1 {
		t.Errorf("Expected user_not_found reason count to be 1, got %d", reasonCounts["user_not_found"])
	}

	// Test listing filtered emails
	filteredEmails, err := service.GetFilteredEmailsByFilter(ctx, "user-verification", 10)
	if err != nil {
		t.Fatalf("Failed to get filtered emails by filter: %v", err)
	}

	if len(filteredEmails) != 1 {
		t.Errorf("Expected 1 filtered email, got %d", len(filteredEmails))
	}

	if filteredEmails[0].From != testEmail.From {
		t.Errorf("Expected From to be %s, got %s", testEmail.From, filteredEmails[0].From)
	}

	if filteredEmails[0].ExtractedEmail != "madcow@gmail.com" {
		t.Errorf("Expected ExtractedEmail to be madcow@gmail.com, got %s", filteredEmails[0].ExtractedEmail)
	}

	t.Logf("✅ Filtered email tracking test passed")
}
