package vapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// loadTestData loads test data from a JSON file
func loadTestData(filename string, v any) error {
	data, err := os.ReadFile(filepath.Join("/workspaces/talent-rodeo/testdata", "vapi", filename))
	if err != nil {
		return fmt.Errorf("failed to read test data: %w", err)
	}
	return json.Unmarshal(data, v)
}

const (
	VoiceMailDetectionProviderTwilio = "twilio"
)

// CreateCall creates a new call with the given configuration
func CreateCall(ctx context.Context, call Call) (*Call, error) {
	if os.Getenv("TESTING_MODE") == "true" {
		var result Call
		if err := loadTestData("create-call-response.json", &result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	b, err := json.Marshal(call)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(b)
	req, err := http.NewRequest("POST", "https://api.vapi.ai/call", buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for call: %w", err)
	}

	apiKey := os.Getenv("VAPI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("VAPI_API_KEY not set")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to http client failed: %w", err)
	}
	defer resp.Body.Close()

	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create call. code: %d msg: %s", resp.StatusCode, body.String())
	}

	var result Call
	if err := json.Unmarshal(body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetCall retrieves a call by its ID
func GetCall(ctx context.Context, id string) (*Call, error) {
	if os.Getenv("TESTING_MODE") == "true" {
		var result Call
		if err := loadTestData("get-call-response.json", &result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.vapi.ai/call/%s", id), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for call: %w", err)
	}

	apiKey := os.Getenv("VAPI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("VAPI_API_KEY not set")
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to http client failed: %w", err)
	}
	defer resp.Body.Close()

	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get call. code: %d msg: %s", resp.StatusCode, body.String())
	}

	var result Call
	if err := json.Unmarshal(body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// SimulateEndOfCallWebhook simulates an end-of-call webhook in test mode
func SimulateEndOfCallWebhook(webhookURL string) error {
	if os.Getenv("TESTING_MODE") != "true" {
		return fmt.Errorf("can only simulate webhooks in test mode")
	}

	// Create a request with the test header
	req, err := http.NewRequest("POST", webhookURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add header to indicate this is a simulated end-of-call
	req.Header.Set("X-Vapi-Simulate", "end-of-call")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
