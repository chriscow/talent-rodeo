package main

import "testing"

func TestOutboundCallSignature(t *testing.T) {
	key := "session-secret-for-scheduled-task-authentication"
	body := []byte("{\"appointment_id\":\"a1\",\"user_id\":\"u1\"}")
	signature := signOutboundCall(body, key)
	tests := []struct {
		name      string
		body      []byte
		signature string
		key       string
		want      bool
	}{
		{name: "valid", body: body, signature: signature, key: key, want: true},
		{name: "changed body", body: []byte("{\"appointment_id\":\"a2\",\"user_id\":\"u1\"}"), signature: signature, key: key},
		{name: "wrong key", body: body, signature: signature, key: "different-key"},
		{name: "missing signature", body: body, key: key},
		{name: "missing key", body: body, signature: signature},
		{name: "invalid hex", body: body, signature: "not-hex", key: key},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verifyOutboundCallSignature(tt.body, tt.signature, tt.key); got != tt.want {
				t.Errorf("verifyOutboundCallSignature(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
