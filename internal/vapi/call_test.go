package vapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetCall_Integration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Ensure VAPI_API_KEY is set
	if os.Getenv("VAPI_API_KEY") == "" {
		t.Skip("VAPI_API_KEY not set")
	}

	ctx := context.Background()
	callID := "2709318a-1b74-44da-9ba9-e1973804b6b9"
	fixtureFile := filepath.Join("testdata", "call_response.json")

	// Load fixture data
	fixtureData, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatalf("Failed to read fixture file: %v", err)
	}

	var want CallResult
	if err := json.Unmarshal(fixtureData, &want); err != nil {
		t.Fatalf("Failed to unmarshal fixture data: %v", err)
	}

	// Make the API call
	got, err := GetCall(ctx, callID)
	if err != nil {
		t.Fatalf("GetCall() error = %v", err)
	}

	// Basic validation of the response
	if got == nil {
		t.Fatal("GetCall() returned nil result")
	}

	// Compare specific fields that should match exactly
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
	if got.OrgID != want.OrgID {
		t.Errorf("OrgID = %v, want %v", got.OrgID, want.OrgID)
	}
	if got.Type != want.Type {
		t.Errorf("Type = %v, want %v", got.Type, want.Type)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %v, want %v", got.Status, want.Status)
	}

	// Print useful information for debugging
	t.Logf("Call Status: %s", got.Status)
	t.Logf("Call Type: %s", got.Type)
	if got.Cost > 0 {
		t.Logf("Call Cost: $%.2f", got.Cost)
	}
}
