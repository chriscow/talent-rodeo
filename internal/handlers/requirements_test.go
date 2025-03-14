package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleRequirementsMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description.
	input := JobDescriptionInput{
		Description: `We are looking for a Senior Software Engineer with:
			- 5+ years of Go experience
			- 3+ years of cloud infrastructure
			Nice to have:
			- Kubernetes experience
			- React knowledge
			Bachelor's degree in Computer Science required.`,
	}

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

	// Create handler
	handler, err := NewRequirementsHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set up mock LLM response for the system prompt
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}
	env.llm.responses[handler.GetSystemPrompt()] = string(wantJSON)

	// Send test message and verify response
	got, err := SendAndReceiveMessage[JobDescriptionInput, Requirements](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicRequirementsExtracted,
		input,
		5*time.Second,
		"requirements-test",
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted requirements
	if len(got.MustHave) != len(want.MustHave) {
		t.Errorf("got %d must-have requirements, want %d", len(got.MustHave), len(want.MustHave))
	}

	// Check for specific requirements
	foundGo := false
	foundCloud := false
	for _, req := range got.MustHave {
		switch req.Skill {
		case "Go":
			foundGo = true
			if req.YearsNeeded != 5 {
				t.Errorf("Expected 5 years Go experience, got %v", req.YearsNeeded)
			}
			if req.Level != "Senior" {
				t.Errorf("Expected Senior level for Go, got %v", req.Level)
			}
		case "Cloud Infrastructure":
			foundCloud = true
			if req.YearsNeeded != 3 {
				t.Errorf("Expected 3 years Cloud Infrastructure experience, got %v", req.YearsNeeded)
			}
		}
	}

	if !foundGo {
		t.Error("Go requirement not found in must-have skills")
	}
	if !foundCloud {
		t.Error("Cloud Infrastructure requirement not found in must-have skills")
	}

	// Validate nice-to-have requirements
	if len(got.NiceToHave) != len(want.NiceToHave) {
		t.Errorf("got %d nice-to-have requirements, want %d", len(got.NiceToHave), len(want.NiceToHave))
	}

	// Validate minimum years experience
	if got.MinYearsExperience != want.MinYearsExperience {
		t.Errorf("got min years %f, want %f", got.MinYearsExperience, want.MinYearsExperience)
	}

	// Validate education requirements
	if len(got.Education) != len(want.Education) {
		t.Errorf("got %d education requirements, want %d", len(got.Education), len(want.Education))
	}
}
