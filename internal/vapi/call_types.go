package vapi

import (
	"encoding/json"
	"time"
)

// ServerConfig defines the configuration for a server endpoint
type ServerConfig struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// Voice represents voice configuration
type Voice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voiceId"`
}

// Model represents model configuration
type Model struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Transcriber represents transcriber configuration
type Transcriber struct {
	Provider string `json:"provider"`
}

// VoicemailDetection represents voicemail detection configuration
type VoicemailDetection struct {
	Provider                         string  `json:"provider"`
	VoicemailExpectedDurationSeconds float64 `json:"voicemailExpectedDurationSeconds"`
}

// KnowledgeBase represents knowledge base configuration
type KnowledgeBase struct {
	Server ServerConfig `json:"server"`
}

// Tool represents tool configuration
type Tool struct {
	Type  string `json:"type"`
	Async bool   `json:"async"`
}

// ChunkPlan represents chunk configuration
type ChunkPlan struct {
	Enabled       bool `json:"enabled"`
	MinCharacters int  `json:"minCharacters"`
}

// FallbackPlan represents fallback configuration
type FallbackPlan struct {
	Voices []FallbackVoice `json:"voices"`
}

// FallbackVoice represents fallback voice configuration
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

// Cost represents a cost entry in the call
type Cost struct {
	Type     string  `json:"type"`
	Cost     float64 `json:"cost"`
	Minutes  float64 `json:"minutes"`
	Provider string  `json:"provider"`
}

// CostBreakdown represents the detailed cost breakdown of a call
type CostBreakdown struct {
	Transport float64 `json:"transport"`
	Stt       float64 `json:"stt"`
	Llm       float64 `json:"llm"`
	Tts       float64 `json:"tts"`
	Vapi      float64 `json:"vapi"`
	Total     float64 `json:"total"`

	LlmPromptTokens     int `json:"llmPromptTokens"`
	LlmCompletionTokens int `json:"llmCompletionTokens"`
	TtsCharacters       int `json:"ttsCharacters"`

	AnalysisCostBreakdown struct {
		Summary                           float64 `json:"summary"`
		SummaryPromptTokens               int     `json:"summaryPromptTokens"`
		SummaryCompletionTokens           int     `json:"summaryCompletionTokens"`
		StructuredData                    float64 `json:"structuredData"`
		StructuredDataPromptTokens        int     `json:"structuredDataPromptTokens"`
		StructuredDataCompletionTokens    int     `json:"structuredDataCompletionTokens"`
		SuccessEvaluation                 float64 `json:"successEvaluation"`
		SuccessEvaluationPromptTokens     int     `json:"successEvaluationPromptTokens"`
		SuccessEvaluationCompletionTokens int     `json:"successEvaluationCompletionTokens"`
	} `json:"analysisCostBreakdown"`
}

