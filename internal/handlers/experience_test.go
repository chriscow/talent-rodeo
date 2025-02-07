package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMessageHandling(t *testing.T) {
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

	want := Experience{
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

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestMessageHandling: Failed to marshal result: %v", err)
	}

	env.llm.responses = map[string]string{
		experiencePrompt: string(wantJSON),
	}

	// Create handler
	handler, err := NewExperienceHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("TestMessageHandling: Failed to create handler: %v", err)
	}

	// Start the handler in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go handler.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Allow handler to initialize

	// Use the generic harness to send the transcript and receive the extracted experience
	got, err := SendAndReceiveMessage[TranscriptInput, Experience](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicTranscriptInput,
		TopicExperienceExtracted,
		input,
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("TestMessageHandling: Failed to send and receive message: %v", err)
	}

	// Validate the extracted experience
	if got.TotalYears != want.TotalYears {
		t.Errorf("TestMessageHandling: Expected %v total years, got %v", want.TotalYears, got.TotalYears)
	}

	if len(got.Roles) != len(want.Roles) {
		t.Errorf("TestMessageHandling: Expected %d roles, got %d", len(want.Roles), len(got.Roles))
	}

	// Check for specific roles
	foundLeadDev := false
	foundFullStack := false
	for _, role := range got.Roles {
		switch role.Title {
		case "Lead Developer":
			foundLeadDev = true
			if role.YearsInRole != 2 {
				t.Errorf("TestMessageHandling: Expected 2 years as Lead Developer, got %v", role.YearsInRole)
			}
		case "Full Stack Developer":
			foundFullStack = true
			if role.YearsInRole != 4 {
				t.Errorf("TestMessageHandling: Expected 4 years as Full Stack Developer, got %v", role.YearsInRole)
			}
		}
	}

	if !foundLeadDev {
		t.Error("TestMessageHandling: Lead Developer role not found")
	}
	if !foundFullStack {
		t.Error("TestMessageHandling: Full Stack Developer role not found")
	}

	// Stop the handler
	cancel()
	time.Sleep(100 * time.Millisecond)
}
