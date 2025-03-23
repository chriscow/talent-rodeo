package vapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// ServerConfig defines the configuration for a server endpoint
type ServerConfig struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// Voice defines voice settings for the assistant
type Voice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voiceId"`
}

// Model defines the LLM model settings
type Model struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Transcriber defines the speech-to-text settings
type Transcriber struct {
	Provider string `json:"provider"`
}

// VoicemailDetection defines voicemail detection settings
type VoicemailDetection struct {
	Provider                         string  `json:"provider"`
	VoicemailExpectedDurationSeconds float64 `json:"voicemailExpectedDurationSeconds"`
}

// KnowledgeBase represents the knowledge base configuration
type KnowledgeBase struct {
	Server ServerConfig `json:"server"`
}

// Tool represents a tool configuration
type Tool struct {
	Type  string `json:"type"`
	Async bool   `json:"async"`
}

// ChunkPlan represents the chunk configuration
type ChunkPlan struct {
	Enabled       bool `json:"enabled"`
	MinCharacters int  `json:"minCharacters"`
}

// FallbackPlan represents the fallback configuration
type FallbackPlan struct {
	Voices []FallbackVoice `json:"voices"`
}

// FallbackVoice represents a fallback voice configuration
type FallbackVoice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voiceId"`
}

// TransportConfig represents transport configuration
type TransportConfig struct {
	Provider string `json:"provider"`
	Timeout  int    `json:"timeout"`
	Record   bool   `json:"record"`
}

// MessagePlan represents message configuration
type MessagePlan struct {
	IdleMessages              []string `json:"idleMessages"`
	IdleMessageMaxSpokenCount float64  `json:"idleMessageMaxSpokenCount"`
	IdleTimeoutSeconds        float64  `json:"idleTimeoutSeconds"`
}

// CustomEndpointingRule represents custom endpointing configuration
type CustomEndpointingRule struct {
	Type           string  `json:"type"`
	AssistantRegex string  `json:"assistantRegex"`
	CustomerRegex  string  `json:"customerRegex"`
	TimeoutSeconds float64 `json:"timeoutSeconds"`
}

// MonitorPlan represents monitoring configuration
type MonitorPlan struct {
	ListenEnabled  bool `json:"listenEnabled"`
	ControlEnabled bool `json:"controlEnabled"`
}

// Squad represents a squad configuration
type Squad struct {
	Members          []any            `json:"members"`
	Name             string           `json:"name"`
	MembersOverrides MembersOverrides `json:"membersOverrides"`
}

// AssistantOverrides represents assistant override configuration
type AssistantOverrides struct {
	Assistant
}

// MembersOverrides represents member override configuration
type MembersOverrides struct {
	Assistant
}

// ArtifactPlan represents the configuration for call artifacts
type ArtifactPlan struct {
	RecordingEnabled      bool           `json:"recordingEnabled"`
	RecordingFormat       string         `json:"recordingFormat"`
	VideoRecordingEnabled bool           `json:"videoRecordingEnabled"`
	PcapEnabled           bool           `json:"pcapEnabled"`
	PcapS3PathPrefix      string         `json:"pcapS3PathPrefix"`
	TranscriptPlan        TranscriptPlan `json:"transcriptPlan"`
	RecordingPath         string         `json:"recordingPath"`
}

// TranscriptPlan represents the configuration for call transcripts
type TranscriptPlan struct {
	Enabled       bool   `json:"enabled"`
	AssistantName string `json:"assistantName,omitempty"`
	UserName      string `json:"userName,omitempty"`
}

// Message represents a message in the call
type Message struct {
	Role             string  `json:"role"`
	Message          string  `json:"message"`
	Time             float64 `json:"time"`
	EndTime          float64 `json:"endTime"`
	SecondsFromStart float64 `json:"secondsFromStart"`
	Duration         float64 `json:"duration"`
}

// Customer contains customer information
type Customer struct {
	NumberE164CheckEnabled *bool   `json:"numberE164CheckEnabled,omitempty"`
	Extension              *string `json:"extension,omitempty"`
	Number                 string  `json:"number"`
	SipURI                 *string `json:"sipUri,omitempty"`
	Name                   string  `json:"name"`
}

// CallConfig represents the top-level request structure
type CallConfig struct {
	ID                 *string    `json:"id,omitempty"`
	OrgID              *string    `json:"orgId,omitempty"`
	AssistantID        *string    `json:"assistantId,omitempty"`
	Assistant          *Assistant `json:"assistant,omitempty"`
	AssistantOverrides *Assistant `json:"assistantOverrides,omitempty"`
	SquadID            *string    `json:"squadId,omitempty"`
	Squad              *Squad     `json:"squad,omitempty"`
	PhoneNumberID      *string    `json:"phoneNumberId,omitempty"`
	CustomerID         *string    `json:"customerId,omitempty"`
	Customer           *Customer  `json:"customer,omitempty"`
	Name               *string    `json:"name,omitempty"`
}

