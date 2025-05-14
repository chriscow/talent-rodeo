package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/transcript"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var completedAppt bool

func init() {
	appointmentCmd.AddCommand(reprocessCmd)
	appointmentCmd.AddCommand(listApptsCmd)
	appointmentCmd.AddCommand(createApptCmd)
	appointmentCmd.AddCommand(latestApptCmd)
	RootCmd.AddCommand(appointmentCmd)

	createApptCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to create the appointment for")
	createApptCmd.MarkFlagRequired("user")

	listApptsCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output in JSON format")
	latestApptCmd.Flags().BoolVarP(&completedAppt, "completed", "c", false, "Only show completed appointments")
}

// Good appointment ids: 230bfc29-2ba0-42b8-90e9-f2288b96965f (Vijaya), 2b0a2620-5117-402d-ba13-daf6e501d8be (Vijaya original)
// 2709318a-1b74-44da-9ba9-e1973804b6b9 (Chris)

var appointmentCmd = &cobra.Command{
	Use:     "appointment",
	Short:   "Manage appointments",
	Long:    `Commands for managing appointments in Talent Rodeo`,
	Aliases: []string{"appt"},
}

var latestApptCmd = &cobra.Command{
	Use:   "latest [user-id]",
	Short: "Get the latest appointment for a user",
	Long: `Get the latest appointment for a user.
	
If the --completed flag is provided, only completed appointments are considered.
Otherwise, all appointments are considered.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		var found bool
		var appt *internal.Appointment
		if completedAppt {
			appt, found, err = internal.GetLatestCompletedAppointment(ctx, fs, args[0])
		} else {
			appt, found, err = internal.GetLatestAppointment(ctx, fs, args[0])
		}
		if err != nil {
			return fmt.Errorf("failed to get latest appointment: %w", err)
		}
		if !found {
			return fmt.Errorf("no appointment found for user %s", args[0])
		}
		if jsonOutput {
			output, err := json.MarshalIndent(appt, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal appointment: %w", err)
			}
			fmt.Println(string(output))
		} else {
			fmt.Println(appt.ID)
		}
		return nil
	},
}

var listApptsCmd = &cobra.Command{
	Use:   "list",
	Short: "List appointments for a user",
	Long:  `List all appointments for a user by ID`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		appts, err := internal.ListAppointments(ctx, fs, args[0])
		if err != nil {
			return fmt.Errorf("failed to list appointments: %w", err)
		}

		if jsonOutput {
			output, err := json.MarshalIndent(appts, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal appointments: %w", err)
			}
			fmt.Println(string(output))
		} else {
			for _, appt := range appts {
				fmt.Println(appt.ID)
			}
		}
		return nil
	},
}

var createApptCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an appointment",
	Long: `Create an appointment for a user from JSON data

The argument is a JSON file for an appointment.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if userID == "" {
			return fmt.Errorf("user ID is required")
		}
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		// validate the user ID is a valid UUID
		if _, err := uuid.Parse(userID); err != nil {
			return fmt.Errorf("invalid user ID: %w", err)
		}

		// the argument is a JSON string for an appointment
		var appt internal.Appointment
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read appointment file: %w", err)
		}
		if err := json.Unmarshal(data, &appt); err != nil {
			return fmt.Errorf("failed to unmarshal appointment: %w", err)
		}
		appt.UserID = userID
		if err := internal.CreateAppointment(ctx, fs, userID, &appt); err != nil {
			return fmt.Errorf("failed to create appointment: %w", err)
		}
		fmt.Printf("Created appointment %s for user %s\n", appt.ID, userID)
		return nil
	},
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
		transcript.ProcessCompletedAppointment(hub, fs, logger, appointment)

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
