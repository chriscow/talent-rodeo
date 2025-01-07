package vapi

// CallRequest represents the top-level request structure
type Call struct {
	ID                 string             `json:"id"`
	OrgID              string             `json:"orgId"`
	AssistantID        string             `json:"assistantId"`
	Assistant          Assistant          `json:"assistant"`
	AssistantOverrides AssistantOverrides `json:"assistantOverrides"`
	SquadID            string             `json:"squadId"`
	Squad              Squad              `json:"squad"`
	PhoneNumberID      string             `json:"phoneNumberId"`
	Customer           Customer           `json:"customer"`
	Name               string             `json:"name"`
}

// // Assistant contains the core configuration
// type Assistant struct {
// 	Transcriber                  TranscriberConfig `json:"transcriber"`
// 	Model                        ModelConfig       `json:"model"`
// 	Voice                        VoiceConfig       `json:"voice"`
// 	FirstMessage                 string            `json:"firstMessage"`
// 	FirstMessageMode             string            `json:"firstMessageMode"`
// 	HipaaEnabled                 bool              `json:"hipaaEnabled"`
// 	ClientMessages               []string          `json:"clientMessages"`
// 	ServerMessages               []string          `json:"serverMessages"`
// 	SilenceTimeoutSeconds        int               `json:"silenceTimeoutSeconds"`
// 	MaxDurationSeconds           int               `json:"maxDurationSeconds"`
// 	BackgroundSound              string            `json:"backgroundSound"`
// 	BackgroundDenoisingEnabled   bool              `json:"backgroundDenoisingEnabled"`
// 	ModelOutputInMessagesEnabled bool              `json:"modelOutputInMessagesEnabled"`
// 	TransportConfigurations      []TransportConfig `json:"transportConfigurations"`
// 	Name                         string            `json:"name"`
// 	VoicemailDetection           VoicemailConfig   `json:"voicemailDetection"`
// 	VoicemailMessage             string            `json:"voicemailMessage"`
// 	EndCallMessage               string            `json:"endCallMessage"`
// 	EndCallPhrases               []string          `json:"endCallPhrases"`
// 	Metadata                     map[string]string `json:"metadata"`
// 	ArtifactPlan                 ArtifactPlan      `json:"artifactPlan"`
// 	MessagePlan                  MessagePlan       `json:"messagePlan"`
// 	StartSpeakingPlan            StartSpeakingPlan `json:"startSpeakingPlan"`
// 	StopSpeakingPlan             StopSpeakingPlan  `json:"stopSpeakingPlan"`
// 	MonitorPlan                  MonitorPlan       `json:"monitorPlan"`
// 	CredentialIDs                []string          `json:"credentialIds"`
// 	Server                       ServerConfig      `json:"server"`
// }

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
