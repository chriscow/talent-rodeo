package internal

import (
	"testing"
)

func TestEncodeDecodePageToken(t *testing.T) {
	tests := []struct {
		name  string
		jobID string
	}{
		{
			name:  "simple job ID",
			jobID: "job123",
		},
		{
			name:  "complex job ID with special chars",
			jobID: "job-abc_123.xyz",
		},
		{
			name:  "firestore-style ID",
			jobID: "2TQVWLyoK8YrEpmM7x9k",
		},
		{
			name:  "empty string",
			jobID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the job ID
			encoded := EncodePageToken(tt.jobID)
			
			// Decode it back
			decoded, err := DecodePageToken(encoded)
			if err != nil {
				t.Fatalf("DecodePageToken() error = %v", err)
			}
			
			// Verify they match
			if decoded != tt.jobID {
				t.Errorf("DecodePageToken() = %v, want %v", decoded, tt.jobID)
			}
			
			// For non-empty strings, verify the encoded token is different from original
			if tt.jobID != "" && encoded == tt.jobID {
				t.Errorf("EncodePageToken() = %v, should be different from original %v", encoded, tt.jobID)
			}
		})
	}
}

func TestDecodePageTokenInvalid(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "invalid base64",
			token: "invalid-base64!!!",
		},
		{
			name:  "malformed token",
			token: "not-base64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodePageToken(tt.token)
			if err == nil {
				t.Errorf("DecodePageToken() should have returned an error for invalid token %v", tt.token)
			}
		})
	}
}
