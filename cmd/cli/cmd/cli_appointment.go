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
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/transcript"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/chriscow/vapi-go"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var completedAppt bool
var sendRecap bool
var allUsers bool

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
	reprocessCmd.Flags().BoolVarP(&sendRecap, "send-recap", "r", false, "Send a recap email after reprocessing the appointment")
	reprocessCmd.Flags().BoolVarP(&allUsers, "all", "A", false, "Reprocess the latest completed appointment for all users")

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

func (u *jobEventHandler) OnJobListing(task internal.SearchTask, listing internal.JobListing) bool {
	fmt.Println("userNotifier:OnJobListing", task, listing)
	return true // always return true to indicate we want to fetch details
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
	Use:   "appointment",
	Short: "Manage user appointments and call processing",
	Long: `Manage user appointments, call processing, and transcript analysis.

Appointments represent scheduled or completed calls between users and recruiters.
This command group allows you to:
- List and view user appointments
- Create appointments from JSON data
- Reprocess completed calls to update job matches
- Send recap emails with call summaries`,
	Aliases: []string{"appt"},
}

var latestApptCmd = &cobra.Command{
	Use:   "latest <user-id>",
	Short: "Get the latest appointment for a user",
	Long: `Get the latest appointment for a user, optionally filtering to completed appointments only.

This is useful for checking appointment status, getting call IDs, or finding
the most recent appointment to reprocess.

Examples:
  trcli appointment latest user123                    # Get latest appointment (any status)
  trcli appointment latest user123 --completed        # Get latest completed appointment only
  trcli appointment latest user123 --json             # Get full appointment details in JSON`,
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

		call, err := vapi.GetCall(ctx, *appt.CallID)
		if err != nil {
			return fmt.Errorf("failed to get call: %w", err)
		}
		fmt.Println(call)
		return nil
	},
}

var listApptsCmd = &cobra.Command{
	Use:   "list <user-id>",
	Short: "List all appointments for a user",
	Long: `List all appointments for a user in chronological order.

Shows appointment IDs, statuses, and timing information to help track
a user's call history and appointment progression.

Examples:
  trcli appointment list user123                      # List appointment IDs
  trcli appointment list user123 --json               # List full appointment details`,
	Args: cobra.ExactArgs(1),
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
	Use:   "create <json-file> --user <user-id>",
	Short: "Create appointments from JSON file",
	Long: `Create one or more appointments for a user from a JSON file.

The JSON file should contain an array of appointment objects. This is typically
used for importing appointment data or creating test appointments.

Required flags:
  --user, -u: User ID to create appointments for

Examples:
  trcli appointment create appointments.json --user user123
  
JSON file format:
  [
    {
      "id": "appt-123",
      "status": "scheduled",
      "scheduled_time": "2024-01-15T10:00:00Z"
    }
  ]`,
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
		appts := []internal.Appointment{}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read appointment file: %w", err)
		}
		if err := json.Unmarshal(data, &appts); err != nil {
			return fmt.Errorf("failed to unmarshal appointment: %w", err)
		}
		for _, appt := range appts {
			appt.UserID = userID
			if err := internal.CreateAppointment(ctx, fs, userID, &appt); err != nil {
				return fmt.Errorf("failed to create appointment: %w", err)
			}
			fmt.Printf("Created appointment %s for user %s\n", appt.ID, userID)
		}
		return nil
	},
}

