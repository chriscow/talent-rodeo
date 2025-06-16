package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/transcript"

	"github.com/spf13/cobra"
)

var (
	userID        string
	appointmentID string
	resumeID      string
	saveFlag      bool
)

func init() {
	RootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(genJobDescCmd)
	generateCmd.AddCommand(genSteeringQueryCmd)
	generateCmd.AddCommand(genStatementCmd)

	genJobDescCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
	genJobDescCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID")
	genJobDescCmd.Flags().StringVarP(&resumeID, "resume", "r", "", "Resume ID")

	genSteeringQueryCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
	genSteeringQueryCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID")
	genSteeringQueryCmd.Flags().BoolVar(&saveFlag, "save", false, "Save the steering query to the database")
	genSteeringQueryCmd.Flags().BoolVar(&allUsers, "all", false, "Generate steering queries for all users")

	genStatementCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
}

var generateCmd = &cobra.Command{
	Use:     "generate",
	Short:   "Use an LLM to generate various artifacts",
	Long:    `Use an LLM to generate various artifacts`,
	Aliases: []string{"gen"},
}

var genJobDescCmd = &cobra.Command{
	Use:   "job-desc --user <userID> --appointment <appointmentID> --resume <resumeID>",
	Short: "Generate a job description for a completed appointment using a resume.",
	Long: `Generate a fictitious job description for a completed appointment.

This command uses a user's resume and appointment data to generate a job description using an LLM.
All three flags --user, --appointment, and --resume are required.

Note: The appointment must have status 'completed'.`,
	Example: `
  # Generate a job description using long flags
  go run ./cmd/cli generate job-desc --user 1234abcd --appointment abcd5678 --resume 9876wxyz

  # Generate a job description using short flags
  go run ./cmd/cli generate job-desc -u 1234abcd -a abcd5678 -r 9876wxyz
    `,
	Run: func(cmd *cobra.Command, args []string) {
		if userID == "" {
			log.Fatalf("--user flag is required")
		}
		if appointmentID == "" {
			log.Fatalf("--appointment flag is required")
		}
		if resumeID == "" {
			log.Fatalf("--resume flag is required")
		}
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			log.Fatalf("failed to create firestore client: %v", err)
		}
		defer fs.Close()

		//
		// Get the user, appointment, and resume
		//
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			log.Fatalf("Error getting user: %v", err)
		}
		if !found {
			log.Fatalf("User not found: %s", userID)
		}
		appointment, err := internal.GetAppointment(ctx, fs, user.ID, appointmentID)
		if err != nil {
			log.Fatalf("Error getting appointment: %v", err)
		}
		if appointment.Status != internal.AppointmentStatusCompleted {
			log.Fatalf("Appointment is not completed: %s", appointmentID)
		}
		resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
		if err != nil {
			log.Fatalf("Error getting resume: %v", err)
		}
		if !found {
			log.Fatalf("Resume not found: %s", resumeID)
		}

		jobDesc, err := internal.GenerateJobDescription(ctx, *resume, *appointment)
		if err != nil {
			log.Fatalf("Error generating job description: %v", err)
		}

		fmt.Println(jobDesc)
	},
}

