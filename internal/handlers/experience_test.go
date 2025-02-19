package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleExperienceMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific interview transcript
	input := TranscriptInput{
		Content: `I've been in the tech industry for 6 years.
		I'm currently a Lead Developer at InnovativeTech for 2 years,
		where I work with React and Node.js. Previously, I was at
		CloudSystems for 4 years as a Full Stack Developer using
		Python and JavaScript.`,
	}

	want := &Experience{
		TotalYears: 6,
		Roles: []Role{
			{
				Title:       "Lead Developer",
				Company:     "InnovativeTech",
				YearsInRole: 2,
				Skills:      []string{"React", "Node.js"},
			},
			{
				Title:       "Full Stack Developer",
				Company:     "CloudSystems",
				YearsInRole: 4,
				Skills:      []string{"Python", "JavaScript"},
			},
		},
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	// Create handler
	handler, err := NewExperienceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set the LLM response for the system prompt
	env.llm.responses[handler.GetSystemPrompt()] = string(wantJSON)

	// Send test message and verify response
	got, err := SendAndReceiveMessage[TranscriptInput, Experience](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicTranscriptInput,
		TopicExperienceExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted experience
	if got.TotalYears != want.TotalYears {
		t.Errorf("got %v total years, want %v", got.TotalYears, want.TotalYears)
	}

	if len(got.Roles) != len(want.Roles) {
		t.Errorf("got %d roles, want %d", len(got.Roles), len(want.Roles))
	}

	// Check for specific roles
	foundLeadDev := false
	foundFullStack := false
	for _, role := range got.Roles {
		switch role.Title {
		case "Lead Developer":
			foundLeadDev = true
			if role.YearsInRole != 2 {
				t.Errorf("got %v years as Lead Developer, want 2", role.YearsInRole)
			}
		case "Full Stack Developer":
			foundFullStack = true
			if role.YearsInRole != 4 {
				t.Errorf("got %v years as Full Stack Developer, want 4", role.YearsInRole)
			}
		}
	}

	if !foundLeadDev {
		t.Error("Lead Developer role not found")
	}
	if !foundFullStack {
		t.Error("Full Stack Developer role not found")
	}
}
