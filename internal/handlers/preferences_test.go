package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestHandlePreferencesMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input
	input := JobDescriptionInput{
		Description: `Looking for candidates who:
			- Are willing to work remotely or hybrid in San Francisco/Seattle
			- Have experience in tech or fintech companies
			- Thrive in collaborative environments
			- Salary range: $150-200k`,
	}

	want := &Preferences{
		PreferredLocations: []string{"San Francisco", "Seattle"},
		RemotePreference:   "hybrid",
		WorkStyle:          "collaborative",
		CompanySize:        "mid-size",
		Industry:           []string{"tech", "fintech"},
		SalaryRange: &SalaryRange{
			Min: 150000,
			Max: 200000,
		},
	}

	// Create handler
	handler, err := NewPreferencesHandler(context.Background(), env.js, env.llm, env.logger)
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
	got, err := SendAndReceiveMessage[JobDescriptionInput, Preferences](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicPreferencesExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted preferences
	if !reflect.DeepEqual(got.PreferredLocations, want.PreferredLocations) {
		t.Errorf("got locations %v, want %v", got.PreferredLocations, want.PreferredLocations)
	}
	if got.RemotePreference != want.RemotePreference {
		t.Errorf("got remote preference %q, want %q", got.RemotePreference, want.RemotePreference)
	}
	if got.WorkStyle != want.WorkStyle {
		t.Errorf("got work style %q, want %q", got.WorkStyle, want.WorkStyle)
	}
	if got.CompanySize != want.CompanySize {
		t.Errorf("got company size %q, want %q", got.CompanySize, want.CompanySize)
	}
	if !reflect.DeepEqual(got.Industry, want.Industry) {
		t.Errorf("got industry %v, want %v", got.Industry, want.Industry)
	}
	if !reflect.DeepEqual(got.SalaryRange, want.SalaryRange) {
		t.Errorf("got salary range %v, want %v", got.SalaryRange, want.SalaryRange)
	}
}