// Analysis represents the analysis results of a call
type Analysis struct {
	Summary             *string         `json:"summary,omitempty"`
	StructuredData      json.RawMessage `json:"structuredData,omitempty"`
	StructuredDataMulti any             `json:"structuredDataMulti,omitempty"`
	SuccessEvaluation   *string         `json:"successEvaluation,omitempty"`
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

// Monitor represents monitoring URLs for a call
type Monitor struct {
	ListenUrl  string `json:"listenUrl"`
	ControlUrl string `json:"controlUrl"`
}

// Artifact represents call artifacts like recordings and transcripts
type Artifact struct {
	Messages                        []Message `json:"messages"`
	MessagesOpenAIFormatted         []Message `json:"messagesOpenAIFormatted"`
	RecordingUrl                    string    `json:"recordingUrl"`
	StereoRecordingUrl              string    `json:"stereoRecordingUrl"`
	VideoRecordingUrl               string    `json:"videoRecordingUrl"`
	VideoRecordingStartDelaySeconds int       `json:"videoRecordingStartDelaySeconds"`
	Transcript                      string    `json:"transcript"`
	PcapUrl                         string    `json:"pcapUrl"`
}

// Transport represents transport configuration for a call
type Transport struct {
	Provider              string `json:"provider"`
	AssistantVideoEnabled bool   `json:"assistantVideoEnabled"`
}

// PhoneNumber represents phone number configuration
type PhoneNumber struct {
	TwilioAccountSid    string        `json:"twilioAccountSid"`
	TwilioAuthToken     string        `json:"twilioAuthToken"`
	TwilioPhoneNumber   string        `json:"twilioPhoneNumber"`
	FallbackDestination *Destination  `json:"fallbackDestination,omitempty"`
	Name                string        `json:"name,omitempty"`
	AssistantID         string        `json:"assistantId,omitempty"`
	SquadID             string        `json:"squadId,omitempty"`
	Server              *ServerConfig `json:"server,omitempty"`
}

// Destination represents call destination configuration
type Destination struct {
	Type                   string       `json:"type"`
	Number                 string       `json:"number"`
	CallerId               string       `json:"callerId"`
	Description            string       `json:"description"`
	Extension              string       `json:"extension"`
	Message                string       `json:"message"`
	NumberE164CheckEnabled bool         `json:"numberE164CheckEnabled"`
	TransferPlan           TransferPlan `json:"transferPlan"`
}

// TransferPlan represents call transfer configuration
type TransferPlan struct {
	Mode    string  `json:"mode"`
	Message string  `json:"message"`
	SipVerb *string `json:"sipVerb"`
	Twiml   string  `json:"twiml"`
}

// Squad represents a squad configuration
type Squad struct {
	Members          []any     `json:"members"`
	Name             string    `json:"name"`
	MembersOverrides Assistant `json:"membersOverrides"`
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
	NumberE164CheckEnabled *bool  `json:"numberE164CheckEnabled,omitempty"`
	Extension              string `json:"extension,omitempty"`
	Number                 string `json:"number"`
	SipURI                 string `json:"sipUri,omitempty"`
	Name                   string `json:"name"`
}

// Call represents a call request
type Call struct {
	// Required fields
	ID        *string    `json:"id,omitempty"`
	OrgID     *string    `json:"orgId,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`

	// Optional fields
	Type     *string   `json:"type,omitempty"`
	Analysis *Analysis `json:"analysis,omitempty"`
	Artifact *Artifact `json:"artifact,omitempty"`

	Costs              []Cost         `json:"costs,omitempty"`
	Messages           []Message      `json:"messages,omitempty"`
	PhoneCallProvider  *string        `json:"phoneCallProvider,omitempty"`
	PhoneCallTransport *string        `json:"phoneCallTransport,omitempty"`
	Status             *string        `json:"status,omitempty"`
	EndedReason        *string        `json:"endedReason,omitempty"`
	Destination        *Destination   `json:"destination,omitempty"`
	StartedAt          *time.Time     `json:"startedAt,omitempty"`
	EndedAt            *time.Time     `json:"endedAt,omitempty"`
	Cost               *float64       `json:"cost,omitempty"`
	CostBreakdown      *CostBreakdown `json:"costBreakdown,omitempty"`
	ArtifactPlan       *ArtifactPlan  `json:"artifactPlan,omitempty"`
	Monitor            *Monitor       `json:"monitor,omitempty"`
	Transport          *Transport     `json:"transport,omitempty"`
	AssistantID        *string        `json:"assistantId,omitempty"`
	Assistant          *Assistant     `json:"assistant,omitempty"`
	AssistantOverrides *Assistant     `json:"assistantOverrides,omitempty"`
	SquadID            *string        `json:"squadId,omitempty"`
	Squad              *Squad         `json:"squad,omitempty"`
	PhoneNumberID      *string        `json:"phoneNumberId,omitempty"`
	PhoneNumber        *PhoneNumber   `json:"phoneNumber,omitempty"`
	CustomerID         *string        `json:"customerId,omitempty"`
	Customer           *Customer      `json:"customer,omitempty"`
	Name               *string        `json:"name,omitempty"`
}

// EndOfCallReport represents the report generated at the end of a call
type EndOfCallReport struct {
	Timestamp *float64  `json:"timestamp,omitempty"`
	Type      string    `json:"type"`
	Artifact  *Artifact `json:"artifact"`
	Analysis  *Analysis `json:"analysis"`

	// Optional fields
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	EndedReason string     `json:"endedReason"`
	Cost        *float64   `json:"cost,omitempty"`
	Costs       []Cost     `json:"costs,omitempty"`

	Summary    *string `json:"summary,omitempty"`
	Transcript *string `json:"transcript,omitempty"`

	Messages           []Message    `json:"messages,omitempty"`
	RecordingUrl       *string      `json:"recordingUrl,omitempty"`
	StereoRecordingUrl *string      `json:"stereoRecordingUrl,omitempty"`
	Call               *Call        `json:"call,omitempty"`
	PhoneNumber        *PhoneNumber `json:"phoneNumber,omitempty"`
	Customer           *Customer    `json:"customer,omitempty"`

	Assistant *Assistant `json:"assistant,omitempty"`
}
