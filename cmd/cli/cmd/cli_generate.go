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
	Use:   "generate",
	Short: "Generate AI content from user data and transcripts",
	Long: `Generate various AI-powered content using LLMs and user data.

This command group uses large language models to generate content based on
user resumes, appointment transcripts, and other data. Useful for creating
test data, analyzing user preferences, and generating personalized content.

Generated content includes:
- Job descriptions based on user qualifications
- Steering queries to improve job matching
- User statements summarizing preferences`,
	Aliases: []string{"gen"},
}

var genJobDescCmd = &cobra.Command{
	Use:   "job-desc --user <user-id> --appointment <appointment-id> --resume <resume-id>",
	Short: "Generate a fictitious job description tailored to a user",
	Long: `Generate a fictitious job description based on a user's resume and appointment data.

This command uses AI to create a realistic job description that matches the
user's qualifications and preferences discussed in their appointment. Useful
for testing, demos, or creating sample job postings.

The appointment must have "completed" status for this command to work.

Required flags:
  --user, -u: User ID who owns the resume and appointment
  --appointment, -a: Completed appointment ID to base description on
  --resume, -r: Resume ID to use for qualification matching

Examples:
  trcli generate job-desc --user user123 --appointment appt456 --resume resume789`,
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

		jobDesc, err := internal.GenerateJobDescription(ctx, fs, slog.Default(), user.ID, *resume, *appointment)
		if err != nil {
			log.Fatalf("Error generating job description: %v", err)
		}

		fmt.Println(jobDesc)
	},
}

var genSteeringQueryCmd = &cobra.Command{
	Use:     "steering-query [--user <user-id> --appointment <appointment-id>] [--all] [--save]",
	Aliases: []string{"query"},
	Short:   "Generate steering queries to improve job matching for users",
	Long: `Generate steering queries based on appointment transcripts and resumes.

Steering queries are AI-generated search terms and preferences that improve
job matching accuracy. They're derived from what users say they want during
their appointment calls and their resume content.

Options:
  Single user: --user + --appointment to generate for specific appointment
  All users: --all to generate for all users' latest completed appointments
  
Optional flags:
  --save: Save the generated steering query to the database

Examples:
  # Generate steering query for specific user/appointment
  trcli generate steering-query --user user123 --appointment appt456
  
  # Generate and save to database
  trcli generate steering-query --user user123 --appointment appt456 --save
  
  # Generate steering queries for all users (bulk operation)
  trcli generate steering-query --all --save`,
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
	Use:   "statement --user <user-id>",
	Short: "Generate a user statement summarizing their preferences",
	Long: `Generate a user statement from their latest completed appointment transcript.

The user statement is an AI-generated summary of what the user said about
their job preferences, career goals, and requirements during their call.
This statement is used to improve job matching and personalization.

The statement is automatically saved to the user's profile.

Examples:
  trcli generate statement --user user123            # Generate from latest appointment`,
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
