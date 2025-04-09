package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../.env"); err != nil {
		wd, _ := os.Getwd()
		log.Printf("failed to load .env from %s: %v", wd, err)
	}

	/*
		// Get the absolute path to the project root
		wd, err := os.Getwd()
		if err != nil {
			log.Printf("error getting working directory: %v", err)
			return
		}
		log.Printf("present working directory: %s", wd)

		// The project root is one level up from the internal directory
		projectRoot := filepath.Dir(wd)
		envPath := filepath.Join(projectRoot, ".env")

		log.Printf("attempting to load .env from: %s", envPath)
		if err := godotenv.Overload(envPath); err != nil {
			log.Printf("failed to load .env from %s: %v", envPath, err)
			// Try loading from current directory as fallback
			if err := godotenv.Overload(".env"); err != nil {
				log.Printf("failed to load .env from current directory: %v", err)
			}
		} else {
			log.Printf("successfully loaded .env from: %s", envPath)
		}

		// Log environment variables (without showing sensitive data)
		projectID := os.Getenv("FIREBASE_PROJECT_ID")
		credentials := os.Getenv("FIREBASE_CREDENTIALS")

		log.Printf("FIREBASE_PROJECT_ID is %s", map[bool]string{true: "set", false: "not set"}[projectID != ""])
		log.Printf("FIREBASE_CREDENTIALS is %s", map[bool]string{true: "set", false: "not set"}[credentials != ""])

		if projectID == "" {
			log.Fatal("FIREBASE_PROJECT_ID is not set")
		}

		if credentials == "" {
			log.Fatal("FIREBASE_CREDENTIALS is not set")
		}
	*/
}

func TestUser(t *testing.T) {
	ctx := context.Background()

	client, err := NewFirestoreClient(ctx)
	if err != nil {
		// Log the error and working directory
		dir, _ := os.Getwd()
		log.Printf("present working directory: %s", dir)
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-")
		client.Close()
	})

	// Create test user using helper with unique email
	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("test-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Test User", uniqueEmail, "+14251234567", false)

	// Test GetUser
	t.Run("GetUser", func(t *testing.T) {
		got, found, err := GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("GetUser() got found = false, want true")
		}
		if got == nil {
			t.Fatal("GetUser() got nil, want user")
		}
		if got.Email != user.Email {
			t.Errorf("GetUser() got email = %v, want %v", got.Email, user.Email)
		}
	})

	// Test GetUserByEmail
	t.Run("GetUserByEmail", func(t *testing.T) {
		got, found, err := GetUserByEmail(ctx, client, user.Email)
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if !found {
			t.Fatal("GetUserByEmail() got found = false, want true")
		}
		if got == nil {
			t.Fatal("GetUserByEmail() got nil, want user")
		}
		if got.ID != user.ID {
			t.Errorf("GetUserByEmail() got ID = %v, want %v", got.ID, user.ID)
		}
	})

	// Test UpdateUser
	t.Run("UpdateUser", func(t *testing.T) {
		updates := map[string]any{
			"first_name": "Updated",
		}
		if err := UpdateUser(ctx, client, user.ID, updates); err != nil {
			t.Fatalf("UpdateUser() error = %v", err)
		}

		got, found, err := GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUser() after update error = %v", err)
		}
		if !found {
			t.Fatal("GetUser() after update got found = false, want true")
		}
		if got.FirstName != "Updated" {
			t.Errorf("UpdateUser() got first_name = %v, want %v", got.FirstName, "Updated")
		}
	})

	// Test ListUsers
	t.Run("ListUsers", func(t *testing.T) {
		users, err := ListUsers(ctx, client, 10, 0)
		if err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}
		if len(users) == 0 {
			t.Error("ListUsers() got empty list, want at least one user")
		}
	})

	// Test DeleteUser
	t.Run("DeleteUser", func(t *testing.T) {
		if err := DeleteUser(ctx, client, user.ID); err != nil {
			t.Fatalf("DeleteUser() error = %v", err)
		}

		got, found, err := GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUser() after delete error = %v", err)
		}
		if found {
			t.Error("GetUser() after delete got found = true, want false")
		}
		if got != nil {
			t.Error("GetUser() after delete got user, want nil")
		}
	})
}

