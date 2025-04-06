package cloudflare

/*
import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
	"github.com/joho/godotenv"
)

const (
	// Cloudflare KV has eventual consistency, so we need to wait and retry
	maxRetries = 5
	retryDelay = 2 * time.Second
)

func setupTestEnv(t *testing.T) {
	t.Helper()

	// Load the env file for the tests
	if err := godotenv.Overload("../../.env"); err != nil {
		t.Fatalf("Error loading .env file: %v", err)
	}

	// Verify required env vars are present
	required := []string{
		"CLOUDFLARE_KV_API_TOKEN",
		"CLOUDFLARE_KV_NAMESPACE_ID",
		"CLOUDFLARE_ACCOUNT_ID",
	}

	for _, env := range required {
		if os.Getenv(env) == "" {
			t.Fatalf("Required environment variable %s is not set", env)
		}
	}
}

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// waitForKVOperation waits for a KV operation to propagate with retries
func waitForKVOperation(t *testing.T, kv *KV, key string, expectExists bool) error {
	t.Helper()

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		var val string
		err := kv.Get(key, &val)

		// If we expect the key to exist
		if expectExists && err == nil {
			return nil
		}

		// If we expect the key to not exist
		if !expectExists && err == ErrNotFound {
			return nil
		}

		lastErr = err
		time.Sleep(retryDelay)
	}

	if expectExists {
		return lastErr
	}
	return nil
}

// cleanupTestKeys removes test keys from KV store and waits for deletion to propagate
func cleanupTestKeys(t *testing.T, kv *KV, keys []string) {
	t.Helper()
	for _, key := range keys {
		if err := kv.Delete(key); err != nil && err != ErrNotFound {
			t.Logf("Failed to cleanup test key %s: %v", key, err)
			continue
		}

		// Wait for deletion to propagate
		if err := waitForKVOperation(t, kv, key, false); err != nil {
			t.Logf("Failed waiting for key deletion to propagate %s: %v", key, err)
		}
	}
}

func TestKV_Set(t *testing.T) {
	setupTestEnv(t)
	kv, err := NewKV()
	if err != nil {
		t.Fatalf("NewKV() failed: %v", err)
	}

	// Track test keys for cleanup
	testKeys := []string{"test-key", "test-struct", "test-exp"}

	// Clean up any leftover test keys first
	cleanupTestKeys(t, kv, testKeys)

	// Ensure cleanup runs after test
	t.Cleanup(func() {
		cleanupTestKeys(t, kv, testKeys)
	})

	tests := []struct {
		name      string
		key       string
		value     any
		exp       time.Duration
		wantError bool
	}{
		{
			name:      "empty key",
			key:       "",
			value:     "test",
			wantError: true,
		},
		{
			name:      "string value no expiration",
			key:       "test-key",
			value:     "test-value",
			wantError: false,
		},
		{
			name: "struct value with expiration",
			key:  "test-struct",
			value: testStruct{
				Name:  "test",
				Value: 42,
			},
			exp:       time.Hour,
			wantError: false,
		},
		{
			name:      "too short expiration",
			key:       "test-exp",
			value:     "test",
			exp:       time.Second * 30,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up the key before test if it exists
			if tt.key != "" {
				cleanupTestKeys(t, kv, []string{tt.key})
			}

			err := kv.Set(tt.key, tt.value, tt.exp)
			if tt.wantError {
				if err == nil {
					t.Error("Set() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Set() unexpected error: %v", err)
				return
			}

			// Verify the value was set correctly
			if tt.key != "" {
				if err := waitForKVOperation(t, kv, tt.key, true); err != nil {
					t.Errorf("Failed waiting for key to be set %s: %v", tt.key, err)
					return
				}

				// Verify the value
				switch v := tt.value.(type) {
				case string:
					var s string
					if err := kv.Get(tt.key, &s); err != nil {
						t.Errorf("Get() after Set() failed: %v", err)
						return
					}
					if s != v {
						t.Errorf("Get() after Set() = %v, want %v", s, v)
					}
				case testStruct:
					var ts testStruct
					if err := kv.Get(tt.key, &ts); err != nil {
						t.Errorf("Get() after Set() failed: %v", err)
						return
					}
					if ts != v {
						t.Errorf("Get() after Set() = %v, want %v", ts, v)
					}
				}
			}
		})
	}
}

func TestKV_Get(t *testing.T) {
	setupTestEnv(t)
	kv, err := NewKV()
	if err != nil {
		t.Fatalf("NewKV() failed: %v", err)
	}

	// Track test keys for cleanup
	testKeys := []string{"test-string", "test-struct", "expired-key"}

	// Clean up any leftover test keys first
	cleanupTestKeys(t, kv, testKeys)

	// Ensure cleanup runs after test
	t.Cleanup(func() {
		cleanupTestKeys(t, kv, testKeys)
	})

	// Test setup - store some values
	testStr := "test-value"
	testStructVal := testStruct{Name: "test", Value: 42}
	expiredStr := "expired-value"

	if err := kv.Set("test-string", testStr, 0); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if err := waitForKVOperation(t, kv, "test-string", true); err != nil {
		t.Fatalf("Failed waiting for test-string to be set: %v", err)
	}

	if err := kv.Set("test-struct", testStructVal, 0); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if err := waitForKVOperation(t, kv, "test-struct", true); err != nil {
		t.Fatalf("Failed waiting for test-struct to be set: %v", err)
	}

	if err := kv.Set("expired-key", expiredStr, time.Minute); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if err := waitForKVOperation(t, kv, "expired-key", true); err != nil {
		t.Fatalf("Failed waiting for expired-key to be set: %v", err)
	}

	// Force expiration by updating the ExpiresAt field directly
	entry := KVEntry{
		Value:     expiredStr,
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal expired entry: %v", err)
	}
	params := cf.WriteWorkersKVEntryParams{
		NamespaceID: kv.namespaceID,
		Key:         "expired-key",
		Value:       data,
	}
	_, err = kv.api.WriteWorkersKVEntry(context.Background(), cf.AccountIdentifier(kv.accountID), params)
	if err != nil {
		t.Fatalf("Failed to write expired entry: %v", err)
	}
	// Wait for the expired key update to propagate
	time.Sleep(retryDelay)

	tests := []struct {
		name      string
		key       string
		want      any
		wantError bool
	}{
		{
			name:      "empty key",
			key:       "",
			wantError: true,
		},
		{
			name:      "non-existent key",
			key:       "not-found",
			wantError: true,
		},
		{
			name: "get string",
			key:  "test-string",
			want: testStr,
		},
		{
			name: "get struct",
			key:  "test-struct",
			want: testStructVal,
		},
		{
			name:      "expired key",
			key:       "expired-key",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch want := tt.want.(type) {
			case string:
				var got string
				err := kv.Get(tt.key, &got)
				if tt.wantError {
					if err == nil {
						t.Error("Get() expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Errorf("Get() unexpected error: %v", err)
					return
				}
				if got != want {
					t.Errorf("Get() got = %v, want %v", got, want)
				}
			case testStruct:
				var got testStruct
				err := kv.Get(tt.key, &got)
				if tt.wantError {
					if err == nil {
						t.Error("Get() expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Errorf("Get() unexpected error: %v", err)
					return
				}
				if got != want {
					t.Errorf("Get() got = %v, want %v", got, want)
				}
			default:
				if !tt.wantError {
					t.Error("Get() test case missing type switch case")
					return
				}
				var got any
				err := kv.Get(tt.key, &got)
				if err == nil {
					t.Error("Get() expected error, got nil")
				}
			}
		})
	}
}

func TestKV_Delete(t *testing.T) {
	setupTestEnv(t)
	kv, err := NewKV()
	if err != nil {
		t.Fatalf("NewKV() failed: %v", err)
	}

	// Track test keys for cleanup
	testKeys := []string{"test-key"}

	// Clean up any leftover test keys first
	cleanupTestKeys(t, kv, testKeys)

	// Ensure cleanup runs after test
	t.Cleanup(func() {
		cleanupTestKeys(t, kv, testKeys)
	})

	// Test setup - store a value
	if err := kv.Set("test-key", "test-value", 0); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if err := waitForKVOperation(t, kv, "test-key", true); err != nil {
		t.Fatalf("Failed waiting for test-key to be set: %v", err)
	}

	tests := []struct {
		name      string
		key       string
		wantError bool
	}{
		{
			name:      "empty key",
			key:       "",
			wantError: true,
		},
		{
			name: "existing key",
			key:  "test-key",
		},
		{
			name: "non-existent key",
			key:  "not-found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := kv.Delete(tt.key)
			if tt.wantError {
				if err == nil {
					t.Error("Delete() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Delete() unexpected error: %v", err)
				return
			}

			// Wait for deletion to propagate and verify
			if err := waitForKVOperation(t, kv, tt.key, false); err != nil {
				t.Errorf("Failed waiting for deletion to propagate: %v", err)
				return
			}

			// Double check the key is really gone
			var got string
			err = kv.Get(tt.key, &got)
			if err != ErrNotFound {
				t.Errorf("after Delete(), Get() = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestKV_DeleteWithContext(t *testing.T) {
	setupTestEnv(t)
	kv, err := NewKV()
	if err != nil {
		t.Fatalf("NewKV() failed: %v", err)
	}

	// Track test keys for cleanup
	testKeys := []string{"test-key"}

	// Clean up any leftover test keys first
	cleanupTestKeys(t, kv, testKeys)

	// Ensure cleanup runs after test
	t.Cleanup(func() {
		cleanupTestKeys(t, kv, testKeys)
	})

	// Test setup - store a value
	if err := kv.Set("test-key", "test-value", 0); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	if err := waitForKVOperation(t, kv, "test-key", true); err != nil {
		t.Fatalf("Failed waiting for test-key to be set: %v", err)
	}

	tests := []struct {
		name      string
		key       string
		ctx       context.Context
		wantError bool
	}{
		{
			name:      "empty key",
			key:       "",
			ctx:       context.Background(),
			wantError: true,
		},
		{
			name: "existing key",
			key:  "test-key",
			ctx:  context.Background(),
		},
		{
			name:      "canceled context",
			key:       "test-key",
			ctx:       func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }(),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the test key for each test case if needed
			if tt.key == "test-key" {
				if err := kv.Set("test-key", "test-value", 0); err != nil {
					t.Fatalf("Set() failed: %v", err)
				}
				if err := waitForKVOperation(t, kv, "test-key", true); err != nil {
					t.Fatalf("Failed waiting for test-key to be set: %v", err)
				}
			}

			err := kv.DeleteWithContext(tt.ctx, tt.key)
			if tt.wantError {
				if err == nil {
					t.Error("DeleteWithContext() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("DeleteWithContext() unexpected error: %v", err)
				return
			}

			// Wait for deletion to propagate and verify
			if err := waitForKVOperation(t, kv, tt.key, false); err != nil {
				t.Errorf("Failed waiting for deletion to propagate: %v", err)
				return
			}

			// Double check the key is really gone
			var got string
			err = kv.Get(tt.key, &got)
			if err != ErrNotFound {
				t.Errorf("after DeleteWithContext(), Get() = %v, want ErrNotFound", err)
			}
		})
	}
}
*/
