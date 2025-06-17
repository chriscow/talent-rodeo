package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
	"time"

	"sort"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds/tools"
	"github.com/chriscow/vapi-go"
	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
)

// AppointmentStatus represents the possible states of an appointment
type AppointmentStatus string

const (
	appointmentsCollection = "appointments"
	// top-level collection to track appointments needing processing
	processingAppointmentsCollection = "processing_appointments"

	AppointmentStatusScheduled  AppointmentStatus = "scheduled"
	AppointmentStatusRunning    AppointmentStatus = "running"
	AppointmentStatusProcessing AppointmentStatus = "processing"
	AppointmentStatusCompleted  AppointmentStatus = "completed"
	AppointmentStatusCancelled  AppointmentStatus = "cancelled"
	AppointmentStatusFailed     AppointmentStatus = "failed"
	AppointmentStatusOther      AppointmentStatus = "other"
)

// Appointment represents a scheduled phone interview
type Appointment struct {
	ID            string
	Name          string
	UserID        string
	ScheduledTime time.Time
	Timezone      string
	Status        AppointmentStatus

	// This is the end reason given by VAPI when we get the end of call report
	EndReason *string

	// Error is an internal error that occurred during the appointment
	Error       *string
	CallID      *string
	Summary     *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time
	FailedAt    *time.Time
	Notes       *string

	RecapSent bool // Whether the recap has been sent to the user
}

func CreateAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointment *Appointment) error {
	// Validate user exists first
	_, found, err := GetUser(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}
	if !found {
		return fmt.Errorf("user %s not found", userID)
	}

	if appointment.ID == "" {
		appointment.ID = funcs.Slugify(fmt.Sprintf("%s-%d", appointment.Name, time.Now().UnixNano()))
	}

	appointment.UserID = userID
	appointment.CreatedAt = time.Now()
	appointment.UpdatedAt = appointment.CreatedAt

	_, err = fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	return nil
}

func GetAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointmentID string) (*Appointment, error) {
	doc, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment for user %s: %w", userID, err)
	}

	var appointment Appointment
	if err := doc.DataTo(&appointment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
	}

	return &appointment, nil
}

func GetLatestAppointment(ctx context.Context, fs *FirestoreClient, userID string) (*Appointment, bool, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).OrderBy("CompletedAt", firestore.Desc).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest appointment: %w", err)
	}

	var appt Appointment
	if err := doc.DataTo(&appt); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal appointment data: %w", err)
	}

	return &appt, true, nil
}

func GetLatestCompletedAppointment(ctx context.Context, fs *FirestoreClient, userID string) (*Appointment, bool, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Where("Status", "==", AppointmentStatusCompleted).OrderBy("CompletedAt", firestore.Desc).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest completed appointment: %w", err)
	}

	var appt Appointment
	if err := doc.DataTo(&appt); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal appointment data: %w", err)
	}

	return &appt, true, nil
}

func ListAppointments(ctx context.Context, fs *FirestoreClient, userID string) ([]*Appointment, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Documents(ctx)

	var appointments []*Appointment
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate appointments: %w", err)
		}

		var appointment Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
		}

		appointments = append(appointments, &appointment)
	}

	return appointments, nil
}

// ListCompletedAppointments retrieves all completed appointments for a user
// and returns them sorted by CompletedAt in descending order
func ListCompletedAppointments(ctx context.Context, fs *FirestoreClient, userID string) ([]*Appointment, error) {
	// Create a query for completed appointments where completed_at exists
	query := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).
		Where("Status", "==", AppointmentStatusCompleted).
		Where("CompletedAt", ">", time.Time{}) // Filter for non-nil completed_at values

	iter := query.Documents(ctx)

	var appointments []*Appointment
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate completed appointments: %w", err)
		}

		var appointment Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
		}

		// Double-check that CompletedAt is not nil - this shouldn't happen based on our query
		// but it's good practice to validate
		if appointment.CompletedAt == nil {
			// Log a warning since this indicates data inconsistency
			fmt.Printf(BGYellow("WARNING: Found appointment %s with status 'completed' but nil CompletedAt field\n"), appointment.ID)
			continue
		}

		appointments = append(appointments, &appointment)
	}

	// Sort appointments by CompletedAt field in descending order (newest first)
	sort.SliceStable(appointments, func(i, j int) bool {
		return appointments[i].CompletedAt.After(*appointments[j].CompletedAt)
	})

	return appointments, nil
}

