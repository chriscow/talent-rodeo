package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestExtractRequirements(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test job description
	jobDesc := `We are looking for a Senior Software Engineer with:
    - 5+ years of Go experience
    - 3+ years of cloud infrastructure
    Nice to have:
    - Kubernetes experience
    - React knowledge
    Bachelor's degree in Computer Science required.`

	// Expected response from LLM
	want := &Requirements{
		MustHave: []Requirement{
			{
				Skill:       "Go",
				YearsNeeded: 5,
				Level:       "Senior",
				Context:     "Primary programming language",
			},
			{
				Skill:       "Cloud Infrastructure",
				YearsNeeded: 3,
				Level:       "Intermediate",
				Context:     "Cloud platform experience",
			},
		},
		NiceToHave: []Requirement{
			{
				Skill:   "Kubernetes",
				Level:   "Any",
				Context: "Container orchestration",
			},
			{
				Skill:   "React",
				Level:   "Any",
				Context: "Frontend development",
			},
		},
		MinYearsExperience: 5,
		Education:          []string{"Bachelor's degree in Computer Science"},
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestExtractRequirements: Failed to marshal result: %v", err)
	}
	env.llm.responses = map[string]string{
		requirementsPrompt: string(wantJSON),
	}

	// Create handler
	handler, err := NewRequirementsHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test ExtractRequirements
	got, err := handler.extract(context.Background(), jobDesc)
	if err != nil {
		t.Fatalf("ExtractRequirements failed: %v", err)
	}

	// Compare results
	if len(got.MustHave) != len(want.MustHave) {
		t.Errorf("got %d must-have requirements, want %d", len(got.MustHave), len(want.MustHave))
	}

	if len(got.NiceToHave) != len(want.NiceToHave) {
		t.Errorf("got %d nice-to-have requirements, want %d", len(got.NiceToHave), len(want.NiceToHave))
	}

	if got.MinYearsExperience != want.MinYearsExperience {
		t.Errorf("got min years %f, want %f", got.MinYearsExperience, want.MinYearsExperience)
	}

	if len(got.Education) != len(want.Education) {
		t.Errorf("got %d education requirements, want %d", len(got.Education), len(want.Education))
	}
}

func TestHandleRequirementsMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description.
	input := JobDescriptionInput{
		Description: `We are looking for a Go developer with:
			- 3+ years of Go experience
			- Docker knowledge required
			Bachelor's degree preferred.`,
	}

	want := Requirements{
		MustHave: []Requirement{
			{Skill: "Go", YearsNeeded: 3, Level: "Senior", Context: "Primary programming language"},
			{Skill: "Docker", Level: "Any", Context: "Containerization"},
		},
		NiceToHave: []Requirement{
			{Skill: "Bachelor's degree", Level: "Preferred", Context: "Education"},
		},
		MinYearsExperience: 3,
		Education:          []string{"Bachelor's degree"},
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestHandleMessage: Failed to marshal result: %v", err)
	}

	env.llm.responses = map[string]string{
		requirementsPrompt: string(wantJSON),
	}

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewRequirementsHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	got, err := SendAndReceiveMessage[JobDescriptionInput, Requirements](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicRequirementsExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted requirements.
	if len(got.MustHave) == 0 {
		t.Error("TestHandleMessage: Expected must-have requirements, got none")
	}

	// Check for specific requirements.
	foundGo := false
	foundDocker := false
	for _, req := range got.MustHave {
		switch req.Skill {
		case "Go":
			foundGo = true
			if req.YearsNeeded != 3 {
				t.Errorf("TestHandleMessage: Expected 3 years Go experience, got %v", req.YearsNeeded)
			}
		case "Docker":
			foundDocker = true
		}
	}

	if !foundGo {
		t.Error("TestHandleMessage: Go requirement not found in must-have skills")
	}
	if !foundDocker {
		t.Error("TestHandleMessage: Docker requirement not found in must-have skills")
	}

	// Validate education requirements.
	if len(got.Education) == 0 {
		t.Error("TestHandleMessage: Expected education requirements, got none")
	}
}
