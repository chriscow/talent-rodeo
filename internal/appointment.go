package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const (
	appointmentsCollection = "appointments"
)

// AppointmentStatus represents the possible states of an appointment
type AppointmentStatus string

const (
	AppointmentStatusScheduled AppointmentStatus = "scheduled"
	AppointmentStatusRunning   AppointmentStatus = "running"
	AppointmentStatusCompleted AppointmentStatus = "completed"
	AppointmentStatusCancelled AppointmentStatus = "cancelled"
	AppointmentStatusFailed    AppointmentStatus = "failed"
	AppointmentStatusOther     AppointmentStatus = "other"
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
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	var appointment Appointment
	if err := doc.DataTo(&appointment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
	}

	return &appointment, nil
}

func GetAppointments(ctx context.Context, fs *firestore.Client, userID string) ([]*Appointment, error) {
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
