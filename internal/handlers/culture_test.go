package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestExtractCulture(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test job description
	jobDesc := `We are a fast-paced startup looking for passionate individuals who thrive in a collaborative environment.
    Our values include innovation, transparency, and continuous learning.
    We believe in flexible work arrangements and encourage open communication.
    Team members should be self-motivated and comfortable with rapid iteration.`

	// Expected response from LLM
	expectedCulture := &Culture{
		Description:  "A dynamic startup environment that emphasizes collaboration and innovation",
		Values:       []string{"Innovation", "Transparency", "Continuous Learning"},
		TeamDynamics: "Highly collaborative with open communication",
		WorkStyle:    "Self-motivated with rapid iteration",
	}

	// Create handler
	handler, err := NewCultureHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set up mock LLM response
	cultureJSON, err := json.Marshal(expectedCulture)
	if err != nil {
		t.Fatalf("Failed to marshal culture: %v", err)
	}
	env.llm.responses[culturePrompt] = string(cultureJSON)

	// Test ExtractCulture
	got, err := handler.extract(context.Background(), jobDesc)
	if err != nil {
		t.Fatalf("ExtractCulture failed: %v", err)
	}

	// Compare results
	if got.Description != expectedCulture.Description {
		t.Errorf("got description %q, want %q", got.Description, expectedCulture.Description)
	}
	if len(got.Values) != len(expectedCulture.Values) {
		t.Errorf("got %d values, want %d", len(got.Values), len(expectedCulture.Values))
	}
	for i, v := range got.Values {
		if v != expectedCulture.Values[i] {
			t.Errorf("got value[%d] %q, want %q", i, v, expectedCulture.Values[i])
		}
	}
	if got.TeamDynamics != expectedCulture.TeamDynamics {
		t.Errorf("got team dynamics %q, want %q", got.TeamDynamics, expectedCulture.TeamDynamics)
	}
	if got.WorkStyle != expectedCulture.WorkStyle {
		t.Errorf("got work style %q, want %q", got.WorkStyle, expectedCulture.WorkStyle)
	}
}

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

	want := &Culture{
		Description:  "A dynamic startup environment that emphasizes collaboration and innovation",
		Values:       []string{"Innovation", "Transparency", "Continuous Learning"},
		TeamDynamics: "Highly collaborative with open communication",
		WorkStyle:    "Self-motivated with rapid iteration",
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestHandleCultureMessage: Failed to marshal result: %v", err)
	}

	env.llm.responses[culturePrompt] = string(wantJSON)

	// Create handler
	handler, err := NewCultureHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("TestHandleCultureMessage: Failed to create handler: %v", err)
	}

	// Start the handler in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go handler.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Allow handler to initialize

	// Use the generic harness to send the job description and receive the extracted culture
	got, err := SendAndReceiveMessage[JobDescriptionInput, Culture](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicCultureExtracted,
		input,
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("TestHandleCultureMessage: Failed to send and receive message: %v", err)
	}

	// Validate the extracted culture
	if got.Description != want.Description {
		t.Errorf("TestHandleCultureMessage: got description %q, want %q", got.Description, want.Description)
	}
	if len(got.Values) != len(want.Values) {
		t.Errorf("TestHandleCultureMessage: got %d values, want %d", len(got.Values), len(want.Values))
	}
	for i, v := range got.Values {
		if v != want.Values[i] {
			t.Errorf("TestHandleCultureMessage: got value[%d] %q, want %q", i, v, want.Values[i])
		}
	}
	if got.TeamDynamics != want.TeamDynamics {
		t.Errorf("TestHandleCultureMessage: got team dynamics %q, want %q", got.TeamDynamics, want.TeamDynamics)
	}
	if got.WorkStyle != want.WorkStyle {
		t.Errorf("TestHandleCultureMessage: got work style %q, want %q", got.WorkStyle, want.WorkStyle)
	}

	// Stop the handler
	cancel()
	time.Sleep(100 * time.Millisecond)
}
