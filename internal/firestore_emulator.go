package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"cloud.google.com/go/firestore"
)

var (
	emulatorCmd *exec.Cmd
	emulatorWg  sync.WaitGroup
)

const (
	firestoreEmulatorHost = "FIRESTORE_EMULATOR_HOST"
	testProjectID         = "test-project"
)

// NewTestFirestoreClient creates a new Firestore client for testing using the emulator
func NewTestFirestoreClient(ctx context.Context) (*FirestoreClient, error) {
	// Ensure we're using the emulator
	if os.Getenv(firestoreEmulatorHost) == "" {
		return nil, fmt.Errorf("FIRESTORE_EMULATOR_HOST not set")
	}

	// Suppress debug output in CLI mode to avoid contaminating JSON output
	if os.Getenv("CLI_JSON_MODE") == "" {
		println("Using Firestore emulator at", os.Getenv(firestoreEmulatorHost))
	}

	client, err := firestore.NewClient(ctx, testProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create test firestore client: %w", err)
	}

	return &FirestoreClient{
		Client: client,
	}, nil
}
