package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestExtractWorkplace(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test job description
	jobDesc := `This is a remote position with flexible hours. We are looking for someone who can work
		in the EST timezone. Some travel (10-20%) may be required for quarterly team meetings.
		Our office environment is modern and collaborative when you do come in.`

	// Expected response from LLM
	want := &Workplace{
		Location:    "Remote",
		Type:        "Remote",
		Schedule:    "Flexible hours",
		Environment: "Modern and collaborative office",
		Travel:      []string{"10-20% travel for quarterly team meetings"},
	}

	// Create handler
	handler, err := NewWorkplaceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal workplace: %v", err)
	}
	env.llm.responses[workplacePrompt] = string(wantJSON)

	// Test ExtractWorkplace
	got, err := handler.extract(context.Background(), jobDesc)
	if err != nil {
		t.Fatalf("ExtractWorkplace failed: %v", err)
	}

	// Compare results
	if got.Location != want.Location {
		t.Errorf("got location %q, want %q", got.Location, want.Location)
	}
	if got.Type != want.Type {
		t.Errorf("got type %q, want %q", got.Type, want.Type)
	}
	if got.Schedule != want.Schedule {
		t.Errorf("got schedule %q, want %q", got.Schedule, want.Schedule)
	}
	if got.Environment != want.Environment {
		t.Errorf("got environment %q, want %q", got.Environment, want.Environment)
	}
	if len(got.Travel) != len(want.Travel) {
		t.Errorf("got %d travel requirements, want %d", len(got.Travel), len(want.Travel))
	} else {
		for i, v := range got.Travel {
			if v != want.Travel[i] {
				t.Errorf("got travel[%d] %q, want %q", i, v, want.Travel[i])
			}
		}
	}
}

func TestWorkplaceHandleMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description
	input := JobDescriptionInput{
		Description: "Remote position with flexible hours in EST timezone. Some travel required.",
	}

	// Create expected response
	want := &Workplace{
		Location:    "Remote",
		Type:        "Remote",
		Schedule:    "Flexible hours",
		Environment: "Not specified",
		Travel:      []string{"Some travel required"},
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestWorkplaceHandleMessage: Failed to marshal result: %v", err)
	}
	env.llm.responses[workplacePrompt] = string(wantJSON)

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewWorkplaceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	got, err := SendAndReceiveMessage[JobDescriptionInput, Workplace](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicWorkplaceExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Verify response
	if got.Location != want.Location {
		t.Errorf("got location %q, want %q", got.Location, want.Location)
	}
	if got.Type != want.Type {
		t.Errorf("got type %q, want %q", got.Type, want.Type)
	}
	if got.Schedule != want.Schedule {
		t.Errorf("got schedule %q, want %q", got.Schedule, want.Schedule)
	}
	if got.Environment != want.Environment {
		t.Errorf("got environment %q, want %q", got.Environment, want.Environment)
	}
	if len(got.Travel) != len(want.Travel) {
		t.Errorf("got %d travel requirements, want %d", len(got.Travel), len(want.Travel))
	} else {
		for i, v := range got.Travel {
			if v != want.Travel[i] {
				t.Errorf("got travel[%d] %q, want %q", i, v, want.Travel[i])
			}
		}
	}
}
