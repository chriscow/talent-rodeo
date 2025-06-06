package inbox

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestNewInboxReader(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("CreateGmailReader", func(t *testing.T) {
		config := InboxConfig{
			CredentialsJSON: []byte(`{"type": "service_account"}`),
			TokenPath:       "test_token.json",
		}

		// This will fail due to invalid credentials, but should attempt Gmail reader
		reader, err := NewInboxReader(config, logger)
		if err == nil {
			t.Error("Expected error with invalid credentials")
			if reader != nil {
				reader.Close()
			}
		}
	})

	t.Run("CreateIMAPReader", func(t *testing.T) {
		config := InboxConfig{
			Host:     "imap.gmail.com",
			Port:     993,
			Username: "test@example.com",
			Password: "password",
			UseTLS:   true,
		}

		reader, err := NewInboxReader(config, logger)
		if err != nil {
			t.Errorf("Failed to create IMAP reader: %v", err)
		}
		if reader != nil {
			reader.Close()
		}
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		config := InboxConfig{}

		reader, err := NewInboxReader(config, logger)
		if err == nil {
			t.Error("Expected error with empty config")
			if reader != nil {
				reader.Close()
			}
		}

		expectedErr := "invalid inbox configuration: missing required fields"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})
}

func TestInboxConfig(t *testing.T) {
	t.Run("GmailConfig", func(t *testing.T) {
		config := InboxConfig{
			CredentialsJSON: []byte(`{"client_id": "test"}`),
			TokenPath:       "/tmp/token.json",
		}

		if config.CredentialsJSON == nil {
			t.Error("CredentialsJSON should not be nil")
		}
		if config.TokenPath == "" {
			t.Error("TokenPath should not be empty")
		}
	})

	t.Run("IMAPConfig", func(t *testing.T) {
		config := InboxConfig{
			Host:     "imap.example.com",
			Port:     993,
			Username: "user@example.com",
			Password: "secret",
			UseTLS:   true,
		}

		if config.Host == "" {
			t.Error("Host should not be empty")
		}
		if config.Port == 0 {
			t.Error("Port should not be zero")
		}
		if config.Username == "" {
			t.Error("Username should not be empty")
		}
		if config.Password == "" {
			t.Error("Password should not be empty")
		}
		if !config.UseTLS {
			t.Error("UseTLS should be true")
		}
	})
}

// TestInboxReaderInterface ensures our interface is properly defined
func TestInboxReaderInterface(t *testing.T) {
	// This test just ensures the interface compiles correctly
	var reader InboxReader

	if reader != nil {
		ctx := context.Background()
		_, err := reader.GetUnreadEmails(ctx, 10)
		if err == nil {
			t.Log("Interface method exists")
		}

		err = reader.MarkAsRead(ctx, "email-id")
		if err == nil {
			t.Log("Interface method exists")
		}

		err = reader.Close()
		if err == nil {
			t.Log("Interface method exists")
		}
	}
}