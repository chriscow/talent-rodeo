package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const outboundSignatureHeader = "X-Outbound-Call-Signature"

type outboundCallRequest struct {
	AppointmentID string `json:"appointment_id"`
	UserID        string `json:"user_id"`
}

// signOutboundCall binds a scheduled task's exact request body to the server key.
func signOutboundCall(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyOutboundCallSignature(body []byte, signature, key string) bool {
	if key == "" || signature == "" {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	expected, err := hex.DecodeString(signOutboundCall(body, key))
	return err == nil && hmac.Equal(provided, expected)
}
