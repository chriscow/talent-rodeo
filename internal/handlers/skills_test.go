package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

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

	// Create handler
	handler, err := NewSkillsHandler(context.Background(), env.js, env.llm, env.logger, env.db)
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
	got, err := SendAndReceiveMessage[JobDescriptionInput, Skills](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicSkillsExtracted,
		input,
		5*time.Second,
		"skills-test",
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
