package cloudflare

import (
	"context"
	"fmt"
	"os"

	cf "github.com/cloudflare/cloudflare-go"
)

func KVSave(ctx context.Context, key string, value []byte) (*cf.Response, error) {
	cfApiKey := os.Getenv("CLOUDFLARE_GLOBAL_API_KEY")
	if cfApiKey == "" {
		return nil, fmt.Errorf("missing Cloudflare API key")
	}

	apiEmail := os.Getenv("CLOUDFLARE_API_EMAIL")
	if apiEmail == "" {
		return nil, fmt.Errorf("missing Cloudflare API email")
	}

	// Construct a new API object using a global API key
	api, err := cf.New(cfApiKey, apiEmail)
	// alternatively, you can use a scoped API token
	// api, err := cf.NewWithAPIToken(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare API object: %w", err)
	}

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if accountID == "" {
		return nil, fmt.Errorf("missing Cloudflare account ID")
	}

	// Set up the KV namespace ID
	namespaceID := os.Getenv("CLOUDFLARE_KV_NAMESPACE_ID")
	if namespaceID == "" {
		return nil, fmt.Errorf("missing Cloudflare KV namespace ID")
	}

	// Write a value to KV
	params := cf.WriteWorkersKVEntryParams{
		NamespaceID: namespaceID,
		Key:         key,
		Value:       value,
	}

	resp, err := api.WriteWorkersKVEntry(ctx, cf.AccountIdentifier(accountID), params)
	if err != nil {
		return nil, fmt.Errorf("failed to write to CloudFlare KV: %w", err)
	}

	return &resp, nil
}

func KVGet(ctx context.Context, key string) ([]byte, error) {
	cfApiKey := os.Getenv("CLOUDFLARE_GLOBAL_API_KEY")
	if cfApiKey == "" {
		return nil, fmt.Errorf("missing Cloudflare API key")
	}

	apiEmail := os.Getenv("CLOUDFLARE_API_EMAIL")
	if apiEmail == "" {
		return nil, fmt.Errorf("missing Cloudflare API email")
	}

	// Construct a new API object using a global API key
	api, err := cf.New(cfApiKey, apiEmail)
	// alternatively, you can use a scoped API token
	// api, err := cf.NewWithAPIToken(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare API object: %w", err)
	}

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if accountID == "" {
		return nil, fmt.Errorf("missing Cloudflare account ID")
	}

	// Set up the KV namespace ID
	namespaceID := os.Getenv("CLOUDFLARE_KV_NAMESPACE_ID")
	if namespaceID == "" {
		return nil, fmt.Errorf("missing Cloudflare KV namespace ID")
	}

	// Read a value from KV
	getParams := cf.GetWorkersKVParams{
		NamespaceID: namespaceID,
		Key:         key,
	}

	value, err := api.GetWorkersKV(ctx, cf.AccountIdentifier(accountID), getParams)
	if err != nil {
		return nil, fmt.Errorf("failed to read from CloudFlare KV: %w", err)
	}

	return value, nil
}
