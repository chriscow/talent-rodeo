package cmd

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"talent-rodeo/internal/inbox"

	"github.com/spf13/cobra"
)

var (
	gmailTokenPath string
	gmailMaxEmails int
)

var gmailCmd = &cobra.Command{
	Use:   "gmail",
	Short: "Gmail integration commands",
	Long:  `Commands for setting up and testing Gmail integration`,
}

var gmailSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up Gmail OAuth authorization",
	Long: `Initialize Gmail OAuth authorization by walking through the authorization flow.
This will create a token file that can be used for subsequent Gmail operations.

Requires GMAIL_READER_CREDENTIALS environment variable to be set with base64-encoded
Google Cloud service account credentials.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
			return fmt.Errorf("GMAIL_READER_CREDENTIALS environment variable is required")
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))

		config := inbox.InboxConfig{
			TokenPath: gmailTokenPath,
		}

		fmt.Println("=== Gmail OAuth Setup ===")
		fmt.Printf("Token will be saved to: %s\n", gmailTokenPath)
		fmt.Println()

		// Check if token already exists
		if _, err := os.Stat(gmailTokenPath); err == nil {
			fmt.Printf("🔍 Found existing token at %s\n", gmailTokenPath)
			fmt.Println("📋 Testing existing token...")

			// Test the existing token
			reader, err := inbox.NewGmailReader(config, logger)
			if err == nil {
				// Try to fetch emails to test the token
				ctx := context.Background()
				_, err = reader.GetUnreadEmails(ctx, 1)
				reader.Close()

				if err == nil {
					fmt.Println("✅ Existing token is valid and working!")
					fmt.Println("🎉 No setup needed. You can use 'gmail test' to verify.")
					return nil
				} else if strings.Contains(err.Error(), "invalid_grant") {
					fmt.Println("❌ Existing token has expired or been revoked.")
					fmt.Println("🗑️  I need to delete the old token and create a new one.")
					fmt.Print("❓ Delete the old token and continue? (y/N): ")

					var response string
					fmt.Scanln(&response)
					if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
						return fmt.Errorf("setup cancelled by user")
					}

					fmt.Println("🗑️  Deleting old token...")
					if err := os.Remove(gmailTokenPath); err != nil {
						return fmt.Errorf("failed to delete old token: %w", err)
					}
					fmt.Println("✅ Old token deleted.")
				} else {
					fmt.Printf("❌ Error testing existing token: %v\n", err)
					fmt.Println("🗑️  The existing token appears to be corrupted or invalid.")
					fmt.Print("❓ Delete the corrupted token and create a new one? (y/N): ")

					var response string
					fmt.Scanln(&response)
					if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
						return fmt.Errorf("setup cancelled by user")
					}

					fmt.Println("🗑️  Deleting corrupted token...")
					if err := os.Remove(gmailTokenPath); err != nil {
						return fmt.Errorf("failed to delete corrupted token: %w", err)
					}
					fmt.Println("✅ Corrupted token deleted.")
				}
			} else {
				fmt.Printf("❌ Error loading existing token: %v\n", err)
				fmt.Println("🗑️  The token file appears to be corrupted or incomplete.")
				fmt.Print("❓ Delete the corrupted token file and create a new one? (y/N): ")

				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					return fmt.Errorf("setup cancelled by user")
				}

				fmt.Println("🗑️  Deleting corrupted token file...")
				if err := os.Remove(gmailTokenPath); err != nil {
					return fmt.Errorf("failed to delete corrupted token file: %w", err)
				}
				fmt.Println("✅ Corrupted token file deleted.")
			}
			fmt.Println()
		}

		// Create new token through OAuth flow
		fmt.Println("🚀 Starting OAuth authorization flow...")
		fmt.Println("📝 This will open a browser window for you to authorize the application.")
		fmt.Println()

		reader, err := inbox.NewGmailReader(config, logger)
		if err != nil {
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		// Verify the token was actually created
		if _, err := os.Stat(gmailTokenPath); os.IsNotExist(err) {
			return fmt.Errorf("❌ OAuth flow did not complete successfully - token file was not created.\n" +
				"This usually means:\n" +
				"1. You didn't complete the browser authorization\n" +
				"2. You didn't paste the authorization code\n" +
				"3. There was a network error\n" +
				"Please try running 'gmail setup' again.")
		}

		// Test the new token by trying to fetch emails
		fmt.Println("🧪 Testing new token...")
		ctx := context.Background()
		_, err = reader.GetUnreadEmails(ctx, 1)
		if err != nil {
			// Delete the bad token so user doesn't get confused
			os.Remove(gmailTokenPath)
			return fmt.Errorf("❌ New token test failed: %w\n"+
				"The token was created but doesn't work properly.\n"+
				"I've deleted the bad token. Please try running 'gmail setup' again.", err)
		}

		fmt.Println("✅ Gmail OAuth setup complete!")
		fmt.Printf("💾 Token saved to: %s\n", gmailTokenPath)
		fmt.Println()
		fmt.Println("🎉 You can now use 'gmail test' to verify the setup.")
		fmt.Println("💡 Use 'gmail list --json' to see your unread emails in JSON format.")

		// Offer to update env files and deploy
		if err := offerTokenUpdates(gmailTokenPath); err != nil {
			fmt.Printf("⚠️  Warning: %v\n", err)
		}

		return nil
	},
}

// offerTokenUpdates offers to update env files and deploy to Fly.io
func offerTokenUpdates(tokenPath string) error {
	// Read and encode the token
	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("failed to read token file: %w", err)
	}

	encodedToken := base64.StdEncoding.EncodeToString(tokenData)

	fmt.Println()
	fmt.Println("🔧 Additional Setup Options:")
	fmt.Println()

	// Offer to update .env file
	if _, err := os.Stat(".env"); err == nil {
		fmt.Print("📝 Update .env file with GMAIL_TOKEN_BASE64? (y/N): ")
		if getUserConfirmation() {
			if err := updateEnvFile(".env", "GMAIL_TOKEN_BASE64", encodedToken); err != nil {
				fmt.Printf("❌ Failed to update .env: %v\n", err)
			} else {
				fmt.Println("✅ Updated .env file")
			}
		}
	}

	// Offer to update .prod.env file
	if _, err := os.Stat(".prod.env"); err == nil {
		fmt.Print("📝 Update .prod.env file with GMAIL_TOKEN_BASE64? (y/N): ")
		if getUserConfirmation() {
			if err := updateEnvFile(".prod.env", "GMAIL_TOKEN_BASE64", encodedToken); err != nil {
				fmt.Printf("❌ Failed to update .prod.env: %v\n", err)
			} else {
				fmt.Println("✅ Updated .prod.env file")
			}
		}
	}

	// Offer to deploy to Fly.io
	if isFlyAvailable() {
		fmt.Print("🚀 Deploy GMAIL_TOKEN_BASE64 to Fly.io? (y/N): ")
		if getUserConfirmation() {
			if err := deployToFly("GMAIL_TOKEN_BASE64", encodedToken); err != nil {
				fmt.Printf("❌ Failed to deploy to Fly.io: %v\n", err)
			} else {
				fmt.Println("✅ Deployed to Fly.io")
			}
		}
	}

	return nil
}

// getUserConfirmation gets user confirmation for y/N prompts
func getUserConfirmation() bool {
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// updateEnvFile updates an environment file with a new variable value
func updateEnvFile(filename, key, value string) error {
	// Read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filename, err)
	}

	lines := strings.Split(string(content), "\n")
	keyPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\s*=`)
	found := false

	// Update existing line or add new one
	for i, line := range lines {
		if keyPattern.MatchString(line) {
			lines[i] = fmt.Sprintf(`%s="%s"`, key, value)
			found = true
			break
		}
	}

	if !found {
		// Add new line before the last line (if it's empty) or at the end
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = append(lines[:len(lines)-1], fmt.Sprintf(`%s="%s"`, key, value), "")
		} else {
			lines = append(lines, fmt.Sprintf(`%s="%s"`, key, value))
		}
	}

	// Write back to file
	newContent := strings.Join(lines, "\n")
	return os.WriteFile(filename, []byte(newContent), 0644)
}

