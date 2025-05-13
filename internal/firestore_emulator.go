package internal

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

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

	println("Using Firestore emulator at", os.Getenv(firestoreEmulatorHost))

	client, err := firestore.NewClient(ctx, testProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create test firestore client: %w", err)
	}

	return &FirestoreClient{
		Client: client,
	}, nil
}

// StartEmulator starts the Firestore emulator and waits for it to be ready
func StartEmulator() error {
	return nil
	// Command to start firestore emulator without specifying host-port
	// Let it choose its own port to avoid conflicts
	emulatorCmd = exec.Command("gcloud", "beta", "emulators", "firestore", "start")

	// Make it killable
	emulatorCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture output to know when it's started
	stderr, err := emulatorCmd.StderrPipe()
	if err != nil {
		return err
	}

	// Start the emulator
	if err := emulatorCmd.Start(); err != nil {
		return err
	}

	// Wait until emulator is running
	emulatorWg.Add(1)

	go func() {
		buf := make([]byte, 256)
		for {
			n, err := stderr.Read(buf[:])
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("reading stderr %v", err)
				break
			}

			if n > 0 {
				d := string(buf[:n])
				log.Printf("%s", d)

				// Look for the export command line which contains the host:port
				if strings.Contains(d, "export FIRESTORE_EMULATOR_HOST=") {
					// Extract the host:port from the line
					// Format: export FIRESTORE_EMULATOR_HOST=[::1]:8875
					parts := strings.Split(d, "=")
					if len(parts) == 2 {
						// Clean up the host value - remove any trailing text or newlines
						host := strings.TrimSpace(parts[1])
						host = strings.Split(host, "\n")[0] // Take only the first line
						host = strings.TrimSuffix(host, "[firestore]")
						host = strings.TrimSpace(host)
						os.Setenv(firestoreEmulatorHost, host)
						log.Printf("Set %s=%s", firestoreEmulatorHost, host)
					}
				}

				if strings.Contains(d, "Dev App Server is now running") {
					emulatorWg.Done()
				}
			}
		}
	}()

	emulatorWg.Wait()

	// Verify we got the host
	if os.Getenv(firestoreEmulatorHost) == "" {
		return fmt.Errorf("failed to get emulator host from output")
	}

	return nil
}

// StopEmulator stops the Firestore emulator
func StopEmulator() {
	if emulatorCmd != nil && emulatorCmd.Process != nil {
		syscall.Kill(-emulatorCmd.Process.Pid, syscall.SIGKILL)
		emulatorCmd = nil
	}
}

// // NewTestFirestoreClient creates a new Firestore client for testing using the emulator
// func NewTestFirestoreClient(ctx context.Context) (*FirestoreClient, error) {
// 	// Ensure we're using the emulator
// 	if os.Getenv(firestoreEmulatorHost) == "" {
// 		return nil, fmt.Errorf("FIRESTORE_EMULATOR_HOST not set")
// 	}

// 	client, err := firestore.NewClient(ctx, testProjectID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create test firestore client: %w", err)
// 	}

// 	return &FirestoreClient{
// 		Client: client,
// 	}, nil
// }
