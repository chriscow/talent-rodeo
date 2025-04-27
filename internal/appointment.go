package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"sort"

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

	// This is the end reson given by VAPI when we get the end of call report
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

	// New fields for storing promptLLM responses
	ExtractedData *ExtractedData
}

type ExtractedData struct {
	TranscriptExtract   *string
	ResolvedPreferences *string
	WorkPreferences     *string
	KeywordAnalysis     *string
	UserStatement       *string
}

func CreateAppointment(ctx context.Context, fs *FirestoreClient, name string, user User, appointment *Appointment) error {
	// Validate user exists first
	_, found, err := GetUser(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}
	if !found {
		return fmt.Errorf("user %s not found", user.ID)
	}

	if appointment.Name == "" {
		appointment.Name = name
	}

	if appointment.ID == "" {
		appointment.ID = funcs.Slugify(fmt.Sprintf("%s-%d", appointment.Name, time.Now().UnixNano()))
	}

	if user.Name == "" {
		return fmt.Errorf("user name is required")
	}

	appointment.UserID = user.ID
	appointment.CreatedAt = time.Now()
	appointment.UpdatedAt = appointment.CreatedAt

	_, err = fs.Collection(usersCollection).Doc(user.ID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
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
		Where("status", "==", AppointmentStatusCompleted).
		Where("completed_at", ">", time.Time{}) // Filter for non-nil completed_at values

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
	if len(appointments) > 1 {
		sort.SliceStable(appointments, func(i, j int) bool {
			return appointments[i].CompletedAt.After(*appointments[j].CompletedAt)
		})
	}

	return appointments, nil
}

func UpdateAppointment(ctx context.Context, fs *FirestoreClient, appointment *Appointment) error {
	_, err := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}
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

// MapVapiEndReasonToAppointmentStatus maps a VAPI call end reason to our appointment status
// and returns both the status and whether the reason should be stored in OtherStatus
func MapVapiEndReasonToAppointmentStatus(endReason string) (string, bool) {
	// Successful completions
	// switch endReason {
	// case "vonage-completed",
	// 	"assistant-ended-call",
	// 	"assistant-said-end-call-phrase",
	// 	"assistant-ended-call-with-hangup-task",
	// 	"assistant-ended-call-after-message-spoken":
	// 	return AppointmentStatusCompleted, false
	// }

	// // Customer-initiated endings that we consider completed
	// switch endReason {
	// case "customer-ended-call":
	// 	return AppointmentStatusCompleted, true
	// }

	// // Customer-initiated cancellations
	// switch endReason {
	// case "customer-busy",
	// 	"customer-did-not-answer",
	// 	"customer-did-not-give-microphone-permission",
	// 	"voicemail":
	// 	return AppointmentStatusCancelled, true
	// }

	// // System cancellations
	// switch endReason {
	// case "exceeded-max-duration",
	// 	"manually-canceled",
	// 	"silence-timed-out",
	// 	"assistant-forwarded-call":
	// 	return AppointmentStatusCancelled, true
	// }

	// All other cases are considered failures and should store the specific reason
	return endReason, true
}

func DeleteAppointment(ctx context.Context, fs *FirestoreClient, userID, appointmentID string) error {
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}
	return nil
}

// When an appointment enters processing state, add a reference to this collection
func MarkAppointmentAsProcessing(ctx context.Context, fs *FirestoreClient, appointment *Appointment) error {
	// Update the appointment as normal
	appointment.Status = AppointmentStatusProcessing
	appointment.UpdatedAt = time.Now()

	if err := UpdateAppointment(ctx, fs, appointment); err != nil {
		return err
	}

	// Add reference to the processing tracking collection
	processingRef := map[string]interface{}{
		"UserID":        appointment.UserID,
		"AppointmentID": appointment.ID,
		"StartedAt":     time.Now(),
	}

	_, err := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID).Set(ctx, processingRef)
	return err
}

// When processing is complete, remove from tracking collection
func CompleteProcessing(ctx context.Context, fs *FirestoreClient, appointment *Appointment) error {
	// Update appointment status
	appointment.Status = AppointmentStatusCompleted
	appointment.UpdatedAt = time.Now()
	appointment.CompletedAt = &appointment.UpdatedAt

	if err := UpdateAppointment(ctx, fs, appointment); err != nil {
		return err
	}

	// Remove from processing tracking collection
	_, err := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID).Delete(ctx)
	return err
}

// Replace ListAppointmentsNeedingProcessing with this more efficient version
func ListAppointmentsNeedingProcessing(ctx context.Context, fs *FirestoreClient) ([]*Appointment, error) {
	var appointments []*Appointment

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

		// Extract user and appointment IDs
		data := doc.Data()
		userID, _ := data["UserID"].(string)
		appointmentID, _ := data["AppointmentID"].(string)

		// Get the actual appointment
		appointment, err := GetAppointment(ctx, fs, userID, appointmentID)
		if err != nil {
			// Log but continue if appointment not found or can't be retrieved
			fmt.Printf("Warning: Processing appointment %s for user %s not found: %v\n", appointmentID, userID, err)
			continue
		}

		appointments = append(appointments, appointment)
	}

	return appointments, nil
}
