package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestExtractSkills(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test transcript
	transcript := `I have been working as a software engineer for 10 years.
    I'm an expert in Go with 5 years of experience, and have intermediate knowledge of Python with 3 years of use.
    I also started learning Rust about 6 months ago.`

	// Expected response from LLM
	want := &Skills{
		Skills: []Skill{
			{
				Name:       "Go",
				Level:      "expert",
				YearsUsed:  5.0,
				Confidence: "high",
			},
			{
				Name:       "Python",
				Level:      "intermediate",
				YearsUsed:  3.0,
				Confidence: "high",
			},
			{
				Name:       "Rust",
				Level:      "beginner",
				YearsUsed:  0.5,
				Confidence: "high",
			},
		},
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal skills: %v", err)
	}
	env.llm.responses[skillsPrompt] = string(wantJSON)

	// Create handler
	handler, err := NewSkillsHandler(context.Background(), env.js, env.llm, env.logger)
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

func TestHandleSkillsMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input
	input := JobDescriptionInput{
		Description: `We are looking for a software engineer with:
			- Expert level Go programming (5+ years)
			- Strong Python skills (3+ years)
			- Some experience with Rust`,
	}

	want := &Skills{
		Skills: []Skill{
			{
				Name:       "Go",
				Level:      "expert",
				YearsUsed:  5.0,
				Confidence: "high",
			},
			{
				Name:       "Python",
				Level:      "intermediate",
				YearsUsed:  3.0,
				Confidence: "high",
			},
			{
				Name:       "Rust",
				Level:      "beginner",
				YearsUsed:  0.5,
				Confidence: "medium",
			},
		},
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	env.llm.responses[skillsPrompt] = string(wantJSON)

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewSkillsHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	got, err := SendAndReceiveMessage[JobDescriptionInput, Skills](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicSkillsExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted skills
	if len(got.Skills) != len(want.Skills) {
		t.Fatalf("got %d skills, want %d", len(got.Skills), len(want.Skills))
	}

	for i, gotSkill := range got.Skills {
		wantSkill := want.Skills[i]
		if gotSkill.Name != wantSkill.Name {
			t.Errorf("got skill name %q, want %q", gotSkill.Name, wantSkill.Name)
		}
		if gotSkill.Level != wantSkill.Level {
			t.Errorf("got skill level %q, want %q", gotSkill.Level, wantSkill.Level)
		}
		if gotSkill.YearsUsed != wantSkill.YearsUsed {
			t.Errorf("got years used %.1f, want %.1f", gotSkill.YearsUsed, wantSkill.YearsUsed)
		}
		if gotSkill.Confidence != wantSkill.Confidence {
			t.Errorf("got confidence %q, want %q", gotSkill.Confidence, wantSkill.Confidence)
		}
	}
}
