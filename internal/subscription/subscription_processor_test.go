package subscription

import (
	"talent-rodeo/internal/cloudflare"
	"testing"
	"time"
)

// TestFeatureFlagFromCloudflareKV is an integration test that checks
// if we can read the feature flag from Cloudflare KV
func TestFeatureFlagFromCloudflareKV(t *testing.T) {
	// Create a KV client
	kv, err := cloudflare.NewKV()
	if err != nil {
		t.Fatalf("Failed to create Cloudflare KV client: %v", err)
	}

	// Set a test value
	flagKey := SubscriptionProcessorKey
	testValue := true

	// Set with a short expiration for testing purposes
	err = kv.Set(flagKey, testValue, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to set test flag value: %v", err)
	}
	t.Logf("Set test value %t for key %s", testValue, flagKey)

	// Try to read the value
	var readValue bool
	err = kv.Get(flagKey, &readValue)
	if err != nil {
		t.Fatalf("Failed to read flag value: %v", err)
	}

	// Verify the value matches what we set
	if readValue != testValue {
		t.Errorf("Read value %t doesn't match set value %t", readValue, testValue)
	} else {
		t.Logf("Successfully read flag value: %t", readValue)
	}

	// We don't need to test the processor directly since it requires a real Firestore client
	// Just test the KV store functionality which is what we care about

	// Test direct feature flag reading
	var isEnabled bool
	err = kv.Get(SubscriptionProcessorKey, &isEnabled)
	if err != nil && err != cloudflare.ErrNotFound {
		t.Errorf("Error reading flag directly: %v", err)
	} else if err == nil {
		t.Logf("Successfully read flag directly: %t", isEnabled)
	}

	// Clean up - delete the test flag
	err = kv.Delete(flagKey)
	if err != nil {
		t.Logf("Warning: failed to delete test flag: %v", err)
	}
}
