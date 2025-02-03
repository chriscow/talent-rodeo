package handlers

import (
	"context"
	"testing"
)

func TestMockTwilioClient_MakeCall(t *testing.T) {
	client := NewMockTwilioClient()
	ctx := context.Background()

	t.Run("successful call", func(t *testing.T) {
		from := "+1234567890"
		to := "+9876543210"
		webhook := "https://example.com/webhook"

		callSID, err := client.MakeCall(ctx, from, to, webhook)
		if err != nil {
			t.Errorf("MakeCall() error = %v; want nil", err)
		}
		if got := callSID; got != "test_call_"+to {
			t.Errorf("MakeCall() callSID = %v; want %v", got, "test_call_"+to)
		}

		// Verify call status is queued
		status, err := client.GetCallStatus(ctx, callSID)
		if err != nil {
			t.Errorf("GetCallStatus() error = %v; want nil", err)
		}
		if got := status; got != "queued" {
			t.Errorf("GetCallStatus() status = %v; want queued", got)
		}
	})
}

func TestMockTwilioClient_GetCallStatus(t *testing.T) {
	client := NewMockTwilioClient()
	ctx := context.Background()

	t.Run("call not found", func(t *testing.T) {
		_, err := client.GetCallStatus(ctx, "nonexistent_call")
		if err != ErrCallNotFound {
			t.Errorf("GetCallStatus() error = %v; want %v", err, ErrCallNotFound)
		}
	})

	t.Run("call status changes", func(t *testing.T) {
		callSID := "test_call"
		client.SetCallStatus(callSID, "in-progress")

		status, err := client.GetCallStatus(ctx, callSID)
		if err != nil {
			t.Errorf("GetCallStatus() error = %v; want nil", err)
		}
		if got := status; got != "in-progress" {
			t.Errorf("GetCallStatus() status = %v; want in-progress", got)
		}

		// Update status
		client.SetCallStatus(callSID, "completed")
		status, err = client.GetCallStatus(ctx, callSID)
		if err != nil {
			t.Errorf("GetCallStatus() error = %v; want nil", err)
		}
		if got := status; got != "completed" {
			t.Errorf("GetCallStatus() status = %v; want completed", got)
		}
	})
}

func TestMockTwilioClient_EndCall(t *testing.T) {
	client := NewMockTwilioClient()
	ctx := context.Background()

	t.Run("end nonexistent call", func(t *testing.T) {
		err := client.EndCall(ctx, "nonexistent_call")
		if err != ErrCallNotFound {
			t.Errorf("EndCall() error = %v; want %v", err, ErrCallNotFound)
		}
	})

	t.Run("end active call", func(t *testing.T) {
		// Set up an active call
		callSID := "test_call"
		client.SetCallStatus(callSID, "in-progress")

		// End the call
		err := client.EndCall(ctx, callSID)
		if err != nil {
			t.Errorf("EndCall() error = %v; want nil", err)
		}

		// Verify call status is completed
		status, err := client.GetCallStatus(ctx, callSID)
		if err != nil {
			t.Errorf("GetCallStatus() error = %v; want nil", err)
		}
		if got := status; got != "completed" {
			t.Errorf("GetCallStatus() status = %v; want completed", got)
		}
	})
}
