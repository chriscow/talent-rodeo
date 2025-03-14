package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleCultureMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description
	input := JobDescriptionInput{
		Description: `We are a fast-paced startup looking for passionate individuals who thrive in a collaborative environment.
			Our values include innovation, transparency, and continuous learning.
			We believe in flexible work arrangements and encourage open communication.
			Team members should be self-motivated and comfortable with rapid iteration.`,
	}

	// Expected culture result
	want := &Culture{
		Description:  "A dynamic startup environment that emphasizes collaboration and innovation",
		Values:       []string{"Innovation", "Transparency", "Continuous Learning"},
		TeamDynamics: "Highly collaborative with open communication",
		WorkStyle:    "Self-motivated with rapid iteration",
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	// Create handler
	handler, err := NewCultureHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set the LLM response for the system prompt
	env.llm.responses[handler.GetSystemPrompt()] = string(wantJSON)

	// Send test message and verify response
	got, err := SendAndReceiveMessage[JobDescriptionInput, Culture](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicCultureExtracted,
		input,
		5*time.Second,
		"culture-test",
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the extracted culture
	if got.Description != want.Description {
		t.Errorf("got description %q, want %q", got.Description, want.Description)
	}
	if len(got.Values) != len(want.Values) {
		t.Errorf("got %d values, want %d", len(got.Values), len(want.Values))
	}
	for i, v := range got.Values {
		if v != want.Values[i] {
			t.Errorf("got value[%d] %q, want %q", i, v, want.Values[i])
		}
	}
	if got.TeamDynamics != want.TeamDynamics {
		t.Errorf("got team dynamics %q, want %q", got.TeamDynamics, want.TeamDynamics)
	}
	if got.WorkStyle != want.WorkStyle {
		t.Errorf("got work style %q, want %q", got.WorkStyle, want.WorkStyle)
	}
}