// isFlyAvailable checks if the fly CLI is available
func isFlyAvailable() bool {
	_, err := exec.LookPath("fly")
	return err == nil
}

// deployToFly deploys a secret to Fly.io
func deployToFly(key, value string) error {
	cmd := exec.Command("fly", "secrets", "set", fmt.Sprintf("%s=%s", key, value))

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("fly command failed: %w\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}

	return nil
}

var gmailTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test Gmail connection and fetch unread emails",
	Long: `Test the Gmail connection by fetching a few unread emails.
Requires Gmail OAuth to be set up first using 'gmail setup'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
			return fmt.Errorf("GMAIL_READER_CREDENTIALS environment variable is required")
		}

		// Check if token file exists
		if _, err := os.Stat(gmailTokenPath); os.IsNotExist(err) {
			return fmt.Errorf("Gmail token file not found at %s. Run 'gmail setup' first", gmailTokenPath)
		}

		// Set up logger with appropriate level
		logLevel := slog.LevelInfo
		if verbose {
			logLevel = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		}))

		config := inbox.InboxConfig{
			TokenPath: gmailTokenPath,
		}

		fmt.Println("=== Gmail Connection Test ===")
		fmt.Printf("Using token file: %s\n", gmailTokenPath)
		fmt.Printf("Fetching up to %d unread emails...\n", gmailMaxEmails)
		fmt.Println()

		reader, err := inbox.NewGmailReader(config, logger)
		if err != nil {
			// Show the full error for debugging
			if verbose {
				fmt.Printf("Detailed error: %v\n", err)
			}
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		ctx := context.Background()
		emails, err := reader.GetUnreadEmails(ctx, gmailMaxEmails)
		if err != nil {
			// Show the full error for debugging
			if verbose {
				fmt.Printf("Detailed error: %v\n", err)
			}

			// Check for common OAuth errors and provide helpful messages
			if strings.Contains(err.Error(), "invalid_grant") {
				return fmt.Errorf("Gmail token has expired or been revoked. Run 'gmail setup' to get a new token.\nOriginal error: %w", err)
			}
			return fmt.Errorf("failed to get unread emails: %w", err)
		}

		fmt.Printf("📧 Found %d unread emails:\n", len(emails))

		if len(emails) == 0 {
			fmt.Println("   No unread emails found.")
		} else {
			for i, email := range emails {
				fmt.Printf("\n%d. From: %s\n", i+1, email.From)
				fmt.Printf("   Subject: %s\n", email.Subject)
				fmt.Printf("   Date: %s\n", email.Date.Format("2006-01-02 15:04:05"))
				fmt.Printf("   ID: %s\n", email.ID)
				fmt.Printf("   Labels: %v\n", email.Labels)
			}
		}

		fmt.Println("\n✅ Gmail connection test complete!")
		return nil
	},
}

var gmailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List unread emails in JSON format",
	Long: `List unread emails from Gmail in JSON format.
Useful for integration with other tools or scripts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
			return fmt.Errorf("GMAIL_READER_CREDENTIALS environment variable is required")
		}

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn, // Only show warnings/errors on stderr
		}))

		config := inbox.InboxConfig{
			TokenPath: gmailTokenPath,
		}

		reader, err := inbox.NewGmailReader(config, logger)
		if err != nil {
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		ctx := context.Background()
		emails, err := reader.GetUnreadEmails(ctx, gmailMaxEmails)
		if err != nil {
			return fmt.Errorf("failed to get unread emails: %w", err)
		}

		// Output JSON to stdout
		if jsonOutput {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(emails)
		} else {
			// Simple text output
			for _, email := range emails {
				fmt.Printf("ID: %s | From: %s | Subject: %s | Date: %s\n",
					email.ID, email.From, email.Subject, email.Date.Format("2006-01-02 15:04:05"))
			}
		}

		return nil
	},
}