var reprocessCmd = &cobra.Command{
	Use:   "reprocess [--user <user-id>] [--appointment <appointment-id>] [--all]",
	Short: "Reprocess completed appointments to update job matches",
	Long: `Reprocess completed appointments by re-running transcript analysis and job matching.

This command re-analyzes call transcripts to extract user preferences, update
steering queries, and find new job matches based on the conversation. Useful
when the job matching algorithm has been improved or when you want to refresh
a user's job recommendations.

Options:
  --user + --appointment: Reprocess specific appointment
  --user only: Reprocess latest completed appointment for user  
  --all: Reprocess latest completed appointment for all users (bulk operation)

Examples:
  # Reprocess specific appointment
  trcli appointment reprocess --user user123 --appointment appt-456
  
  # Reprocess latest completed appointment for user
  trcli appointment reprocess --user user123
  
  # Reprocess and send recap email
  trcli appointment reprocess --user user123 --send-recap
  
  # Bulk reprocess all users (use with caution)
  trcli appointment reprocess --all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if allUsers {
			// Process all users in parallel
			fs, err := internal.NewFirestoreClient(ctx)
			if err != nil {
				return fmt.Errorf("failed to create firestore client: %w", err)
			}
			defer fs.Close()

			users, err := internal.ListUsers(ctx, fs, 1000, 0)
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}
			var wg sync.WaitGroup
			bar := pb.StartNew(len(users))
			defer bar.Finish()
			for _, user := range users {
				wg.Add(1)
				go func(user *internal.User) {
					defer wg.Done()
					// fmt.Printf("[ALL] Processing user %s (%s)\n", user.ID, user.Email)
					// Get latest appointment for this user
					appt, found, err := internal.GetLatestAppointment(ctx, fs, user.ID)
					if err != nil {
						// fmt.Printf("[ALL] Error getting latest appointment for user %s: %v\n", user.ID, err)
						bar.Increment()
						return
					}
					if !found {
						// fmt.Printf("[ALL] No appointment found for user %s\n", user.ID)
						bar.Increment()
						return
					}
					// Reuse the reprocessing logic for a single appointment
					tmpFile, err := os.CreateTemp("", "appt-reprocess-*.log")
					if err != nil {
						// fmt.Printf("[ALL] Error creating temp log file: %v\n", err)
						bar.Increment()
						return
					}
					defer tmpFile.Close()

					logger := slog.New(tint.NewHandler(tmpFile, &tint.Options{Level: slog.LevelDebug}))
					hub := sentry.CurrentHub()
					dispatch := internal.NewJobEventDispatch()

					srp := internal.NewSearchResultProcessor(fs, logger, nil, dispatch)
					dispatch.Register(user.ID, srp)
					srp.Start()
					defer srp.Stop()

					queue := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"), logger, scrapingdog.WithJobEventHandler(dispatch))
					transcript.ProcessCompletedCall(hub, fs, nil, queue, logger, dispatch, user, appt, nil)
					time.Sleep(5 * time.Second)
					currentAppt, err := internal.GetAppointment(ctx, fs, user.ID, appt.ID)
					if err != nil {
						// fmt.Printf("[ALL] Error getting appointment status for user %s: %v\n", user.ID, err)
						bar.Increment()
						return
					}
					if currentAppt.Status == internal.AppointmentStatusCompleted {
						bar.Increment()
					} else if currentAppt.Status == internal.AppointmentStatusFailed {
						fmt.Printf("[ALL] Appointment processing failed for user %s\n", user.ID)
						bar.Increment()
					} else {
						// still processing
					}
				}(user)
			}
			wg.Wait()
			return nil
		}

		if userID == "" {
			return fmt.Errorf("user ID is required")
		}

		var cfg *smtp.Config
		if sendRecap {
			portStr := os.Getenv("SMTP_PORT")
			if portStr == "" {
				portStr = "587" // default SMTP port
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid SMTP_PORT environment variable: %w", err)
			}

			if os.Getenv("SMTP_HOST") == "" {
				return fmt.Errorf("SMTP_HOST environment variable is required")
			}
			if os.Getenv("SMTP_USERNAME") == "" {
				return fmt.Errorf("SMTP_USERNAME environment variable is required")
			}
			if os.Getenv("SMTP_PASSWORD") == "" {
				return fmt.Errorf("SMTP_PASSWORD environment variable is required")
			}
			if os.Getenv("SMTP_FROM") == "" {
				return fmt.Errorf("SMTP_FROM environment variable is required")
			}
			cfg = &smtp.Config{
				Host:     os.Getenv("SMTP_HOST"),
				Port:     port,
				Username: os.Getenv("SMTP_USERNAME"),
				Password: os.Getenv("SMTP_PASSWORD"),
				From:     os.Getenv("SMTP_FROM"),
			}
		}

		fmt.Printf("Started reprocessing appointment %s\n", appointmentID)

		// Initialize Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		// Initialize logger with debug level
		// os.MkdirAll(".logs", 0755)
		// tmpFile, err := os.OpenFile(".logs/appt-reprocess.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		// if err != nil {
		// 	return fmt.Errorf("failed to create temp log file: %w", err)
		// }
		// defer tmpFile.Close()

		// logger := slog.New(tint.NewHandler(tmpFile, &tint.Options{Level: slog.LevelDebug}))
		logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

		// Initialize Sentry hub
		hub := sentry.CurrentHub()

		var appointment *internal.Appointment
		if appointmentID != "" {
			appointment, err = internal.GetAppointment(ctx, fs, userID, appointmentID)
			if err != nil {
				return fmt.Errorf("failed to get appointment: %w", err)
			}
		} else {
			var found bool
			appointment, found, err = internal.GetLatestCompletedAppointment(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get latest completed appointment: %w", err)
			}

			if !found {
				return fmt.Errorf("no completed appointment found for user %s", userID)
			}

			appointmentID = appointment.ID
		}

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found")
		}

		dispatch := internal.NewJobEventDispatch()
		dispatch.Register(userID, internal.NewSearchResultProcessor(fs, logger, nil, dispatch))
		queue := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"), logger, scrapingdog.WithJobEventHandler(dispatch))

		transcript.ProcessCompletedCall(hub, fs, nil, queue, logger, dispatch, user, appointment, cfg)
		time.Sleep(5 * time.Second) // Wait for processing to start

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
	Use:   "send-recap --user <user-id> --appointment <appointment-id>",
	Short: "Send recap email for a completed appointment",
	Long: `Generate and send a recap email summarizing a completed appointment.

The recap email includes key points from the call transcript, job recommendations
discussed, and next steps. Requires SMTP configuration via environment variables.

Required flags:
  --user, -u: User ID for the appointment
  --appointment, -a: Appointment ID to send recap for

Required environment variables:
  SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM

Examples:
  trcli appointment send-recap --user user123 --appointment appt-456`,
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
		appointmentTranscript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment.CallID)
		if err != nil {
			return fmt.Errorf("failed to get appointment transcript: %w", err)
		}

		host := os.Getenv("SMTP_HOST")
		if host == "" {
			host = "smtp.gmail.com"
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

		cfg := smtp.Config{
			Host:     host,
			Port:     port,
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
		}

		// Validate required SMTP configuration
		if cfg.Host == "" {
			return fmt.Errorf("SMTP_HOST environment variable is required")
		}

		if cfg.Username == "" {
			return fmt.Errorf("SMTP_USERNAME environment variable is required")
		}

		if cfg.Password == "" {
			return fmt.Errorf("SMTP_PASSWORD environment variable is required")
		}

		if cfg.From == "" {
			return fmt.Errorf("SMTP_FROM environment variable is required")
		}
		if cfg.Port <= 0 {
			return fmt.Errorf("SMTP_PORT environment variable must be a valid port number")
		}

		logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

		// Generate and send the recap email
		fmt.Printf("Sending recap email for appointment %s to %s...\n", appointmentID, user.Email)
		err = transcript.GenerateRecapEmail(ctx, fs, user, appointmentTranscript, cfg, logger)
		if err != nil {
			return fmt.Errorf("failed to generate and send recap email: %w", err)
		}

		fmt.Printf("Successfully sent recap email for appointment %s\n", appointmentID)
		return nil
	},
}
