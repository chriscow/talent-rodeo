package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/cmd/web/transcript"
	"talent-rodeo/internal"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var appointmentCmd = &cobra.Command{
	Use:   "appointment",
	Short: "Manage appointments",
	Long:  `Commands for managing appointments in Talent Rodeo`,
}

var reprocessCmd = &cobra.Command{
	Use:   "reprocess [appointment-id]",
	Short: "Reprocess a completed appointment",
	Long:  `Reprocess a completed appointment by running it through the transcript processor again`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		appointmentID := args[0]

		// Initialize Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		// Initialize logger with debug level
		logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

		// Initialize Sentry hub
		hub := sentry.CurrentHub()

		// Get all users since we don't know which user owns this appointment
		logger.Debug("Listing users...")
		users, err := internal.ListUsers(ctx, fs, 1000, 0)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}
		logger.Debug("Found users", "count", len(users))

		var appointment *internal.Appointment
		var found bool
		var userID string

		// Search for the appointment across all users
		for _, user := range users {
			logger.Debug("Checking user for appointment", "user_id", user.ID, "user_name", user.Name)
			appt, err := internal.GetAppointment(ctx, fs, user.ID, appointmentID)
			if err == nil {
				appointment = appt
				found = true
				userID = user.ID
				logger.Debug("Found appointment", "user_id", user.ID, "user_name", user.Name)
				break
			}
			logger.Debug("Appointment not found for user", "user_id", user.ID, "error", err)
		}

		if !found {
			return fmt.Errorf("appointment %s not found", appointmentID)
		}

		// Process the appointment
		if err := transcript.ProcessCompletedAppointment(hub, fs, logger, appointment); err != nil {
			return fmt.Errorf("failed to process appointment: %w", err)
		}

		fmt.Printf("Started reprocessing appointment %s\n", appointmentID)

		// Poll for completion
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		timeout := time.After(10 * time.Minute)

		for {
			select {
			case <-timeout:
				return fmt.Errorf("timed out waiting for appointment processing to complete")
			case <-ticker.C:
				currentAppt, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
				if err != nil {
					return fmt.Errorf("failed to get appointment status: %w", err)
				}

				logger.Debug("Current appointment status", "status", currentAppt.Status)

				if currentAppt.Status != internal.AppointmentStatusProcessing {
					if currentAppt.Status == internal.AppointmentStatusCompleted {
						fmt.Printf("Successfully reprocessed appointment %s\n", appointmentID)
						return nil
					}
					if currentAppt.Status == internal.AppointmentStatusFailed {
						return fmt.Errorf("appointment processing failed")
					}
					return fmt.Errorf("unexpected appointment status: %s", currentAppt.Status)
				}

				fmt.Printf("Still processing appointment %s...\n", appointmentID)
			}
		}
	},
}

func init() {
	appointmentCmd.AddCommand(reprocessCmd)
	RootCmd.AddCommand(appointmentCmd)
}