var gmailListAllCmd = &cobra.Command{
	Use:   "list-all",
	Short: "List all emails (read and unread) in JSON format",
	Long: `List all emails from Gmail in JSON format, including both read and unread emails.
Useful for checking historical emails that might contain search adjustment requests.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
			return fmt.Errorf("GMAIL_READER_CREDENTIALS environment variable is required")
		}

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn, // Only show warnings/errors on stderr
		}))

		config := inbox.InboxConfig{
			TokenPath: gmailTokenPath,
		}

		reader, err := inbox.NewGmailReader(config, logger)
		if err != nil {
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		ctx := context.Background()
		emails, err := reader.GetAllEmails(ctx, gmailMaxEmails)
		if err != nil {
			return fmt.Errorf("failed to get all emails: %w", err)
		}

		// Output JSON to stdout
		if jsonOutput {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(emails)
		} else {
			// Simple text output
			for _, email := range emails {
				fmt.Printf("ID: %s | From: %s | Subject: %s | Date: %s\n",
					email.ID, email.From, email.Subject, email.Date.Format("2006-01-02 15:04:05"))
			}
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(gmailCmd)

	gmailCmd.AddCommand(gmailSetupCmd)
	gmailCmd.AddCommand(gmailTestCmd)
	gmailCmd.AddCommand(gmailListCmd)
	gmailCmd.AddCommand(gmailListAllCmd)

	// Flags for all gmail commands
	gmailCmd.PersistentFlags().StringVar(&gmailTokenPath, "token", "gmail_token.json", "Path to Gmail OAuth token file")

	// Flags for test and list commands
	gmailTestCmd.Flags().IntVar(&gmailMaxEmails, "max", 5, "Maximum number of emails to fetch")
	gmailListCmd.Flags().IntVar(&gmailMaxEmails, "max", 10, "Maximum number of emails to fetch")
	gmailListAllCmd.Flags().IntVar(&gmailMaxEmails, "max", 20, "Maximum number of emails to fetch")
}
