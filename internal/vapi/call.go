package vapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// CallRequest represents the top-level request structure
type CallConfig struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"orgId"`
	AssistantID        string    `json:"assistantId"`
	Assistant          Assistant `json:"assistant"`
	AssistantOverrides Assistant `json:"assistantOverrides"`
	SquadID            string    `json:"squadId"`
	Squad              Squad     `json:"squad"`
	PhoneNumberID      string    `json:"phoneNumberId"`
	Customer           Customer  `json:"customer"`
	Name               string    `json:"name"`
}

func CreateCall(call CallConfig) error {
	// curl -X POST https://api.vapi.ai/call \
	// -H "Authorization: Bearer <token>" \
	// -H "Content-Type: application/json" \
	// -d '{}'
	b, err := json.Marshal(call)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(b)
	req, err := http.NewRequest("POST", "https://api.vapi.ai/call", buf)
	if err != nil {
		return fmt.Errorf("failed to create request for call: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to http client failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create call: %w", err)
	}

	return nil
}

// Customer contains customer information
type Customer struct {
	NumberE164CheckEnabled bool   `json:"numberE164CheckEnabled"`
	Extension              string `json:"extension"`
	Number                 string `json:"number"`
	SipURI                 string `json:"sipUri"`
	Name                   string `json:"name"`
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
