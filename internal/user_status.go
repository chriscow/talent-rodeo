package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

type UserStatus int

const (
	UserStatusUnknown             UserStatus = iota // 0
	UserStatusError                                 // 1
	UserStatusPendingResumeUpload                   // 2
	UserStatusProcessingResume                      // 3
	UserStatusResumeUploaded                        // 4
	UserStatusCallScheduled                         // 5
	UserStatusCallStarted                           // 6
	UserStatusCallAnalysisStarted                   // 7
	UserStatusCallProcessing                        // 8
	UserStatusCallCompleted                         // 9
	UserStatusJobSearchScheduled                    // 10
	UserStatusJobSearchStarted                      // 11
	UserStatusJobSearchComplete                     // 12
)

// type UserState struct {
// 	// Status  UserStatus
// 	// Started time.Time
// 	// Error   *string // Pointer to allow nil value for no error
// }

func UpdateUserStatus(ctx context.Context, fs *FirestoreClient, user *User, status UserStatus) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}
	user.Status = status
	user.StatusUpdatedAt = time.Now()
	user.Error = nil // Clear any previous errors

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"Status":          status,
		"StatusUpdatedAt": user.StatusUpdatedAt,
		"Error":           nil,
	})
}

func UpdateUserStatusTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, user *User, status UserStatus) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}
	user.Status = status
	user.StatusUpdatedAt = time.Now()
	user.Error = nil // Clear any previous errors

	return UpdateUserTx(ctx, fs, tx, user.ID, map[string]any{
		"Status":          status,
		"StatusUpdatedAt": user.StatusUpdatedAt,
		"Error":           nil,
	})
}

// UpdateUserStateWithError updates the user state to Error and sets the error message
func UpdateUserStateWithError(ctx context.Context, fs *FirestoreClient, user *User, errorMsg string) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}
	user.Status = UserStatusError
	user.StatusUpdatedAt = time.Now()
	errorCopy := errorMsg // Make a copy to ensure it stays in memory
	user.Error = &errorCopy

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"Status":          UserStatusError,
		"Error":           errorCopy,
		"StatusUpdatedAt": user.StatusUpdatedAt,
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
	case UserStatusCallScheduled:
		return "Call Scheduled"
	case UserStatusCallStarted:
		return "Call Started"
	case UserStatusCallAnalysisStarted:
		return "Call Analysis Started"
	case UserStatusCallProcessing:
		return "Call Processing"
	case UserStatusCallCompleted:
		return "Call Completed"
	case UserStatusJobSearchScheduled:
		return "Job Search Scheduled"
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
	case UserStatusCallScheduled:
		return "We have your resume and a call has been scheduled."
	case UserStatusCallStarted:
		return "Your call has started."
	case UserStatusCallAnalysisStarted:
		return "Your call has been completed and we are analyzing the call."
	case UserStatusCallProcessing:
		return "We are processing your conversation and building your personalized job search criteria."
	case UserStatusCallCompleted:
		return "Your call has been completed and processed."
	case UserStatusJobSearchScheduled:
		return "The job search has been scheduled."
	case UserStatusJobSearchStarted:
		return "The job search has started."
	case UserStatusJobSearchComplete:
		return "The job search has been completed."
	default:
		return "No description available for this status."
	}
}

// IsOnboarding returns true if the user is in an onboarding state
// This includes processing resume, call analysis, and call completion states
func (s UserStatus) IsOnboarding() bool {
	return s == UserStatusProcessingResume ||
		s == UserStatusCallScheduled ||
		s == UserStatusCallStarted ||
		s == UserStatusCallAnalysisStarted ||
		s == UserStatusCallProcessing
}
