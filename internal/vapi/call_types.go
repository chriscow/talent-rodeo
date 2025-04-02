package vapi

import "time"

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

type VoiceMailDetection struct {
	Provider string `json:"provider"`
}

// Call represents the top-level request structure
type Call struct {
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

	VoiceMailDetection *VoiceMailDetection `json:"voiceMailDetection,omitempty"`
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

// EndOfCallReport represents the end-of-call-report message type
type EndOfCallReport struct {
	// The following are required fields
	Type        string   `json:"type"`
	EndedReason string   `json:"endedReason"`
	Artifact    Artifact `json:"artifact"`
	Analysis    Analysis `json:"analysis"`

	// The following are optional fields
	Call      *Call     `json:"call,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Customer  *Customer `json:"customer,omitempty"`

	// There are more but we are ignoring them for now
}
