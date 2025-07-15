package internal

import (
	"encoding/base64"
	"fmt"
)

// EncodePageToken encodes a job ID into a page token
func EncodePageToken(jobID string) string {
	if jobID == "" {
		return ""
	}
	return base64.URLEncoding.EncodeToString([]byte(jobID))
}

// DecodePageToken decodes a page token back to a job ID
func DecodePageToken(token string) (string, error) {
	if token == "" {
		return "", nil
	}

	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid page token: %w", err)
	}

	return string(decoded), nil
}
