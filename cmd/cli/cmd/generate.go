package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

var (
	userID        string
	appointmentID string
	resumeID      string
)

func init() {
	RootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(genJobDescCmd)
	generateCmd.AddCommand(genSearchQueriesCmd)

	genJobDescCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
	genJobDescCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID")
	genJobDescCmd.Flags().StringVarP(&resumeID, "resume", "r", "", "Resume ID")

	genSearchQueriesCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
	genSearchQueriesCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID")
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
var genSearchQueriesCmd = &cobra.Command{
	Use:   "queries --user <userID> --appointment <appointmentID>",
	Short: "Generate search queries for a completed appointment.",
	Long:  `Generate search queries for a completed appointment.`,
	Run: func(cmd *cobra.Command, args []string) {
		if userID == "" {
			log.Fatalf("--user flag is required")
		}
		if appointmentID == "" {
			log.Fatalf("--appointment flag is required")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			log.Fatalf("failed to create firestore client: %v", err)
		}
		defer fs.Close()

		appointment, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
		if err != nil {
			log.Fatalf("Error getting appointment: %v", err)
		}
		if appointment.Status != internal.AppointmentStatusCompleted {
			log.Fatalf("Appointment is not completed: %s", appointmentID)
		}

		transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment)
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

		p, err := internal.NewPineconeClient()
		if err != nil {
			log.Fatalf("failed to create pinecone client: %v", err)
		}

		_, err = q.GetDenseVector(ctx, p, internal.PineconeEmbedDense)
		if err != nil {
			log.Fatalf("Error getting dense vector: %v", err)
		}
	},
}