var genSteeringQueryCmd = &cobra.Command{
	Use:     "steering-query --user <userID> --appointment <appointmentID> [--save]",
	Aliases: []string{"query"},
	Short:   "Generate a steering query for a completed appointment.",
	Long:    `Generate a steering query for a completed appointment.`,
	Example: `
  # Generate a steering query using long flags
  go run ./cmd/cli generate steering-query --user 1234abcd --appointment abcd5678 --save

  # Generate a steering query using short flags
  go run ./cmd/cli generate steering-query -u 1234abcd -a abcd5678 -s
    `,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			log.Fatalf("failed to create firestore client: %v", err)
		}
		defer fs.Close()

		if allUsers {
			users, err := internal.ListUsers(ctx, fs, 0, 0)
			if err != nil {
				log.Fatalf("failed to list users: %v", err)
			}
			for _, user := range users {
				appt, found, err := internal.GetLatestCompletedAppointment(ctx, fs, user.ID)
				if err != nil {
					fmt.Printf("[SKIP] Error getting latest completed appointment for user %s: %v\n", user.ID, err)
					continue
				}
				if !found {
					fmt.Printf("[SKIP] No completed appointment for user %s\n", user.ID)
					continue
				}
				if appt.CallID == nil {
					fmt.Printf("[SKIP] Appointment for user %s has no CallID\n", user.ID)
					continue
				}
				transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appt.CallID)
				if err != nil {
					fmt.Printf("[SKIP] Error getting transcript for user %s: %v\n", user.ID, err)
					continue
				}
				resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
				if err != nil {
					fmt.Printf("[SKIP] Error getting resume for user %s: %v\n", user.ID, err)
					continue
				}
				if !found {
					fmt.Printf("[SKIP] No resume found for user %s\n", user.ID)
					continue
				}
				q, err := internal.GenerateSteeringQuery(ctx, *resume, *transcript)
				if err != nil {
					fmt.Printf("[SKIP] Error generating steering query for user %s: %v\n", user.ID, err)
					continue
				}
				if saveFlag {
					if err := internal.SetUserSteeringQuery(ctx, fs, user.ID, *q); err != nil {
						fmt.Printf("[SKIP] Error saving steering query for user %s: %v\n", user.ID, err)
						continue
					}
					fmt.Printf("[USER %s] Steering query saved to database\n", user.ID)
				} else {
					qJSON, err := json.MarshalIndent(q, "", "  ")
					if err != nil {
						fmt.Printf("[SKIP] Error marshalling steering query for user %s: %v\n", user.ID, err)
						continue
					}
					fmt.Printf("[USER %s] Steering Query:\n%s\n", user.ID, string(qJSON))
					fmt.Println()
				}
			}
			return
		}

		// Single user logic (original)
		if userID == "" {
			log.Fatalf("--user flag is required")
		}
		if appointmentID == "" {
			log.Fatalf("--appointment flag is required")
		}

		appointment, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
		if err != nil {
			log.Fatalf("Error getting appointment: %v", err)
		}
		if appointment.Status != internal.AppointmentStatusCompleted {
			log.Fatalf("Appointment is not completed: %s", appointmentID)
		}

		transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment.CallID)
		if err != nil {
			log.Fatalf("Error getting appointment transcript: %v", err)
		}

		resume, found, err := internal.GetLatestResume(ctx, fs, userID)
		if err != nil {
			log.Fatalf("Error getting resume: %v", err)
		}
		if !found {
			log.Fatalf("Resume not found for user: %s", userID)
		}

		q, err := internal.GenerateSteeringQuery(ctx, *resume, *transcript)
		if err != nil {
			log.Fatalf("Error generating search queries: %v", err)
		}

		qJSON, err := json.MarshalIndent(q, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling search queries to JSON: %v", err)
		}
		fmt.Println(string(qJSON))

		if saveFlag {
			internal.SetUserSteeringQuery(ctx, fs, userID, *q)
			fmt.Println("Steering query saved to database")
		}
	},
}

var genStatementCmd = &cobra.Command{
	Use:   "statement --user <userID>",
	Short: "Generate a user statement for the latest completed appointment.",
	Long:  `Generate a user statement for the latest completed appointment for a user using the transcript and LLM.`,
	Run: func(cmd *cobra.Command, args []string) {
		if userID == "" {
			log.Fatalf("--user flag is required")
		}
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			log.Fatalf("failed to create firestore client: %v", err)
		}
		defer fs.Close()

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			log.Fatalf("Error getting user: %v", err)
		}
		if !found {
			log.Fatalf("User not found: %s", userID)
		}

		appointment, found, err := internal.GetLatestCompletedAppointment(ctx, fs, userID)
		if err != nil {
			log.Fatalf("Error getting latest completed appointment: %v", err)
		}
		if !found {
			log.Fatalf("No completed appointment found for user: %s", userID)
		}
		if appointment.CallID == nil || *appointment.CallID == "" {
			log.Fatalf("Appointment has no CallID")
		}

		ts, err := internal.GetAppointmentTranscript(ctx, fs, *appointment.CallID)
		if err != nil {
			log.Fatalf("Error getting appointment transcript: %v", err)
		}

		// Use a stub logger for CLI
		stubLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		err = transcript.GetUserStatement(ctx, user, ts, fs, stubLogger)
		if err != nil {
			log.Fatalf("Error generating user statement: %v", err)
		}
		fmt.Println("User statement generated and saved to user profile.")
	},
}
