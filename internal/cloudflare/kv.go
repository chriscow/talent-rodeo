package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
)

// Common errors
var (
	ErrNotFound = errors.New("key not found in KV store")
)

// KVEntry represents a value stored in KV with expiration
type KVEntry struct {
	Value     any   `json:"value"`
	ExpiresAt int64 `json:"expires_at,omitempty"`
	Metadata  any   `json:"metadata,omitempty"`
}

// KV represents a Cloudflare KV namespace client
type KV struct {
	api         *cf.API
	accountID   string
	namespaceID string
}

// NewKV creates a new KV client
func NewKV() (*KV, error) {
	apiToken := os.Getenv("CLOUDFLARE_KV_API_TOKEN")
	if apiToken == "" {
		return nil, fmt.Errorf("CLOUDFLARE_KV_API_TOKEN is required")
	}

	api, err := cf.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare API object: %w", err)
	}

	namespaceID := os.Getenv("CLOUDFLARE_KV_NAMESPACE_ID")
	if namespaceID == "" {
		return nil, fmt.Errorf("missing Cloudflare KV namespace ID")
	}

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if accountID == "" {
		return nil, fmt.Errorf("missing Cloudflare account ID")
	}

	return &KV{
		api:         api,
		accountID:   accountID,
		namespaceID: namespaceID,
	}, nil
}

// Set stores a value in KV with optional expiration
func (kv *KV) Set(key string, value any, expiration time.Duration) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	entry := KVEntry{
		Value: value,
	}

	if expiration > 0 {
		if expiration < time.Minute {
			return fmt.Errorf("expiration must be at least 60 seconds")
		}
		entry.ExpiresAt = time.Now().Add(expiration).Unix()
	}

	// Marshal the entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	params := cf.WriteWorkersKVEntryParams{
		NamespaceID: kv.namespaceID,
		Key:         key,
		Value:       data,
	}

	_, err = kv.api.WriteWorkersKVEntry(context.Background(), cf.AccountIdentifier(kv.accountID), params)
	if err != nil {
		return fmt.Errorf("failed to write to CloudFlare KV: %w", err)
	}

	return nil
}

// Get retrieves a value from KV and unmarshals it into the provided interface
func (kv *KV) Get(key string, value any) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	params := cf.GetWorkersKVParams{
		NamespaceID: kv.namespaceID,
		Key:         key,
	}

	data, err := kv.api.GetWorkersKV(context.Background(), cf.AccountIdentifier(kv.accountID), params)
	if err != nil {
		// Check for various "not found" error messages
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "key not found") || strings.Contains(errStr, "not found") {
			return ErrNotFound
		}
		return fmt.Errorf("failed to read from CloudFlare KV: %w", err)
	}

	var entry KVEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// If we can't unmarshal as a KVEntry, try direct unmarshal
		if err := json.Unmarshal(data, value); err != nil {
			return fmt.Errorf("failed to unmarshal value: %w", err)
		}
		return nil
	}

	// Check expiration
	if entry.ExpiresAt > 0 && time.Now().Unix() > entry.ExpiresAt {
		// Delete expired entry in the background
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = kv.DeleteWithContext(ctx, key)
		}()
		return ErrNotFound
	}

	// For simple types like string, try direct assignment
	if str, ok := entry.Value.(string); ok {
		if pstr, ok := value.(*string); ok {
			*pstr = str
			return nil
		}
	}

	// For complex types, use JSON marshal/unmarshal
	valueData, err := json.Marshal(entry.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal entry value: %w", err)
	}

	if err := json.Unmarshal(valueData, value); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a key from KV
func (kv *KV) Delete(key string) error {
	return kv.DeleteWithContext(context.Background(), key)
}

// DeleteWithContext removes a key from KV with context
func (kv *KV) DeleteWithContext(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	params := cf.DeleteWorkersKVEntryParams{
		NamespaceID: kv.namespaceID,
		Key:         key,
	}

	_, err := kv.api.DeleteWorkersKVEntry(ctx, cf.AccountIdentifier(kv.accountID), params)
	if err != nil {
		// Ignore "key not found" errors on delete
		if err.Error() == "workers kv key not found" || err.Error() == "get: 'key not found' (10009)" {
			return nil
		}
		return fmt.Errorf("failed to delete from CloudFlare KV: %w", err)
	}

	return nil
}