// CallResult represents the response from creating a call
type CallResult struct {
	ID                  string        `json:"id"`
	OrgID               string        `json:"orgId"`
	CreatedAt           string        `json:"createdAt"`
	UpdatedAt           string        `json:"updatedAt"`
	Type                string        `json:"type"`
	Costs               []Cost        `json:"costs"`
	Messages            []Message     `json:"messages"`
	PhoneCallProvider   string        `json:"phoneCallProvider"`
	PhoneCallTransport  string        `json:"phoneCallTransport"`
	Status              string        `json:"status"`
	EndedReason         string        `json:"endedReason"`
	Destination         Destination   `json:"destination"`
	StartedAt           string        `json:"startedAt"`
	EndedAt             string        `json:"endedAt"`
	Cost                float64       `json:"cost"`
	CostBreakdown       CostBreakdown `json:"costBreakdown"`
	ArtifactPlan        ArtifactPlan  `json:"artifactPlan"`
	Analysis            Analysis      `json:"analysis"`
	Monitor             Monitor       `json:"monitor"`
	Artifact            Artifact      `json:"artifact"`
	Transport           Transport     `json:"transport"`
	PhoneCallProviderID string        `json:"phoneCallProviderId"`
	AssistantID         string        `json:"assistantId"`
	Assistant           Assistant     `json:"assistant"`
	AssistantOverrides  Assistant     `json:"assistantOverrides"`
	SquadID             string        `json:"squadId"`
	Squad               Squad         `json:"squad"`
	PhoneNumberID       string        `json:"phoneNumberId"`
	PhoneNumber         PhoneNumber   `json:"phoneNumber"`
	CustomerID          string        `json:"customerId"`
	Customer            Customer      `json:"customer"`
	Name                string        `json:"name"`
}

// CreateCall creates a new call with the given configuration
func CreateCall(ctx context.Context, call CallConfig) (*CallResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create call. code: %d msg: %s", resp.StatusCode, body.String())
	}

	var result CallResult
	if err := json.Unmarshal(body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetCall retrieves a call by its ID
func GetCall(ctx context.Context, id string) (*CallResult, error) {
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

	var result CallResult
	if err := json.Unmarshal(body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// Cost represents a cost entry in the call result
type Cost struct {
	Type     string  `json:"type"`
	Cost     float64 `json:"cost"`
	Minutes  float64 `json:"minutes"`
	Provider string  `json:"provider"`
}

// Destination represents the call destination details
type Destination struct {
	Type                   string       `json:"type"`
	Number                 string       `json:"number"`
	CallerID               string       `json:"callerId"`
	Description            string       `json:"description"`
	Extension              string       `json:"extension"`
	Message                string       `json:"message"`
	NumberE164CheckEnabled bool         `json:"numberE164CheckEnabled"`
	TransferPlan           TransferPlan `json:"transferPlan"`
}

// TransferPlan represents the transfer configuration
type TransferPlan struct {
	Mode    string         `json:"mode"`
	Message string         `json:"message"`
	SipVerb map[string]any `json:"sipVerb"`
	Twiml   string         `json:"twiml"`
}

// CostBreakdown represents the detailed cost breakdown
type CostBreakdown struct {
	Transport             float64               `json:"transport"`
	STT                   float64               `json:"stt"`
	LLM                   float64               `json:"llm"`
	TTS                   float64               `json:"tts"`
	VAPI                  float64               `json:"vapi"`
	Total                 float64               `json:"total"`
	LLMPromptTokens       float64               `json:"llmPromptTokens"`
	LLMCompletionTokens   float64               `json:"llmCompletionTokens"`
	TTSCharacters         float64               `json:"ttsCharacters"`
	AnalysisCostBreakdown AnalysisCostBreakdown `json:"analysisCostBreakdown"`
}

// AnalysisCostBreakdown represents the cost breakdown for analysis
type AnalysisCostBreakdown struct {
	Summary                           float64 `json:"summary"`
	SummaryPromptTokens               float64 `json:"summaryPromptTokens"`
	SummaryCompletionTokens           float64 `json:"summaryCompletionTokens"`
	StructuredData                    float64 `json:"structuredData"`
	StructuredDataPromptTokens        float64 `json:"structuredDataPromptTokens"`
	StructuredDataCompletionTokens    float64 `json:"structuredDataCompletionTokens"`
	SuccessEvaluation                 float64 `json:"successEvaluation"`
	SuccessEvaluationPromptTokens     float64 `json:"successEvaluationPromptTokens"`
	SuccessEvaluationCompletionTokens float64 `json:"successEvaluationCompletionTokens"`
}

// Analysis represents the call analysis results
type Analysis struct {
	Summary             string           `json:"summary"`
	StructuredData      map[string]any   `json:"structuredData"`
	StructuredDataMulti []map[string]any `json:"structuredDataMulti"`
	SuccessEvaluation   string           `json:"successEvaluation"`
}

// Monitor represents monitoring URLs
type Monitor struct {
	ListenURL  string `json:"listenUrl"`
	ControlURL string `json:"controlUrl"`
}

// Artifact represents call artifacts
type Artifact struct {
	Messages                        []Message `json:"messages"`
	MessagesOpenAIFormatted         []Message `json:"messagesOpenAIFormatted"`
	RecordingURL                    string    `json:"recordingUrl"`
	StereoRecordingURL              string    `json:"stereoRecordingUrl"`
	VideoRecordingURL               string    `json:"videoRecordingUrl"`
	VideoRecordingStartDelaySeconds float64   `json:"videoRecordingStartDelaySeconds"`
	Transcript                      string    `json:"transcript"`
	PcapURL                         string    `json:"pcapUrl"`
}

// Transport represents transport configuration
type Transport struct {
	Provider              string `json:"provider"`
	AssistantVideoEnabled bool   `json:"assistantVideoEnabled"`
}

// PhoneNumber represents phone number configuration
type PhoneNumber struct {
	TwilioAccountSid    string       `json:"twilioAccountSid"`
	TwilioAuthToken     string       `json:"twilioAuthToken"`
	TwilioPhoneNumber   string       `json:"twilioPhoneNumber"`
	FallbackDestination Destination  `json:"fallbackDestination"`
	Name                string       `json:"name"`
	AssistantID         string       `json:"assistantId"`
	SquadID             string       `json:"squadId"`
	Server              ServerConfig `json:"server"`
}