func TestUserStates(t *testing.T) {
	ctx := context.Background()

	client, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("setupFirestoreClient() error = %v", err)
	}

	// Clean up before running tests and after tests complete
	DeleteTestData(t, client, "test-user-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-")
		client.Close()
	})

	// Test creating user with invalid state
	t.Run("CreateUserWithInvalidState", func(t *testing.T) {
		invalidUser := &User{
			Name:  "Invalid State User",
			Email: "invalid@example.com",
			State: UserState{
				Status: UserStatusUnknown,
			},
		}
		err := CreateUser(ctx, client, invalidUser)
		if err == nil {
			t.Error("CreateUser() with UserStatusUnknown should return error")
		}

		invalidUser.State.Status = UserStatusError
		err = CreateUser(ctx, client, invalidUser)
		if err == nil {
			t.Error("CreateUser() with UserStatusError should return error")
		}
	})

	// Test state transitions
	t.Run("UpdateUserState", func(t *testing.T) {
		// Create a test user
		user := CreateTestUser(t, client, "State Test User", "state@example.com", "+14251234567", false)

		// Initial state should be PendingResumeUpload
		if user.State.Status != UserStatusPendingResumeUpload {
			t.Errorf("Initial state = %v, want %v", user.State.Status, UserStatusPendingResumeUpload)
		}

		// Test state transition sequence
		states := []UserStatus{
			UserStatusResumeUploaded,
			UserStatusCallScheduled,
			UserStatusCallStarted,
			UserStatusCallAnalysisStarted,
		}

		for _, newStatus := range states {
			prevStarted := user.State.Started
			time.Sleep(time.Millisecond) // Ensure time difference

			err := UpdateUserStatus(ctx, client, user, newStatus)
			if err != nil {
				t.Fatalf("UpdateUserState() to %v error = %v", newStatus, err)
			}

			// Fetch updated user
			updated, found, err := GetUser(ctx, client, user.ID)
			if err != nil || !found {
				t.Fatalf("GetUser() error = %v, found = %v", err, found)
			}

			// Verify state
			if updated.State.Status != newStatus {
				t.Errorf("State = %v, want %v", updated.State.Status, newStatus)
			}
			if updated.State.Started.IsZero() {
				t.Error("State.Started should not be zero")
			}
			if updated.State.Started.Before(prevStarted) || updated.State.Started.Equal(prevStarted) {
				t.Error("State.Started should be after previous state's start time")
			}
			if updated.State.Error != nil {
				t.Error("State.Error should be nil during normal transition")
			}

			// Update our reference
			user = updated
		}
	})

	// Test String() and Description() methods
	t.Run("StatusStringAndDescription", func(t *testing.T) {
		tests := []struct {
			status     UserStatus
			wantString string
			wantDesc   string
		}{
			{UserStatusUnknown, "Unknown", "The user status is unknown."},
			{UserStatusError, "Error", "The user is in an error state. Check the error field for more information."},
			{UserStatusPendingResumeUpload, "Pending Resume Upload", "We are waiting for your resume to be uploaded."},
			{UserStatusResumeUploaded, "Resume Uploaded", "The resume has been successfully uploaded but a call has not been scheduled yet."},
			{UserStatusCallScheduled, "Call Scheduled", "We have your resume and a call has been scheduled."},
			{UserStatusCallStarted, "Call Started", "Your introductory call has started."},
			{UserStatusCallAnalysisStarted, "Call Analysis Started", "Your introductory call has been completed and we are analyzing the call."},
		}

		for _, tt := range tests {
			t.Run(tt.wantString, func(t *testing.T) {
				if got := tt.status.String(); got != tt.wantString {
					t.Errorf("String() = %v, want %v", got, tt.wantString)
				}
				if got := tt.status.Description(); got != tt.wantDesc {
					t.Errorf("Description() = %v, want %v", got, tt.wantDesc)
				}
			})
		}
	})
}

func TestUpdateUserStateWithError(t *testing.T) {
	fs, err := NewFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	defer fs.Close()

	tests := []struct {
		name     string
		errorMsg string
		want     *UserState
	}{
		{
			name:     "basic error message",
			errorMsg: "Invalid resume format",
			want: &UserState{
				Status:  UserStatusError,
				Error:   stringPtr("Invalid resume format"),
				Started: time.Now(),
			},
		},
		{
			name:     "empty error message",
			errorMsg: "",
			want: &UserState{
				Status:  UserStatusError,
				Error:   stringPtr(""),
				Started: time.Now(),
			},
		},
		{
			name:     "long error message",
			errorMsg: "This is a very long error message that contains detailed information about what went wrong with the resume validation process",
			want: &UserState{
				Status:  UserStatusError,
				Error:   stringPtr("This is a very long error message that contains detailed information about what went wrong with the resume validation process"),
				Started: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test user
			user := CreateTestUser(t, fs, "Test User", "test@example.com", "+14251234567", false)

			// Update user state with error
			err := UpdateUserStateWithError(context.Background(), fs, user, tt.errorMsg)
			if err != nil {
				t.Fatalf("UpdateUserStateWithError() error = %v", err)
			}

			// Get updated user
			updated, found, err := GetUser(context.Background(), fs, user.ID)
			if err != nil {
				t.Fatalf("GetUser() error = %v", err)
			}
			if !found {
				t.Fatal("GetUser() user not found")
			}

			// Check state
			if updated.State.Status != UserStatusError {
				t.Errorf("Status = %v, want %v", updated.State.Status, UserStatusError)
			}
			if updated.State.Error == nil || *updated.State.Error != tt.errorMsg {
				t.Errorf("Error = %v, want %v", updated.State.Error, tt.errorMsg)
			}
			if updated.State.Started.IsZero() {
				t.Error("Started should not be zero")
			}
		})
	}
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
