package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/chriscow/minds/tools"
)

// Helper function to reset mock LLM values
func resetMockLLM() {
	tools.MockLLMResponse = ""
	tools.MockLLMError = nil
}

// setupResumeTest creates a new test Firestore client and logger for resume tests
func setupResumeTest(t *testing.T) (*FirestoreClient, *slog.Logger, func()) {
	if testing.Short() {
		t.Skip("skipping resume validation tests in CI")
	}

	fs, err := NewTestFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("newTestFirestoreClient() error = %v", err)
	}

	// Create a test logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Return cleanup function
	cleanup := func() {
		DeleteTestData(t, fs, "test-user")
		if err := fs.Close(); err != nil {
			t.Fatalf("Failed to close firestore client: %v", err)
		}
	}

	return fs, logger, cleanup
}

func TestValidResume(t *testing.T) {
	fs, logger, cleanup := setupResumeTest(t)
	defer cleanup()

	// Reset mock LLM for this test
	defer resetMockLLM()

	// Create valid resume mock response
	mockResponse := annotatedResume{
		Analysis: "This is a valid resume for a software engineer with extensive experience.",
		IsResume: true,
		Summary:  stringPtr("name: Chris Cowherd\ntitle: Software Engineering Leader\n"),
	}
	responseJSON, err := json.Marshal(mockResponse)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}
	tools.MockLLMResponse = string(responseJSON)

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
	err = ProcessCandidateProfile(context.Background(), fs, logger, user, resume, "")
	if err != nil {
		t.Fatalf("ProcessCandidateProfile() error = %v", err)
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
}

func TestNotAResumeEasy(t *testing.T) {
	fs, logger, cleanup := setupResumeTest(t)
	defer cleanup()

	// Reset mock LLM for this test
	defer resetMockLLM()

	// Create invalid resume mock response
	mockResponse := annotatedResume{
		Analysis: "This is not a resume but appears to be random text.",
		IsResume: false,
		Summary:  nil,
	}
	responseJSON, err := json.Marshal(mockResponse)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}
	tools.MockLLMResponse = string(responseJSON)

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
	err = ProcessCandidateProfile(context.Background(), fs, logger, user, resume, "")
	if err != nil {
		t.Fatalf("ProcessCandidateProfile() error = %v", err)
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

	if updatedResume == nil {
		t.Fatal("nill resume returned")
	}

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

	wantErrorMsg := "Uploaded document is not a valid resume"
	if updated.State.Status != UserStatusError {
		t.Errorf("User.State.Status = %v, want %v", updated.State.Status, UserStatusError)
	}
	if updated.State.Error == nil {
		t.Errorf("User.State.Error is nil, want %v", wantErrorMsg)
	}

	if updated.State.Error == nil || *updated.State.Error != wantErrorMsg {
		errValue := "<nil>"
		if updated.State.Error != nil {
			errValue = *updated.State.Error
		}
		t.Errorf("User.State.Error = %v, want %v", errValue, wantErrorMsg)
	}
}

func TestNotAResumeMedium(t *testing.T) {
	fs, logger, cleanup := setupResumeTest(t)
	defer cleanup()

	// Reset mock LLM for this test
	defer resetMockLLM()

	// Create invalid resume mock response
	mockResponse := annotatedResume{
		Analysis: "This document appears to be a news article, not a resume.",
		IsResume: false,
		Summary:  nil,
	}
	responseJSON, err := json.Marshal(mockResponse)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}
	tools.MockLLMResponse = string(responseJSON)

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
	err = ProcessCandidateProfile(context.Background(), fs, logger, user, resume, "")
	if err != nil {
		t.Fatalf("ProcessCandidateProfile() error = %v", err)
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
		errValue := "<nil>"
		if updated.State.Error != nil {
			errValue = *updated.State.Error
		}
		t.Errorf("User.State.Error = %v, want %v", errValue, wantErrorMsg)
	}
}

func TestJailbreakAttempt(t *testing.T) {
	fs, logger, cleanup := setupResumeTest(t)
	defer cleanup()

	// Reset mock LLM for this test
	defer resetMockLLM()

	// Create invalid resume mock response for jailbreak attempt
	mockResponse := annotatedResume{
		Analysis: "This appears to be an attempt to jailbreak or influence the system, not a resume.",
		IsResume: false,
		Summary:  nil,
	}
	responseJSON, err := json.Marshal(mockResponse)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}
	tools.MockLLMResponse = string(responseJSON)

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
	err = ProcessCandidateProfile(context.Background(), fs, logger, user, resume, "")
	if err != nil {
		t.Fatalf("ProcessCandidateProfile() error = %v", err)
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
		errValue := "<nil>"
		if updated.State.Error != nil {
			errValue = *updated.State.Error
		}
		t.Errorf("User.State.Error = %v, want %v", errValue, wantErrorMsg)
	}
}
