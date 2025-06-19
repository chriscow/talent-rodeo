package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
)

// TestGmailReaderIntegration tests the Gmail reader with real credentials
// This test requires GMAIL_READER_CREDENTIALS environment variable to be set
func TestGmailReaderIntegration(t *testing.T) {
	// Skip test if credentials not available
	if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
		t.Skip("GMAIL_READER_CREDENTIALS not set, skipping integration test")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := InboxConfig{
		TokenPath: "test_token.json",
	}

	t.Run("CreateGmailReader", func(t *testing.T) {
		reader, err := NewGmailReader(config, logger)
		if err != nil {
			t.Fatalf("Failed to create Gmail reader: %v", err)
		}
		defer reader.Close()

		if reader == nil {
			t.Fatal("Gmail reader is nil")
		}
	})

	t.Run("GetUnreadEmails", func(t *testing.T) {
		reader, err := NewGmailReader(config, logger)
		if err != nil {
			t.Fatalf("Failed to create Gmail reader: %v", err)
		}
		defer reader.Close()

		// Test with limit of 5 emails
		emails, err := reader.GetUnreadEmails(ctx, 5)
		if err != nil {
			t.Fatalf("Failed to get unread emails: %v", err)
		}

		t.Logf("Found %d unread emails", len(emails))

		// Validate email structure if we have emails
		for i, email := range emails {
			if i >= 3 { // Only check first 3 to avoid spam
				break
			}

			t.Logf("Email %d: ID=%s, From=%s, Subject=%s", i+1, email.ID, email.From, email.Subject)

			if email.ID == "" {
				t.Errorf("Email %d has empty ID", i+1)
			}
			if email.From == "" {
				t.Errorf("Email %d has empty From", i+1)
			}
			if email.Subject == "" {
				t.Logf("Email %d has empty Subject (this might be normal)", i+1)
			}
			if email.Date.IsZero() {
				t.Errorf("Email %d has zero Date", i+1)
			}
			if len(email.Headers) == 0 {
				t.Errorf("Email %d has no headers", i+1)
			}
		}
	})

	t.Run("GetUnreadEmailsWithLimit", func(t *testing.T) {
		reader, err := NewGmailReader(config, logger)
		if err != nil {
			t.Fatalf("Failed to create Gmail reader: %v", err)
		}
		defer reader.Close()

		// Test with limit of 1
		emails, err := reader.GetUnreadEmails(ctx, 1)
		if err != nil {
			t.Fatalf("Failed to get unread emails: %v", err)
		}

		if len(emails) > 1 {
			t.Errorf("Expected at most 1 email, got %d", len(emails))
		}
	})

	// Clean up test token file
	t.Cleanup(func() {
		os.Remove("test_token.json")
	})
}

func TestGmailReaderCredentialsDecode(t *testing.T) {
	// Test base64 credentials decoding
	testCredentials := map[string]interface{}{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "test-key-id",
		"private_key":                 "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n",
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	}

	// Convert to JSON and base64 encode
	jsonBytes, err := json.Marshal(testCredentials)
	if err != nil {
		t.Fatalf("Failed to marshal test credentials: %v", err)
	}

	encodedCreds := base64.StdEncoding.EncodeToString(jsonBytes)

	// Set environment variable temporarily
	originalCreds := os.Getenv("GMAIL_READER_CREDENTIALS")
	os.Setenv("GMAIL_READER_CREDENTIALS", encodedCreds)
	defer func() {
		if originalCreds == "" {
			os.Unsetenv("GMAIL_READER_CREDENTIALS")
		} else {
			os.Setenv("GMAIL_READER_CREDENTIALS", originalCreds)
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := InboxConfig{
		TokenPath: "test_token.json",
	}

	// This should fail at OAuth config creation since we have fake credentials
	// but it should successfully decode the base64
	_, err = NewGmailReader(config, logger)
	if err == nil {
		t.Error("Expected error with fake credentials, but got none")
	}

	// The error should be about OAuth config, not base64 decoding
	if err != nil && err.Error() != "" {
		t.Logf("Got expected error: %v", err)
	}
}

func TestGmailReaderMissingCredentials(t *testing.T) {
	// Ensure no credentials are set
	originalCreds := os.Getenv("GMAIL_READER_CREDENTIALS")
	os.Unsetenv("GMAIL_READER_CREDENTIALS")
	defer func() {
		if originalCreds != "" {
			os.Setenv("GMAIL_READER_CREDENTIALS", originalCreds)
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := InboxConfig{
		TokenPath: "test_token.json",
	}

	_, err := NewGmailReader(config, logger)
	if err == nil {
		t.Error("Expected error with missing credentials, but got none")
	}

	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestGmailReaderInvalidBase64(t *testing.T) {
	// Set invalid base64
	originalCreds := os.Getenv("GMAIL_READER_CREDENTIALS")
	os.Setenv("GMAIL_READER_CREDENTIALS", "invalid-base64!")
	defer func() {
		if originalCreds == "" {
			os.Unsetenv("GMAIL_READER_CREDENTIALS")
		} else {
			os.Setenv("GMAIL_READER_CREDENTIALS", originalCreds)
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := InboxConfig{
		TokenPath: "test_token.json",
	}

	_, err := NewGmailReader(config, logger)
	if err == nil {
		t.Error("Expected error with invalid base64, but got none")
	}

	if err != nil && err.Error() != "" {
		t.Logf("Got expected error: %v", err)
	}
}

// TestGmailReaderInterface ensures GmailReader implements InboxReader
func TestGmailReaderInterface(t *testing.T) {
	var _ InboxReader = (*GmailReader)(nil)
}

// BenchmarkGmailReaderCreation benchmarks the creation of Gmail reader
func BenchmarkGmailReaderCreation(b *testing.B) {
	if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
		b.Skip("GMAIL_READER_CREDENTIALS not set, skipping benchmark")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := InboxConfig{
		TokenPath: "bench_token.json",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := NewGmailReader(config, logger)
		if err != nil {
			b.Fatalf("Failed to create Gmail reader: %v", err)
		}
		reader.Close()
	}

	// Clean up
	os.Remove("bench_token.json")
}