func UpdateAppointment(ctx context.Context, fs *FirestoreClient, appointment *Appointment) error {
	if appointment == nil {
		return fmt.Errorf("appointment cannot be nil")
	}
	if appointment.ID == "" || appointment.UserID == "" {
		return fmt.Errorf("appointment ID and UserID are required")
	}
	_, err := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}
	return nil
}

func UpdateAppointmentTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, appointment *Appointment) error {
	tx.Set(fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID), appointment)
	return nil
}

func CancelAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointmentID string) error {
	// First get the appointment
	appointment, err := GetAppointment(ctx, fs, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to get appointment for cancellation: %w", err)
	}

	// Update status and add a cancellation timestamp
	appointment.Status = AppointmentStatusCancelled
	now := time.Now()
	appointment.CancelledAt = &now
	appointment.UpdatedAt = now

	// Update the full appointment
	if err := UpdateAppointment(ctx, fs, appointment); err != nil {
		return fmt.Errorf("failed to cancel appointment: %w", err)
	}

	return nil
}

func GetAppointmentTranscript(ctx context.Context, fs *FirestoreClient, callID string) (*Transcript, error) {
	// Check if the appointment has a call ID
	if callID == "" {
		return nil, fmt.Errorf("appointment does not have a call ID")
	}

	// Get the call from VAPI
	call, err := vapi.GetCall(ctx, callID)
	if err != nil {
		return nil, fmt.Errorf("failed to get call from VAPI: %w", err)
	}

	return NewTranscriptFromCallResult(*call), nil
}

func DeleteAppointment(ctx context.Context, fs *FirestoreClient, userID, appointmentID string) error {
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}
	return nil
}

type processingAppointment struct {
	UserID        string
	AppointmentID string
	StartedAt     time.Time
}

// When an appointment enters processing state, add a reference to this collection
func ProcessAppointmentStarted(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update the appointment as normal
		appointment.Status = AppointmentStatusProcessing
		appointment.UpdatedAt = time.Now()

		if err := UpdateAppointmentTx(ctx, fs, tx, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		// Add reference to the processing tracking collection
		processingRef := processingAppointment{
			UserID:        appointment.UserID,
			AppointmentID: appointment.ID,
			StartedAt:     time.Now(),
		}

		if err := tx.Set(fs.Collection(processingAppointmentsCollection).Doc(appointment.ID), processingRef); err != nil {
			return fmt.Errorf("failed to add processing reference in transaction: %w", err)
		}

		if err := UpdateUserStatusTx(ctx, fs, tx, user, UserStatusCallProcessing); err != nil {
			return fmt.Errorf("failed to update user state: %w", err)
		}
		return nil
	})
}

// When processing is complete, remove from tracking collection
func ProcessAppointmentComplete(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update appointment status
		appointment.Status = AppointmentStatusCompleted
		appointment.UpdatedAt = time.Now()
		appointment.CompletedAt = &appointment.UpdatedAt

		// Get references to both documents we need to update/delete
		appointmentRef := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID)
		processingRef := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID)

		// Within the transaction, update the appointment and delete the processing reference
		if err := tx.Set(appointmentRef, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		if err := tx.Delete(processingRef); err != nil {
			return fmt.Errorf("failed to delete processing reference in transaction: %w", err)
		}

		// Send completion state update
		if err := UpdateUserStatusTx(ctx, fs, tx, user, UserStatusCallCompleted); err != nil {
			return fmt.Errorf("failed to update user state: %w", err)
		}

		if err := UpdateUserSubscriptionEndAtTx(ctx, fs, tx, user, *appointment.CompletedAt); err != nil {
			return fmt.Errorf("failed to update user subscription end at: %w", err)
		}

		return nil
	})
}

