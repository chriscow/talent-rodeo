package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

const (
	appointmentsCollection = "appointments"
)

// AppointmentStatus represents the possible states of an appointment
type AppointmentStatus string

const (
	AppointmentStatusScheduled AppointmentStatus = "scheduled"
	AppointmentStatusCompleted AppointmentStatus = "completed"
	AppointmentStatusCancelled AppointmentStatus = "cancelled"
	AppointmentStatusFailed    AppointmentStatus = "failed"
)

// Appointment represents a scheduled phone interview
type Appointment struct {
	ID                string            `firestore:"id"`
	UserID            string            `firestore:"user_id"`
	ScheduledTime     time.Time         `firestore:"scheduled_time"`
	Timezone          string            `firestore:"timezone"`
	Status            AppointmentStatus `firestore:"status"`
	CallSID           *string           `firestore:"call_sid"`
	Transcript        *string           `firestore:"transcript"`
	TranscriptSummary *string           `firestore:"transcript_summary"`
	CreatedAt         time.Time         `firestore:"created_at"`
	UpdatedAt         time.Time         `firestore:"updated_at"`
}

func CreateAppointment(ctx context.Context, fs *firestore.Client, user User, appointment *Appointment) error {
	if appointment.ID == "" {
		appointment.ID = funcs.Slugify(fmt.Sprintf("%s %s", user.Name, uuid.New().String()))
	}

	if user.Name == "" {
		return fmt.Errorf("user name is required")
	}

	appointment.ID = funcs.Slugify(fmt.Sprintf("%s %s", user.Name, uuid.New().String()))

	appointment.UserID = user.ID
	appointment.CreatedAt = time.Now()
	appointment.UpdatedAt = appointment.CreatedAt

	_, err := fs.Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	return nil
}

func GetAppointment(ctx context.Context, fs *firestore.Client, appointmentID string) (*Appointment, error) {
	doc, err := fs.Collection(appointmentsCollection).Doc(appointmentID).Get(ctx)
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
	iter := fs.Collection(appointmentsCollection).Where("user_id", "==", userID).Documents(ctx)

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
	_, err := fs.Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}
	return nil
}

func CancelAppointment(ctx context.Context, fs *firestore.Client, appointmentID string) error {
	_, err := fs.Collection(appointmentsCollection).Doc(appointmentID).Update(ctx, []firestore.Update{
		{Path: "status", Value: AppointmentStatusCancelled},
	})
	if err != nil {
		return fmt.Errorf("failed to cancel appointment: %w", err)
	}

	return nil
}
