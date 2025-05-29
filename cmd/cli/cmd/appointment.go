package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"talent-rodeo/internal/transcript"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var completedAppt bool
var sendRecap bool

func init() {
	appointmentCmd.AddCommand(reprocessCmd)
	appointmentCmd.AddCommand(listApptsCmd)
	appointmentCmd.AddCommand(createApptCmd)
	appointmentCmd.AddCommand(latestApptCmd)
	appointmentCmd.AddCommand(sendRecapCmd)
	RootCmd.AddCommand(appointmentCmd)

	createApptCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to create the appointment for")
	createApptCmd.MarkFlagRequired("user")

	listApptsCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output in JSON format")
	latestApptCmd.Flags().BoolVarP(&completedAppt, "completed", "c", false, "Only show completed appointments")

	reprocessCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to reprocess the appointment for")
	reprocessCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID to reprocess")
	reprocessCmd.MarkFlagRequired("appointment")
	reprocessCmd.MarkFlagRequired("user")
	reprocessCmd.Flags().BoolVarP(&sendRecap, "send-recap", "r", false, "Send a recap email after reprocessing the appointment")

	sendRecapCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID for the appointment")
	sendRecapCmd.Flags().StringVarP(&appointmentID, "appointment", "a", "", "Appointment ID to send recap email for")
	sendRecapCmd.MarkFlagRequired("appointment")
	sendRecapCmd.MarkFlagRequired("user")
}

// Good appointment ids: 230bfc29-2ba0-42b8-90e9-f2288b96965f (Vijaya), 2b0a2620-5117-402d-ba13-daf6e501d8be (Vijaya original)
// 2709318a-1b74-44da-9ba9-e1973804b6b9 (Chris)

type jobEventHandler struct {
	userID   string
	dispatch *internal.JobEventDispatch
	wg       *sync.WaitGroup
}

func (u *jobEventHandler) OnSearchStarted(task internal.SearchTask) {
	fmt.Println("userNotifier:OnSearchStarted", task)
}

func (u *jobEventHandler) OnJobListing(task internal.SearchTask, listing internal.JobListing) {
	fmt.Println("userNotifier:OnJobListing", task, listing)
}

func (u *jobEventHandler) OnJobDetail(task internal.SearchTask, details internal.JobDetail) {
	fmt.Println("userNotifier:OnJobDetail", task, details)
}

func (u *jobEventHandler) OnPageComplete(task internal.SearchTask, page int, jobsFound int, errors []error) {
	fmt.Printf("userNotifier:OnPageComplete page=%d jobs=%d errors=%d\n", page, jobsFound, len(errors))
}

func (u *jobEventHandler) OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error) {
	fmt.Println("userNotifier:OnSearchCompleted", task, totalJobs, errors)
	u.dispatch.Deregister(u.userID)
	u.wg.Done()
}

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
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if userID == "" {
			return fmt.Errorf("user ID is required")
		}
		if appointmentID == "" {
			return fmt.Errorf("appointment ID is required")
		}

		fmt.Printf("Started reprocessing appointment %s\n", appointmentID)

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

		appointment, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
		if err != nil {
			return fmt.Errorf("failed to get appointment: %w", err)
		}

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found")
		}

		dispatch := internal.NewJobEventDispatch()
		dispatch.Register(userID, internal.NewSearchResultProcessor(fs, logger))
		queue := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"), logger, scrapingdog.WithJobEventHandler(dispatch))

		var cfg *transcript.SMTPConfig
		if sendRecap {
			portStr := os.Getenv("SMTP_PORT")
			if portStr == "" {
				portStr = "587" // default SMTP port
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid SMTP_PORT environment variable: %w", err)
			}
			cfg = &transcript.SMTPConfig{
				Host:     os.Getenv("SMTP_HOST"),
				Port:     port,
				Username: os.Getenv("SMTP_USERNAME"),
				Password: os.Getenv("SMTP_PASSWORD"),
				From:     os.Getenv("SMTP_FROM"),
			}
		}

		transcript.ProcessCompletedCall(hub, fs, nil, queue, logger, dispatch, user, appointment, cfg)

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
		return nil
	},
}

var sendRecapCmd = &cobra.Command{
	Use:   "send-recap",
	Short: "Send a recap email for a completed appointment",
	Long:  `Send a recap email for a completed appointment using the appointment transcript`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if userID == "" {
			return fmt.Errorf("user ID is required")
		}
		if appointmentID == "" {
			return fmt.Errorf("appointment ID is required")
		}

		// Initialize Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		// Get the appointment
		appointment, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
		if err != nil {
			return fmt.Errorf("failed to get appointment: %w", err)
		}

		// Get the user
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found")
		}

		// Get the transcript
		appointmentTranscript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment)
		if err != nil {
			return fmt.Errorf("failed to get appointment transcript: %w", err)
		}

		// Configure SMTP from environment variables
		portStr := os.Getenv("SMTP_PORT")
		if portStr == "" {
			portStr = "587" // default SMTP port
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid SMTP_PORT environment variable: %w", err)
		}

		cfg := transcript.SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     port,
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
		}

		// Validate required SMTP configuration
		if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" || cfg.From == "" {
			return fmt.Errorf("missing required SMTP configuration. Please set SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD, and SMTP_FROM environment variables")
		}

		// Generate and send the recap email
		fmt.Printf("Sending recap email for appointment %s to %s...\n", appointmentID, user.Email)
		err = transcript.GenerateRecapEmail(ctx, fs, user, appointmentTranscript, cfg)
		if err != nil {
			return fmt.Errorf("failed to generate and send recap email: %w", err)
		}

		fmt.Printf("Successfully sent recap email for appointment %s\n", appointmentID)
		return nil
	},
}
