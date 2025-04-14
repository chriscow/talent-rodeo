package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var credentialsFile string
	flag.StringVar(&credentialsFile, "creds", "", "Path to the Firebase service account credentials JSON file")
	flag.Parse()

	if credentialsFile == "" {
		return fmt.Errorf("credentials file path is required. Use -creds flag")
	}

	// Check if credentials file exists
	if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
		return fmt.Errorf("credentials file not found at %s: %w", credentialsFile, err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize Firebase app with credentials
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	// Initialize Firestore client
	client, err := app.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer client.Close()

	// Try to create a test document
	testDoc := map[string]interface{}{
		"message":   "Test connection",
		"timestamp": time.Now(),
	}

	// Add the test document to a test collection
	docRef, _, err := client.Collection("firebase_test").Add(ctx, testDoc)
	if err != nil {
		return fmt.Errorf("failed to add test document: %w", err)
	}

	// Try to retrieve the document we just created
	snapshot, err := docRef.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve test document: %w", err)
	}

	fmt.Printf("Successfully connected to Firebase!\n")
	fmt.Printf("Test document created with ID: %s\n", docRef.ID)
	fmt.Printf("Retrieved document data: %v\n", snapshot.Data())

	// Clean up - delete the test document
	if _, err := docRef.Delete(ctx); err != nil {
		// Don't fail the program for cleanup errors, just log them
		log.Printf("Warning: Failed to delete test document: %v", err)
	}

	return nil
}
