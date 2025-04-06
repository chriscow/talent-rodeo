package internal

import (
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../.env"); err != nil {
		log.Fatal("No .env file found, using system env")
	}
}

func TestResumeValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fs, err := NewFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	defer func() {
		DeleteTestData(t, fs, "test-user")
		if err := fs.Close(); err != nil {
			t.Fatalf("Failed to close firestore client: %v", err)
		}
	}()

	// Create a test logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("valid resume", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Read test file content
		content, err := os.ReadFile(filepath.Join("../testdata/sample-resumes", "chris-cowherd.resume.txt"))
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Create resume
		resume := &Resume{
			RawText: string(content),
		}
		err = CreateResume(context.Background(), fs, *user, resume)
		if err != nil {
			t.Fatalf("CreateResume() error = %v", err)
		}

		// Run resume validation
		err = AnnotateResume(context.Background(), fs, logger, user, resume)
		if err != nil {
			t.Fatalf("AnnotateResume() error = %v", err)
		}

		// Get updated resume
		resumes, err := GetResumes(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetResumes() error = %v", err)
		}
		if len(resumes) == 0 {
			t.Fatal("No resumes found")
		}
		updatedResume := resumes[0]

		// Check resume validation result
		if !updatedResume.IsValid {
			t.Error("Resume should be valid")
		}

		// Get updated user to check state
		updated, found, err := GetUser(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("User not found")
		}

		if updated.State.Status != UserStatusResumeUploaded {
			t.Errorf("User.State.Status = %v, want %v", updated.State.Status, UserStatusResumeUploaded)
		}
		if updated.State.Error != nil {
			t.Errorf("User.State.Error = %v, want nil", *updated.State.Error)
		}
	})

	t.Run("not a resume - easy", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Read test file content
		content, err := os.ReadFile(filepath.Join("../testdata/sample-resumes", "01-not-a-resume.easy.txt"))
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Create resume
		resume := &Resume{
			RawText: string(content),
		}
		err = CreateResume(context.Background(), fs, *user, resume)
		if err != nil {
			t.Fatalf("CreateResume() error = %v", err)
		}

		// Run resume validation
		err = AnnotateResume(context.Background(), fs, logger, user, resume)
		if err != nil {
			t.Fatalf("AnnotateResume() error = %v", err)
		}

		// Get updated resume
		resumes, err := GetResumes(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetResumes() error = %v", err)
		}
		if len(resumes) == 0 {
			t.Fatal("No resumes found")
		}
		updatedResume := resumes[0]

		// Check resume validation result
		if updatedResume.IsValid {
			t.Error("Resume should be invalid")
		}

		// Get updated user to check error state
		updated, found, err := GetUser(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("User not found")
		}

		wantErrorMsg := "The document you uploaded doesn't appear to be a valid resume. Please upload a proper resume document."
		if updated.State.Status != UserStatusError {
			t.Errorf("User.State.Status = %v, want %v", updated.State.Status, UserStatusError)
		}
		if updated.State.Error == nil || *updated.State.Error != wantErrorMsg {
			t.Errorf("User.State.Error = %v, want %v", updated.State.Error, wantErrorMsg)
		}
	})

	t.Run("not a resume - medium", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Read test file content
		content, err := os.ReadFile(filepath.Join("../testdata/sample-resumes", "02-not-a-resume.medium.txt"))
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Create resume
		resume := &Resume{
			RawText: string(content),
		}
		err = CreateResume(context.Background(), fs, *user, resume)
		if err != nil {
			t.Fatalf("CreateResume() error = %v", err)
		}

		// Run resume validation
		err = AnnotateResume(context.Background(), fs, logger, user, resume)
		if err != nil {
			t.Fatalf("AnnotateResume() error = %v", err)
		}

		// Get updated resume
		resumes, err := GetResumes(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetResumes() error = %v", err)
		}
		if len(resumes) == 0 {
			t.Fatal("No resumes found")
		}
		updatedResume := resumes[0]

		// Check resume validation result
		if updatedResume.IsValid {
			t.Error("Resume should be invalid")
		}

		// Get updated user to check error state
		updated, found, err := GetUser(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("User not found")
		}

		wantErrorMsg := "The document you uploaded doesn't appear to be a valid resume. Please upload a proper resume document."
		if updated.State.Status != UserStatusError {
			t.Errorf("User.State.Status = %v, want %v", updated.State.Status, UserStatusError)
		}
		if updated.State.Error == nil || *updated.State.Error != wantErrorMsg {
			t.Errorf("User.State.Error = %v, want %v", updated.State.Error, wantErrorMsg)
		}
	})

	t.Run("jailbreak attempt", func(t *testing.T) {
		// Create test user
		user := CreateTestUser(t, fs, "test-user", "test@example.com", "+1234567890", false)

		// Read test file content
		content, err := os.ReadFile(filepath.Join("../testdata/sample-resumes", "06-jailbreak.hard.txt"))
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Create resume
		resume := &Resume{
			RawText: string(content),
		}
		err = CreateResume(context.Background(), fs, *user, resume)
		if err != nil {
			t.Fatalf("CreateResume() error = %v", err)
		}

		// Run resume validation
		err = AnnotateResume(context.Background(), fs, logger, user, resume)
		if err != nil {
			t.Fatalf("AnnotateResume() error = %v", err)
		}

		// Get updated resume
		resumes, err := GetResumes(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetResumes() error = %v", err)
		}
		if len(resumes) == 0 {
			t.Fatal("No resumes found")
		}
		updatedResume := resumes[0]

		// Check resume validation result
		if updatedResume.IsValid {
			t.Error("Resume should be invalid")
		}

		// Get updated user to check error state
		updated, found, err := GetUser(context.Background(), fs, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("User not found")
		}

		wantErrorMsg := "The document you uploaded doesn't appear to be a valid resume. Please upload a proper resume document."
		if updated.State.Status != UserStatusError {
			t.Errorf("User.State.Status = %v, want %v", updated.State.Status, UserStatusError)
		}
		if updated.State.Error == nil || *updated.State.Error != wantErrorMsg {
			t.Errorf("User.State.Error = %v, want %v", updated.State.Error, wantErrorMsg)
		}
	})
}
