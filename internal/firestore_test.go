package internal

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	os.Exit(code)
}

func TestFirestoreOperations(t *testing.T) {
	ctx := context.Background()

	// Create a new client using the emulator
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	defer client.Close()

	// Test collection operations
	t.Run("Collection Operations", func(t *testing.T) {
		col := client.Collection("test-collection")
		if col == nil {
			t.Error("Collection() returned nil")
		}

		// Test document creation
		doc := col.Doc("test-doc")
		_, err := doc.Set(ctx, map[string]interface{}{
			"field1": "value1",
			"field2": 42,
		})
		if err != nil {
			t.Errorf("Failed to set document: %v", err)
		}

		// Test document retrieval
		snapshot, err := doc.Get(ctx)
		if err != nil {
			t.Errorf("Failed to get document: %v", err)
		}

		data := snapshot.Data()
		if data["field1"] != "value1" || data["field2"] != int64(42) {
			t.Errorf("Document data mismatch. Got %v", data)
		}

		// Test document deletion
		_, err = doc.Delete(ctx)
		if err != nil {
			t.Errorf("Failed to delete document: %v", err)
		}
	})
}
