package internal

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

type UserStatus int

const (
	UserStatusUnknown UserStatus = iota
	UserStatusError
	UserStatusPendingResumeUpload
	UserStatusProcessingResume
	UserStatusResumeUploaded
	UserStatusIntroCallScheduled
	UserStatusIntroCallStarted
	UserStatusIntroCallCompleted
	UserStatusPreferencesExtracted
	UserStatusJobSearchStarted
	UserStatusJobSearchComplete

	usersCollection = "users"
)

type UserState struct {
	Status  UserStatus `firestore:"status"`
	Started time.Time  `firestore:"started"`
	Error   *string    `firestore:"error"`
}

func UpdateUserStatus(ctx context.Context, fs *firestore.Client, user *User, status UserStatus) error {
	user.State.Status = status
	user.State.Started = time.Now()
	user.State.Error = nil // Clear any previous errors

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"state": user.State,
	})
}

// UpdateUserStateWithError updates the user state to Error and sets the error message
func UpdateUserStateWithError(ctx context.Context, fs *firestore.Client, user *User, errorMsg string) error {
	user.State.Status = UserStatusError
	user.State.Started = time.Now()
	errorCopy := errorMsg // Make a copy to ensure it stays in memory
	user.State.Error = &errorCopy

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"state": user.State,
	})
}

func (s UserStatus) String() string {
	switch s {
	case UserStatusError:
		return "Error"
	case UserStatusUnknown:
		return "Unknown"
	case UserStatusPendingResumeUpload:
		return "Pending Resume Upload"
	case UserStatusProcessingResume:
		return "Processing Resume"
	case UserStatusResumeUploaded:
		return "Resume Uploaded"
	case UserStatusIntroCallScheduled:
		return "Call Scheduled"
	case UserStatusIntroCallStarted:
		return "Call Started"
	case UserStatusIntroCallCompleted:
		return "Call Completed"
	case UserStatusPreferencesExtracted:
		return "Preferences Extracted"
	case UserStatusJobSearchStarted:
		return "Job Search Started"
	case UserStatusJobSearchComplete:
		return "Job Search Complete"
	default:
		return "Unknown Status"
	}
}

func (s UserStatus) Description() string {
	switch s {
	case UserStatusError:
		return "The user is in an error state. Check the error field for more information."
	case UserStatusUnknown:
		return "The user status is unknown."
	case UserStatusPendingResumeUpload:
		return "We are waiting for your resume to be uploaded."
	case UserStatusProcessingResume:
		return "The resume is being processed."
	case UserStatusResumeUploaded:
		return "The resume has been successfully uploaded but a call has not been scheduled yet."
	case UserStatusIntroCallScheduled:
		return "We have your resume and a call has been scheduled."
	case UserStatusIntroCallStarted:
		return "Your introductory call has started."
	case UserStatusIntroCallCompleted:
		return "Your introductory call has been completed."
	case UserStatusPreferencesExtracted:
		return "The user's preferences have been extracted."
	case UserStatusJobSearchStarted:
		return "The job search has started."
	case UserStatusJobSearchComplete:
		return "The job search has been completed."
	default:
		return "No description available for this status."
	}
}
