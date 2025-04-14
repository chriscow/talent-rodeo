package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"sort"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// AppointmentStatus represents the possible states of an appointment
type AppointmentStatus string

const (
	appointmentsCollection = "appointments"

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
	ID                string            `firestore:"id"`
	Name              string            `firestore:"name"`
	UserID            string            `firestore:"user_id"`
	ScheduledTime     time.Time         `firestore:"scheduled_time"`
	Timezone          string            `firestore:"timezone"`
	Status            AppointmentStatus `firestore:"status"`
	OtherStatus       *string           `firestore:"other_status"`
	CallID            *string           `firestore:"call_id"`
	TranscriptSummary *string           `firestore:"transcript_summary"`
	CreatedAt         time.Time         `firestore:"created_at"`
	UpdatedAt         time.Time         `firestore:"updated_at"`
	CompletedAt       *time.Time        `firestore:"completed_at"`
	CancelledAt       *time.Time        `firestore:"cancelled_at"`
	FailedAt          *time.Time        `firestore:"failed_at"`
	Notes             *string           `firestore:"notes"`

	// New fields for storing promptLLM responses
	ExtractedData *ExtractedData `firestore:"extracted_data"`
}

type ExtractedData struct {
	TranscriptExtract   *string `firestore:"01_transcript_extracted_data"`
	ResolvedPreferences *string `firestore:"02_resolved_preferences"`
	WorkPreferences     *string `firestore:"03_work_preferences"`
	KeywordAnalysis     *string `firestore:"04_keyword_analysis"`
	UserStatement       *string `firestore:"05_user_statement"`
}

func CreateAppointment(ctx context.Context, fs *firestore.Client, name string, user User, appointment *Appointment) error {
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

func GetAppointment(ctx context.Context, fs *firestore.Client, userID string, appointmentID string) (*Appointment, error) {
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

func ListAppointments(ctx context.Context, fs *firestore.Client, userID string) ([]*Appointment, error) {
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
func ListCompletedAppointments(ctx context.Context, fs *firestore.Client, userID string) ([]*Appointment, error) {
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

func UpdateAppointment(ctx context.Context, fs *firestore.Client, appointment *Appointment) error {
	_, err := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}
	return nil
}

func CancelAppointment(ctx context.Context, fs *firestore.Client, userID string, appointmentID string) error {
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Update(ctx, []firestore.Update{
		{Path: "status", Value: AppointmentStatusCancelled},
	})
	if err != nil {
		return fmt.Errorf("failed to cancel appointment: %w", err)
	}

	return nil
}

// MapVapiEndReasonToAppointmentStatus maps a VAPI call end reason to our appointment status
// and returns both the status and whether the reason should be stored in OtherStatus
func MapVapiEndReasonToAppointmentStatus(endReason string) (AppointmentStatus, bool) {
	// Successful completions
	switch endReason {
	case "vonage-completed",
		"assistant-ended-call",
		"assistant-said-end-call-phrase",
		"assistant-ended-call-with-hangup-task",
		"assistant-ended-call-after-message-spoken":
		return AppointmentStatusCompleted, false
	}

	// Customer-initiated endings that we consider completed
	switch endReason {
	case "customer-ended-call":
		return AppointmentStatusCompleted, true
	}

	// Customer-initiated cancellations
	switch endReason {
	case "customer-busy",
		"customer-did-not-answer",
		"customer-did-not-give-microphone-permission",
		"voicemail":
		return AppointmentStatusCancelled, true
	}

	// System cancellations
	switch endReason {
	case "exceeded-max-duration",
		"manually-canceled",
		"silence-timed-out",
		"assistant-forwarded-call":
		return AppointmentStatusCancelled, true
	}

	// All other cases are considered failures and should store the specific reason
	return AppointmentStatusFailed, true
}

func DeleteAppointment(ctx context.Context, fs *firestore.Client, userID, appointmentID string) error {
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}
	return nil
}

// ListAppointmentsNeedingProcessing retrieves all appointments in the "processing" state
// These are appointments that were interrupted during transcript processing
func ListAppointmentsNeedingProcessing(ctx context.Context, fs *firestore.Client) ([]*Appointment, error) {
	var appointments []*Appointment

	// Query all users
	usersIter := fs.Collection(usersCollection).Documents(ctx)
	for {
		userDoc, err := usersIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate users: %w", err)
		}

		// For each user, query their appointments in "processing" state
		query := userDoc.Ref.Collection(appointmentsCollection).
			Where("status", "==", AppointmentStatusProcessing)

		apptIter := query.Documents(ctx)
		for {
			doc, err := apptIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to iterate appointments for user %s: %w", userDoc.Ref.ID, err)
			}

			var appointment Appointment
			if err := doc.DataTo(&appointment); err != nil {
				return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
			}

			appointments = append(appointments, &appointment)
		}
	}

	return appointments, nil
}
