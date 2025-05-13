package subscription

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestProcessorBasic(t *testing.T) {
	// This is a minimal test to verify the processor can be created and initialized
	ctx := context.Background()

	// Create a test Firestore client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create processor with a short interval for testing
	processor := NewProcessor(fs, logger).WithInterval(100 * time.Millisecond)

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
