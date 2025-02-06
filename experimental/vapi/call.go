package vapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// CallRequest represents the top-level request structure
type CallConfig struct {
	ID                 *string    `json:"id,omitempty"`
	OrgID              *string    `json:"orgId,omitempty"`
	AssistantID        *string    `json:"assistantId,omitempty"`
	Assistant          *Assistant `json:"assistant,omitempty"`
	AssistantOverrides *Assistant `json:"assistantOverrides,omitempty"`
	SquadID            *string    `json:"squadId,omitempty"`
	Squad              *Squad     `json:"squad,omitempty"`
	PhoneNumberID      *string    `json:"phoneNumberId,omitempty"`
	Customer           *Customer  `json:"customer,omitempty"`
	Name               *string    `json:"name,omitempty"`
}

func CreateCall(ctx context.Context, call CallConfig) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// curl -X POST https://api.vapi.ai/call \
	// -H "Authorization: Bearer <token>" \
	// -H "Content-Type: application/json" \
	// -d '{}'
	b, err := json.Marshal(call)
	if err != nil {
		return nil, err
	}

	println(string(b))

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

	if resp.StatusCode != http.StatusOK {
		// read the response body

		return nil, fmt.Errorf("failed to create call. code: %d msg: %s", resp.StatusCode, body.String())
	}

	return body.Bytes(), nil
}

// Customer contains customer information
type Customer struct {
	NumberE164CheckEnabled *bool   `json:"numberE164CheckEnabled,omitempty"`
	Extension              *string `json:"extension,omitempty"`
	Number                 string  `json:"number"`
	SipURI                 *string `json:"sipUri,omitempty"`
	Name                   string  `json:"name"`
}

// Additional required types...
type KnowledgeBase struct {
	Server ServerConfig `json:"server"`
}

type Tool struct {
	Type  string `json:"type"`
	Async bool   `json:"async"`
}

type ChunkPlan struct {
	Enabled       bool `json:"enabled"`
	MinCharacters int  `json:"minCharacters"`
}

type FallbackPlan struct {
	Voices []FallbackVoice `json:"voices"`
}

type FallbackVoice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voiceId"`
}

type TransportConfig struct {
	Provider string `json:"provider"`
	Timeout  int    `json:"timeout"`
	Record   bool   `json:"record"`
}

type ArtifactPlan struct {
	RecordingEnabled      bool           `json:"recordingEnabled"`
	VideoRecordingEnabled bool           `json:"videoRecordingEnabled"`
	TranscriptPlan        TranscriptPlan `json:"transcriptPlan"`
	RecordingPath         string         `json:"recordingPath"`
}

type TranscriptPlan struct {
	Enabled bool `json:"enabled"`
}

type MessagePlan struct {
	IdleMessages              []string `json:"idleMessages"`
	IdleMessageMaxSpokenCount float64  `json:"idleMessageMaxSpokenCount"`
	IdleTimeoutSeconds        float64  `json:"idleTimeoutSeconds"`
}

type CustomEndpointingRule struct {
	Type           string  `json:"type"`
	AssistantRegex string  `json:"assistantRegex"`
	CustomerRegex  string  `json:"customerRegex"`
	TimeoutSeconds float64 `json:"timeoutSeconds"`
}

type MonitorPlan struct {
	ListenEnabled  bool `json:"listenEnabled"`
	ControlEnabled bool `json:"controlEnabled"`
}

type Squad struct {
	Members          []interface{}    `json:"members"`
	Name             string           `json:"name"`
	MembersOverrides MembersOverrides `json:"membersOverrides"`
}

// AssistantOverrides and MembersOverrides can contain any of the Assistant fields
type AssistantOverrides struct {
	Assistant
}

type MembersOverrides struct {
	Assistant
}
