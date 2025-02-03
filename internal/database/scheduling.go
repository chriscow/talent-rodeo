package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
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
	ID                int               `db:"id"`
	UserID            int               `db:"user_id"`
	ScheduledTime     time.Time         `db:"scheduled_time"`
	Timezone          string            `db:"timezone"`
	Status            AppointmentStatus `db:"status"`
	CallSID           *string           `db:"call_sid"`
	Transcript        *string           `db:"transcript"`
	TranscriptSummary *string           `db:"transcript_summary"`
	CreatedAt         time.Time         `db:"created_at"`
	UpdatedAt         time.Time         `db:"updated_at"`
}

// SchedulingPreferences represents a user's preferred interview times
type SchedulingPreferences struct {
	ID                 int       `db:"id"`
	UserID             int       `db:"user_id"`
	Timezone           string    `db:"timezone"`
	PreferredDays      []string  `db:"-"` // Handled separately due to JSON storage
	PreferredDaysJSON  string    `db:"preferred_days"`
	PreferredTimeStart string    `db:"preferred_time_start"`
	PreferredTimeEnd   string    `db:"preferred_time_end"`
	CreatedAt          time.Time `db:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"`
}

// JobSearchCriteria represents a user's job preferences
type JobSearchCriteria struct {
	ID            int       `db:"id"`
	UserID        int       `db:"user_id"`
	CriteriaType  string    `db:"criteria_type"`
	CriteriaValue string    `db:"criteria_value"`
	Priority      int       `db:"priority"`
	Source        string    `db:"source"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// CreateAppointment creates a new appointment
func (db *DB) CreateAppointment(appointment *Appointment) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `
		INSERT INTO appointments (
			user_id, scheduled_time, timezone, status
		) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	err := db.QueryRowContext(ctx, query,
		appointment.UserID,
		appointment.ScheduledTime,
		appointment.Timezone,
		appointment.Status,
	).Scan(&appointment.ID, &appointment.CreatedAt, &appointment.UpdatedAt)

	return err
}

// GetAppointment retrieves an appointment by ID
func (db *DB) GetAppointment(id int) (*Appointment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var appointment Appointment
	query := `SELECT * FROM appointments WHERE id = $1`
	err := db.GetContext(ctx, &appointment, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &appointment, err
}

// UpdateAppointmentStatus updates the status of an appointment
func (db *DB) UpdateAppointmentStatus(id int, status AppointmentStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `UPDATE appointments SET status = $1 WHERE id = $2`
	_, err := db.ExecContext(ctx, query, status, id)
	return err
}

// UpdateAppointmentTranscript updates the transcript and summary for an appointment
func (db *DB) UpdateAppointmentTranscript(id int, transcript, summary string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `
		UPDATE appointments 
		SET transcript = $1, transcript_summary = $2 
		WHERE id = $3`
	_, err := db.ExecContext(ctx, query, transcript, summary, id)
	return err
}

// GetUserAppointments retrieves all appointments for a user
func (db *DB) GetUserAppointments(userID int) ([]Appointment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var appointments []Appointment
	query := `SELECT * FROM appointments WHERE user_id = $1 ORDER BY scheduled_time DESC`
	err := db.SelectContext(ctx, &appointments, query, userID)
	return appointments, err
}

// UpsertSchedulingPreferences creates or updates a user's scheduling preferences
func (db *DB) UpsertSchedulingPreferences(prefs *SchedulingPreferences) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Convert preferred days to JSON
	daysJSON, err := json.Marshal(prefs.PreferredDays)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred days: %w", err)
	}
	prefs.PreferredDaysJSON = string(daysJSON)

	query := `
		INSERT INTO scheduling_preferences (
			user_id, timezone, preferred_days, preferred_time_start, preferred_time_end
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			timezone = EXCLUDED.timezone,
			preferred_days = EXCLUDED.preferred_days,
			preferred_time_start = EXCLUDED.preferred_time_start,
			preferred_time_end = EXCLUDED.preferred_time_end
		RETURNING id, created_at, updated_at`

	err = db.QueryRowContext(ctx, query,
		prefs.UserID,
		prefs.Timezone,
		prefs.PreferredDaysJSON,
		prefs.PreferredTimeStart,
		prefs.PreferredTimeEnd,
	).Scan(&prefs.ID, &prefs.CreatedAt, &prefs.UpdatedAt)

	return err
}

// GetSchedulingPreferences retrieves a user's scheduling preferences
func (db *DB) GetSchedulingPreferences(userID int) (*SchedulingPreferences, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var prefs SchedulingPreferences
	query := `SELECT * FROM scheduling_preferences WHERE user_id = $1`
	err := db.GetContext(ctx, &prefs, query, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse JSON preferred days
	if prefs.PreferredDaysJSON != "" {
		err = json.Unmarshal([]byte(prefs.PreferredDaysJSON), &prefs.PreferredDays)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal preferred days: %w", err)
		}
	}

	return &prefs, nil
}

// CreateJobSearchCriteria adds a new job search criterion for a user
func (db *DB) CreateJobSearchCriteria(criteria *JobSearchCriteria) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `
		INSERT INTO job_search_criteria (
			user_id, criteria_type, criteria_value, priority, source
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err := db.QueryRowContext(ctx, query,
		criteria.UserID,
		criteria.CriteriaType,
		criteria.CriteriaValue,
		criteria.Priority,
		criteria.Source,
	).Scan(&criteria.ID, &criteria.CreatedAt, &criteria.UpdatedAt)

	return err
}

// GetUserJobSearchCriteria retrieves all job search criteria for a user
func (db *DB) GetUserJobSearchCriteria(userID int) ([]JobSearchCriteria, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var criteria []JobSearchCriteria
	query := `
		SELECT * FROM job_search_criteria 
		WHERE user_id = $1 
		ORDER BY priority DESC, created_at DESC`
	err := db.SelectContext(ctx, &criteria, query, userID)
	return criteria, err
}

// DeleteJobSearchCriteria removes a job search criterion
func (db *DB) DeleteJobSearchCriteria(id, userID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `DELETE FROM job_search_criteria WHERE id = $1 AND user_id = $2`
	result, err := db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("no job search criteria found with id %d for user %d", id, userID)
	}

	return nil
}
