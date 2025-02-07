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
	jobDesc := `We are looking for a Senior Software Engineer to join our team.
    This is a remote-first position with quarterly travel to our San Francisco office.
    Must be able to work in PST timezone.
    Full-time position with flexible hours.`

	// Expected response from LLM
	expectedWorkplace := &Workplace{
		Location:       "San Francisco",
		Type:           "Remote-first",
		Schedule:       "Full-time",
		TimeZone:       "PST",
		TravelRequired: "Quarterly",
	}

	// Create handler
	handler, err := NewWorkplaceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set up mock LLM response
	workplaceJSON, err := json.Marshal(expectedWorkplace)
	if err != nil {
		t.Fatalf("Failed to marshal workplace: %v", err)
	}
	env.llm.responses[workplacePrompt] = string(workplaceJSON)

	// Test ExtractWorkplace
	got, err := handler.extract(context.Background(), jobDesc)
	if err != nil {
		t.Fatalf("ExtractWorkplace failed: %v", err)
	}

	// Compare results
	if got.Location != expectedWorkplace.Location {
		t.Errorf("got location %q, want %q", got.Location, expectedWorkplace.Location)
	}
	if got.Type != expectedWorkplace.Type {
		t.Errorf("got type %q, want %q", got.Type, expectedWorkplace.Type)
	}
	if got.Schedule != expectedWorkplace.Schedule {
		t.Errorf("got schedule %q, want %q", got.Schedule, expectedWorkplace.Schedule)
	}
	if got.TimeZone != expectedWorkplace.TimeZone {
		t.Errorf("got timezone %q, want %q", got.TimeZone, expectedWorkplace.TimeZone)
	}
	if got.TravelRequired != expectedWorkplace.TravelRequired {
		t.Errorf("got travel required %q, want %q", got.TravelRequired, expectedWorkplace.TravelRequired)
	}
}

func TestHandleWorkplaceMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description
	input := JobDescriptionInput{
		Description: `This is a remote-first position with quarterly travel to our San Francisco office.
			Must be able to work in PST timezone.
			Full-time position with flexible hours.`,
	}

	want := &Workplace{
		Location:       "San Francisco",
		Type:           "Remote-first",
		Schedule:       "Full-time",
		TimeZone:       "PST",
		TravelRequired: "Quarterly",
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestHandleWorkplaceMessage: Failed to marshal result: %v", err)
	}

	env.llm.responses[workplacePrompt] = string(wantJSON)

	// Create handler
	handler, err := NewWorkplaceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("TestHandleWorkplaceMessage: Failed to create handler: %v", err)
	}

	// Start the handler in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go handler.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Allow handler to initialize

	// Use the generic harness to send the job description and receive the extracted workplace
	got, err := SendAndReceiveMessage[JobDescriptionInput, Workplace](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicWorkplaceExtracted,
		input,
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("TestHandleWorkplaceMessage: Failed to send and receive message: %v", err)
	}

	// Validate the extracted workplace
	if got.Location != want.Location {
		t.Errorf("TestHandleWorkplaceMessage: got location %q, want %q", got.Location, want.Location)
	}
	if got.Type != want.Type {
		t.Errorf("TestHandleWorkplaceMessage: got type %q, want %q", got.Type, want.Type)
	}
	if got.Schedule != want.Schedule {
		t.Errorf("TestHandleWorkplaceMessage: got schedule %q, want %q", got.Schedule, want.Schedule)
	}
	if got.TimeZone != want.TimeZone {
		t.Errorf("TestHandleWorkplaceMessage: got timezone %q, want %q", got.TimeZone, want.TimeZone)
	}
	if got.TravelRequired != want.TravelRequired {
		t.Errorf("TestHandleWorkplaceMessage: got travel required %q, want %q", got.TravelRequired, want.TravelRequired)
	}

	// Stop the handler
	cancel()
	time.Sleep(100 * time.Millisecond)
}
