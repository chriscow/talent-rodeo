package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const (
	userJobsCollection = "user_jobs"
)

// UserJob represents a job that a user has interacted with
type UserJob struct {
	// ID is the same as the job_details document ID (provider_job_id)
	ID string `firestore:"id"`

	// Reference to the job_details document
	JobRef *firestore.DocumentRef `firestore:"job_ref"`

	// User interaction metadata
	Status     string     `firestore:"status"` // "new", "viewed", "saved", "applied", "rejected"
	ViewedAt   *time.Time `firestore:"viewed_at,omitempty"`
	SavedAt    *time.Time `firestore:"saved_at,omitempty"`
	AppliedAt  *time.Time `firestore:"applied_at,omitempty"`
	RejectedAt *time.Time `firestore:"rejected_at,omitempty"`
	Notes      string     `firestore:"notes,omitempty"`

	// Search context
	SearchHash string    `firestore:"search_hash"`
	CreatedAt  time.Time `firestore:"created_at"`
	UpdatedAt  time.Time `firestore:"updated_at"`
}

// SaveUserJob saves a job reference for a user
func SaveUserJob(ctx context.Context, fs *firestore.Client, userID string, job *UserJob) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}

	// Set timestamps
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Set default status if not provided
	if job.Status == "" {
		job.Status = "new"
	}

	// Create reference to job_details document if not set
	if job.JobRef == nil {
		job.JobRef = fs.Collection("job_details").Doc(job.ID)
	}

	// Save to user's jobs subcollection
	// Using job.ID as document ID ensures deduplication
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to save user job: %w", err)
	}

	return nil
}

// GetUserJobs retrieves all jobs for a user
func GetUserJobs(ctx context.Context, fs *firestore.Client, userID string) ([]*UserJob, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Documents(ctx)
	var jobs []*UserJob

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user jobs: %w", err)
		}

		var job UserJob
		if err := doc.DataTo(&job); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// UpdateUserJobStatus updates the status and corresponding timestamp of a user's job
func UpdateUserJobStatus(ctx context.Context, fs *firestore.Client, userID, jobID, status string) error {
	now := time.Now()
	updates := []firestore.Update{
		{Path: "status", Value: status},
		{Path: "updated_at", Value: now},
	}

	// Add timestamp based on status
	switch status {
	case "viewed":
		updates = append(updates, firestore.Update{Path: "viewed_at", Value: now})
	case "saved":
		updates = append(updates, firestore.Update{Path: "saved_at", Value: now})
	case "applied":
		updates = append(updates, firestore.Update{Path: "applied_at", Value: now})
	case "rejected":
		updates = append(updates, firestore.Update{Path: "rejected_at", Value: now})
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job status: %w", err)
	}

	return nil
}

// UpdateUserJobNotes updates the notes for a user's job
func UpdateUserJobNotes(ctx context.Context, fs *firestore.Client, userID, jobID, notes string) error {
	updates := []firestore.Update{
		{Path: "notes", Value: notes},
		{Path: "updated_at", Value: time.Now()},
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(jobID).Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to update user job notes: %w", err)
	}

	return nil
}
