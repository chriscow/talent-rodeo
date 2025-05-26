package internal

import (
	"context"
	"testing"
)

func TestExtractGeoID(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	geoid := GeoID{
		ID:    "12345678",
		Value: "San Francisco, CA",
	}
	if err := CreateGeoID(ctx, client, geoid); err != nil {
		t.Fatalf("Failed to create geo ID: %v", err)
	}

	got, err := ExtractGeoID(ctx, client, "I'm looking for a job in San Francisco")
	if err != nil {
		t.Fatalf("Failed to extract geo ID: %v", err)
	}

	want := "12345678"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestPopulateGeoIDs(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client with emulator
	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Define the initial GeoIDs to populate
	geoIDs := []struct {
		id    string
		value string
	}{
		{"Remote", "103644278"},
		{"United States", "103644278"},
		{"Greater New York Area", ""},
		{"Greater Los Angeles Area", ""},
		{"Greater Chicago Area", ""},
		{"Greater Dallas-Fort Worth Area", ""},
		{"Greater Houston Area", ""},
		{"Greater Washington DC Area", ""},
		{"Greater Philadelphia Area", ""},
		{"Greater San Francisco Area", ""},
		{"Greater Miami Area", ""},
		{"Greater Phoenix Area", ""},
		{"Greater Boston Area", ""},
		{"Greater Atlanta Area", ""},
		{"Greater Detroit Area", ""},
		{"Greater Seattle Area", ""},
		{"Greater Minneapolis-St. Paul Area", ""},
		{"Greater San Diego Area", ""},
		{"Greater Tampa Area", ""},
		{"Greater Denver Area", ""},
		{"Greater Baltimore Area", ""},
	}

	// Batch write to avoid too many individual writes
	bulkWriter := client.BulkWriter(ctx)

	// Add each GeoID to the bulk writer
	for _, geoID := range geoIDs {
		doc := client.Collection("geo_ids").Doc(geoID.id)
		_, err := bulkWriter.Set(doc, map[string]interface{}{
			"id":    geoID.id,
			"value": geoID.value,
		})
		if err != nil {
			t.Fatalf("Failed to add to bulk writer: %v", err)
		}
	}

	// Commit the bulk writer
	bulkWriter.End()
	bulkWriter.Flush()

	// Verify the data was written correctly
	gotGeoIDs, err := ListGeoIDs(ctx, client)
	if err != nil {
		t.Fatalf("Failed to list GeoIDs: %v", err)
	}

	// Create a map for easier comparison
	gotMap := make(map[string]string)
	for _, geoID := range gotGeoIDs {
		gotMap[geoID.ID] = geoID.Value
	}

	// Verify each expected GeoID exists with correct value
	for _, want := range geoIDs {
		got, exists := gotMap[want.id]
		if !exists {
			t.Errorf("GeoID %q not found in Firestore", want.id)
			continue
		}
		if got != want.value {
			t.Errorf("GeoID %q: got value %q, want %q", want.id, got, want.value)
		}
	}
}
