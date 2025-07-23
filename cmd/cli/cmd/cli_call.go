package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/vapi-go"
	"github.com/spf13/cobra"
)

var callCmd = &cobra.Command{
	Use:   "call --user <user-id>",
	Short: "Make an outbound call to a user using VAPI",
	Long: `Make an outbound call to a user using the VAPI assistant.

This command creates an appointment record and initiates an outbound call
using the configured VAPI assistant. The user must exist, have a phone number,
and be in a ready state for calls.

Required environment variables:
  VAPI_ASSISTANT_ID: The assistant ID to use for the call
  VAPI_OUTBOUND_PHONE_ID: The phone number ID for outbound calls
  TWILIO_ACCOUNT_SID: Twilio account SID  
  TWILIO_AUTH_TOKEN: Twilio auth token
  TWILIO_FROM_NUMBER: Twilio phone number to call from
  VAPI_WEBHOOK_URL: Webhook URL for call events

Examples:
  trcli call --user user123                    # Make call to user123
  TESTING_MODE=true trcli call --user user123  # Make simulated call`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate required flags
		userID, _ := cmd.Flags().GetString("user")
		if userID == "" {
			return fmt.Errorf("--user flag is required")
		}

		// Initialize Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Verify all required environment variables are set
		requiredEnvVars := []string{
			"VAPI_ASSISTANT_ID",
			"VAPI_OUTBOUND_PHONE_ID", 
			"VAPI_WEBHOOK_URL",
		}

		// Only require Twilio vars if not in testing mode
		if os.Getenv("TESTING_MODE") != "true" {
			requiredEnvVars = append(requiredEnvVars, 
				"TWILIO_ACCOUNT_SID",
				"TWILIO_AUTH_TOKEN", 
				"TWILIO_FROM_NUMBER",
			)
		}

		for _, envVar := range requiredEnvVars {
			if os.Getenv(envVar) == "" {
				return fmt.Errorf("required environment variable %s is not set", envVar)
			}
		}

		// Get and validate user
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		// Validate user has phone number
		if user.Phone == nil || *user.Phone == "" {
			return fmt.Errorf("user has no phone number: %s", userID)
		}

		// Validate user is not in error state
		if user.Status == internal.UserStatusError {
			userError := ""
			if user.Error != nil {
				userError = *user.Error
			}
			return fmt.Errorf("user is in error state: %s (error: %s)", userID, userError)
		}

		// Validate user is ready for calls
		if user.Status < internal.UserStatusCallScheduled {
			return fmt.Errorf("user is not ready for a call (status: %s)", user.Status)
		}

		fmt.Printf("Making outbound call to user %s (%s) at %s\n", user.Name, userID, *user.Phone)

		// Create appointment record
		appointment := &internal.Appointment{
			UserID:        userID,
			ScheduledTime: time.Now(),
			Status:        internal.AppointmentStatusScheduled,
		}

		// Create appointment in database
		err = internal.CreateAppointment(ctx, fs, userID, appointment)
		if err != nil {
			return fmt.Errorf("failed to create appointment: %w", err)
		}

		fmt.Printf("Created appointment: %s\n", appointment.ID)

		// Build assistant overrides
		assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, fs, nil)
		if err != nil {
			return fmt.Errorf("failed to build assistant overrides: %w", err)
		}

		// Get environment variables
		assistantID := os.Getenv("VAPI_ASSISTANT_ID")

		var result *vapi.Call

		// Handle testing mode vs production
		if os.Getenv("TESTING_MODE") == "true" {
			fmt.Println("Testing mode enabled - simulating call creation")
			result, err = vapi.GetCall(ctx, simulate.TestingCallID)
			if err != nil {
				return fmt.Errorf("failed to get simulated call: %w", err)
			}
			appointment.CallID = result.ID
		} else {
			// Create phone configuration
			phone := vapi.PhoneNumber{
				TwilioAccountSid:  os.Getenv("TWILIO_ACCOUNT_SID"),
				TwilioAuthToken:   os.Getenv("TWILIO_AUTH_TOKEN"),
				TwilioPhoneNumber: os.Getenv("TWILIO_FROM_NUMBER"),
			}

			// Create call configuration
			callConfig := vapi.Call{
				Name:               &appointment.ID,
				AssistantID:        &assistantID,
				PhoneNumber:        &phone,
				AssistantOverrides: assistantOverrides,
				Customer: &vapi.Customer{
					Number: *user.Phone,
					Name:   user.ID,
				},
			}

			fmt.Println("Creating VAPI call...")

			// Create the call
			result, err = vapi.CreateCall(ctx, callConfig)
			if err != nil {
				// Update appointment status to failed
				appointment.Status = internal.AppointmentStatusFailed
				errStr := err.Error()
				appointment.Error = &errStr
				internal.UpdateAppointment(ctx, fs, appointment)
				
				return fmt.Errorf("failed to create VAPI call: %w", err)
			}
		}

		// Validate result
		if result == nil || result.ID == nil || result.Status == nil {
			return fmt.Errorf("unexpected nil result from VAPI call creation")
		}

		fmt.Printf("Call created successfully!\n")
		fmt.Printf("  Call ID: %s\n", *result.ID)
		fmt.Printf("  Status: %s\n", *result.Status)

		// Update appointment and user status in transaction
		err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			// Update appointment
			appointment.Status = internal.AppointmentStatusRunning
			appointment.CallID = result.ID
			if err := internal.UpdateAppointmentTx(ctx, fs, tx, appointment); err != nil {
				return fmt.Errorf("failed to update appointment: %w", err)
			}

			// Update user status
			if err := internal.UpdateUserStatusTx(ctx, fs, tx, user, internal.UserStatusCallStarted); err != nil {
				return fmt.Errorf("failed to update user status: %w", err)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to update appointment and user status: %w", err)
		}

		fmt.Printf("Updated user status to: %s\n", internal.UserStatusCallStarted)

		// In testing mode, schedule simulated end-of-call webhook
		if os.Getenv("TESTING_MODE") == "true" {
			fmt.Println("Testing mode: scheduling simulated end-of-call webhook")
			// Note: This would need access to the task processor, which isn't available in CLI context
			// For now, we'll just log that it would happen
			fmt.Println("(Simulated end-of-call webhook would be scheduled here)")
		}

		if jsonOutput {
			output := map[string]interface{}{
				"success":       true,
				"appointment_id": appointment.ID,
				"call_id":       *result.ID,
				"call_status":   *result.Status,
				"user_id":       userID,
				"user_status":   internal.UserStatusCallStarted,
			}
			
			jsonBytes, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON output: %w", err)
			}
			fmt.Println(string(jsonBytes))
		}

		return nil
	},
}

func init() {
	// Add the call command to root
	RootCmd.AddCommand(callCmd)

	// Add required flag
	callCmd.Flags().StringP("user", "u", "", "User ID to call (required)")
	callCmd.MarkFlagRequired("user")
}