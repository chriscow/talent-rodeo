package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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

		reader, err := inbox.NewGmailReader(config, logger)
		if err != nil {
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		fmt.Println("✅ Gmail OAuth setup complete!")
		fmt.Printf("Token saved to: %s\n", gmailTokenPath)
		fmt.Println()
		fmt.Println("You can now use 'gmail test' to verify the setup.")

		return nil
	},
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

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
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
			return fmt.Errorf("failed to create Gmail reader: %w", err)
		}
		defer reader.Close()

		ctx := context.Background()
		emails, err := reader.GetUnreadEmails(ctx, gmailMaxEmails)
		if err != nil {
			// Check for common OAuth errors and provide helpful messages
			if strings.Contains(err.Error(), "invalid_grant") {
				return fmt.Errorf("Gmail token has expired or been revoked. Run 'gmail setup' to get a new token")
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

func init() {
	RootCmd.AddCommand(gmailCmd)
	
	gmailCmd.AddCommand(gmailSetupCmd)
	gmailCmd.AddCommand(gmailTestCmd)
	gmailCmd.AddCommand(gmailListCmd)

	// Flags for all gmail commands
	gmailCmd.PersistentFlags().StringVar(&gmailTokenPath, "token", "gmail_token.json", "Path to Gmail OAuth token file")
	
	// Flags for test and list commands
	gmailTestCmd.Flags().IntVar(&gmailMaxEmails, "max", 5, "Maximum number of emails to fetch")
	gmailListCmd.Flags().IntVar(&gmailMaxEmails, "max", 10, "Maximum number of emails to fetch")
}