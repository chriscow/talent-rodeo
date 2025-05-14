package internal

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/chriscow/minds/tools"
	"github.com/chriscow/vapi-go"
)

func TestSteeringQueryFromTranscript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in CI")
	}

	// Read the test data file
	data, err := os.ReadFile("../testdata/vapi/create-call-response.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	// Parse the JSON into a struct that has the messages field
	var callResult vapi.Call
	if err := json.Unmarshal(data, &callResult); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Convert the messages to a transcript
	transcript := NewTranscriptFromCallResult(callResult)

	// Verify the conversion
	if len(transcript.Messages) == 0 {
		t.Error("Expected non-empty transcript messages")
	}

	// Create a steering query from the transcript
	sq, err := GenerateSteeringQuery(context.Background(), transcript, tools.WithModel(tools.GPT41Nano))
	if err != nil {
		t.Fatalf("Failed to create steering query: %v", err)
	}

	if err := sq.Validate(); err != nil {
		t.Errorf("Steering query validation failed: %v", err)
	}
}
