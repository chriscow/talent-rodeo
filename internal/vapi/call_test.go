package vapi

import (
	"context"
	"encoding/json"
	"os"
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
	fixtureFile := "/workspaces/talent-rodeo/testdata/vapi/create-call-response.json"

	// Load fixture data
	fixtureData, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatalf("Failed to read fixture file: %v", err)
	}

	var want Call
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
	if *got.ID != *want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
	if *got.OrgID != *want.OrgID {
		t.Errorf("OrgID = %v, want %v", got.OrgID, want.OrgID)
	}
	if *got.Type != *want.Type {
		t.Errorf("Type = %v, want %v", got.Type, want.Type)
	}
	if *got.Status != *want.Status {
		t.Errorf("Status = %v, want %v", got.Status, want.Status)
	}

	// Print useful information for debugging
	if got.Status != nil {
		t.Logf("Call Status: %s", *got.Status)
	}
	if got.Type != nil {
		t.Logf("Call Type: %s", *got.Type)
	}
	if got.Cost != nil && *got.Cost > 0 {
		t.Logf("Call Cost: $%.2f", *got.Cost)
	}
}

func TestEndOfCallReport_Unmarshal(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	t.Logf("cwd: %s", wd)

	fixtureFile := "/workspaces/talent-rodeo/testdata/vapi/webhooks/end-of-call-report/end-of-call-report_1743786911443605712.json"

	// Load fixture data
	fixtureData, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatalf("Failed to read fixture file: %v", err)
	}

	// First unmarshal into a wrapper struct
	var wrapper struct {
		Message EndOfCallReport `json:"message"`
	}
	if err := json.Unmarshal(fixtureData, &wrapper); err != nil {
		t.Fatalf("Failed to unmarshal EndOfCallReport wrapper: %v", err)
	}

	report := wrapper.Message

	// Validate required fields are present
	if report.Type == "" {
		t.Error("Type is required but was empty")
	}
	if report.EndedReason == "" {
		t.Error("EndedReason is required but was empty")
	}
	if report.Artifact.Messages == nil {
		t.Error("Artifact.Messages is required but was nil")
	}
	if report.Analysis.Summary == nil {
		t.Error("Analysis.Summary is required but was empty")
	}

	summary := *report.Analysis.Summary
	if len(summary) > 40 {
		summary = summary[:40] + "..."
	}
	// Log useful information for debugging
	t.Logf("Report Type: %s", report.Type)
	t.Logf("Ended Reason: %s", report.EndedReason)
	t.Logf("Number of Messages: %d", len(report.Artifact.Messages))
	t.Logf("Analysis Summary: %s", summary)
	if report.Cost != nil {
		t.Logf("Call Cost: $%.2f", *report.Cost)
	}
}