func ProcessAppointmentFailed(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment, err error) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update appointment status
		appointment.Status = AppointmentStatusFailed
		appointment.UpdatedAt = time.Now()
		appointment.FailedAt = &appointment.UpdatedAt

		// Get references to both documents we need to update/delete
		appointmentRef := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID)
		processingRef := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID)
		userRef := fs.Collection(usersCollection).Doc(user.ID)

		// Within the transaction, update the appointment and delete the processing reference
		if err := tx.Set(appointmentRef, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		if err := tx.Delete(processingRef); err != nil {
			return fmt.Errorf("failed to delete processing reference in transaction: %w", err)
		}

		err := err.Error()
		user.State.Error = &err
		if err := tx.Set(userRef, user); err != nil {
			return fmt.Errorf("failed to update user state with error: %w", err)
		}

		return nil
	})
}

// GenerateJobDescription generates an idealized, halucinated job description for an appointment transcript and resume
func GenerateJobDescription(ctx context.Context, resume Resume, appointment Appointment) (string, error) {
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-060-job-description.txt")
	if err != nil {
		return "", fmt.Errorf("failed to read job description prompt: %w", err)
	}

	if appointment.CallID == nil {
		return "", fmt.Errorf("appointment call ID is nil")
	}

	call, err := vapi.GetCall(ctx, *appointment.CallID)
	if err != nil {
		return "", fmt.Errorf("failed to get call: %w", err)
	}

	// remove any system messages
	msgs := call.Artifact.MessagesOpenAIFormatted
	msgs = lo.Filter(msgs, func(msg openai.ChatCompletionMessage, _ int) bool {
		return msg.Role != openai.ChatMessageRoleSystem
	})

	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages found in call")
	}

	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal call messages: %w", err)
	}

	if resume.RawText == "" {
		return "", fmt.Errorf("resume is empty")
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Resume":     resume.RawText,
		"Transcript": string(msgsJSON),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	const maxTokens = 8192
	desc, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WithMaxTokens(maxTokens))
	if err != nil {
		return "", fmt.Errorf("failed to generate job description: %w", err)
	}

	// get the text between <ideal_job_description> and </ideal_job_description> tags
	// Use (?s) flag to make dot match newlines (DOTALL mode)
	re := regexp.MustCompile(`(?s)<ideal_job_description>(.*?)</ideal_job_description>`)
	matches := re.FindStringSubmatch(desc)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return desc, nil
}

// ListAppointmentsNeedingProcessing retrieves all appointments that are currently marked as processing
func ListAppointmentsNeedingProcessing(ctx context.Context, fs *FirestoreClient, logger *slog.Logger) ([]*Appointment, error) {
	var appointments []*Appointment
	var orphanedRefs []string

	// Query the processing tracking collection directly
	iter := fs.Collection(processingAppointmentsCollection).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate processing appointments: %w", err)
		}

		// Parse the processing reference using our struct
		var procRef processingAppointment
		if err := doc.DataTo(&procRef); err != nil {
			logger.Warn(BGYellow("failed to parse processing reference %s: %v\n"), doc.Ref.ID, err)
			orphanedRefs = append(orphanedRefs, doc.Ref.ID)
			continue
		}

		// Get the actual appointment
		appointment, err := GetAppointment(ctx, fs, procRef.UserID, procRef.AppointmentID)
		if err != nil {
			// Log the orphaned reference with more visibility
			logger.Warn(BGYellow("orphaned processing reference found"), "appointmentID", procRef.AppointmentID, "userID", procRef.UserID, "error", err)

			// Add to list of orphaned references to clean up
			orphanedRefs = append(orphanedRefs, doc.Ref.ID)
			continue
		}

		appointments = append(appointments, appointment)
	}

	// Clean up orphaned references synchronously
	if len(orphanedRefs) > 0 {
		logger.Warn("found orphaned processing references that need cleanup", "count", len(orphanedRefs))
		cleanupOrphanedProcessingRefs(ctx, fs, logger, orphanedRefs)
	}

	return appointments, nil
}

// cleanupOrphanedProcessingRefs removes references to appointments that no longer exist
func cleanupOrphanedProcessingRefs(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, refIDs []string) {
	for _, refID := range refIDs {
		_, err := fs.Collection(processingAppointmentsCollection).Doc(refID).Delete(ctx)
		if err != nil {
			logger.Error("failed to clean up orphaned processing reference", "refID", refID, "error", err)
		}
	}
}
