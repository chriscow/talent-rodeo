package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestExtractPreferences(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test transcript
	transcript := `I'm looking for a remote position, but I'm open to hybrid work in San Francisco or Seattle.
    I prefer working in mid-size to enterprise companies, especially in the tech or fintech industry.
    I enjoy collaborative environments and I'm looking for a salary between $150,000 and $200,000.`

	// Expected response from LLM
	want := &Preferences{
		PreferredLocations: []string{"San Francisco", "Seattle"},
		RemotePreference:   "remote",
		WorkStyle:          "collaborative",
		CompanySize:        "mid-size",
		Industry:           []string{"tech", "fintech"},
		SalaryRange: &SalaryRange{
			Min: 150000,
			Max: 200000,
		},
	}

	// Set up mock LLM response
	preferencesJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal preferences: %v", err)
	}
	env.llm.responses[preferencesPrompt] = string(preferencesJSON)

	// Create handler
	handler, err := NewPreferencesHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	got, err := handler.extract(context.Background(), transcript)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

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

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	env.llm.responses[preferencesPrompt] = string(wantJSON)

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewPreferencesHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

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
