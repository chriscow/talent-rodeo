package subscription

import (
	"context"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestProcessor(t *testing.T) {
	// This is a minimal test to verify the processor can be created and initialized
	// For a full integration test, we would need a mock Firestore client
	ctx := context.Background()

	// Create a test Firestore client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Create processor with a short interval for testing
	processor := NewProcessor(fs).WithInterval(100 * time.Millisecond)

	// Test that we can start and stop the processor
	err = processor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Wait briefly to allow processor to run once
	time.Sleep(200 * time.Millisecond)

	// Stop the processor
	processor.Stop()

	// Verify processor is no longer running
	if processor.isRunning {
		t.Errorf("Processor should not be running after Stop")
	}
}

// For a full integration test, you would add:
// - Mock data for search criteria
// - Mock subscriptions
// - Mock users with steering queries
// - Verification of job creation
