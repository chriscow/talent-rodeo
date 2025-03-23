package vapi

import "github.com/chriscow/minds"

// Assistant represents the top-level configuration for a voice interaction
type Assistant struct {
	Name                         *string                `json:"name"`
	Voice                        *ElevenLabsVoiceConfig `json:"voice,omitempty"`
	Model                        *ModelConfig           `json:"model,omitempty"`
	Transcriber                  *TranscriberConfig     `json:"transcriber,omitempty"`
	FirstMessage                 *string                `json:"firstMessage,omitempty"`
	ClientMessages               []string               `json:"clientMessages,omitempty"`
	ServerMessages               []string               `json:"serverMessages,omitempty"`
	Server                       *ServerConfig          `json:"server,omitempty"`
	EndCallFunctionEnabled       bool                   `json:"endCallFunctionEnabled,omitempty"`
	EndCallMessage               *string                `json:"endCallMessage,omitempty"`
	EndCallPhrases               []string               `json:"endCallPhrases,omitempty"`
	VoicemailDetection           *VoicemailConfig       `json:"voicemailDetection,omitempty"`
	VoicemailMessage             *string                `json:"voicemailMessage,omitempty"`
	StartSpeakingPlan            *SpeakingPlan          `json:"startSpeakingPlan,omitempty"`
	AnalysisPlan                 *AnalysisPlan          `json:"analysisPlan,omitempty"`
	SilenceTimeoutSeconds        *int                   `json:"silenceTimeoutSeconds,omitempty"`
	MaxDurationSeconds           *int                   `json:"maxDurationSeconds,omitempty"`
	BackgroundSound              *string                `json:"backgroundSound,omitempty"`
	BackchannelingEnabled        *bool                  `json:"backchannelingEnabled,omitempty"`
	BackgroundDenoisingEnabled   *bool                  `json:"backgroundDenoisingEnabled,omitempty"`
	ModelOutputInMessagesEnabled *bool                  `json:"modelOutputInMessagesEnabled,omitempty"`
	VariableValues               map[string]any         `json:"variableValues,omitempty"`
}

// TranscriberConfig contains settings for speech-to-text
type TranscriberConfig struct {
	Model    string `json:"model"`
	Language string `json:"language"`
	Provider string `json:"provider"`
}

// VoicemailConfig contains settings for voicemail detection
type VoicemailConfig struct {
	Provider                           string   `json:"provider"`
	VoicemailDetectionTypes            []string `json:"voicemailDetectionTypes"`
	Enabled                            bool     `json:"enabled"`
	MachineDetectionTimeout            int      `json:"machineDetectionTimeout"`
	MachineDetectionSpeechThreshold    int      `json:"machineDetectionSpeechThreshold"`
	MachineDetectionSpeechEndThreshold int      `json:"machineDetectionSpeechEndThreshold"`
	MachineDetectionSilenceTimeout     int      `json:"machineDetectionSilenceTimeout"`
}

// TranscriptionEndpointingPlan contains timing settings for transcription
type TranscriptionEndpointingPlan struct {
	OnPunctuationSeconds   float64 `json:"onPunctuationSeconds"`
	OnNoPunctuationSeconds float64 `json:"onNoPunctuationSeconds"`
	OnNumberSeconds        float64 `json:"onNumberSeconds"`
}

// SpeakingPlan contains settings for speech timing
type SpeakingPlan struct {
	WaitSeconds              float64                      `json:"waitSeconds"`
	SmartEndpointingEnabled  bool                         `json:"smartEndpointingEnabled"`
	TranscriptionEndpointing TranscriptionEndpointingPlan `json:"transcriptionEndpointingPlan"`
}

// SchemaProperty represents a property in the structured data schema
type SchemaProperty struct {
	Type string `json:"type"`
}

// AnalysisPlan contains settings for call analysis
type AnalysisPlan struct {
	SummaryPrompt           *string           `json:"summaryPrompt,omitempty"`
	StructuredDataPrompt    *string           `json:"structuredDataPrompt,omitempty"`
	StructuredDataSchema    *minds.Definition `json:"structuredDataSchema,omitempty"`
	SuccessEvaluationPrompt *string           `json:"successEvaluationPrompt,omitempty"`
	SuccessEvaluationRubric *string           `json:"successEvaluationRubric,omitempty"`
}

var DefaultElevenLabsVoiceConfig = ElevenLabsVoiceConfig{
	Model:                  "eleven_flash_v2_5",
	VoiceID:                "yM93hbw8Qtvdma2wCnJG",
	Provider:               "11labs",
	Stability:              0.5,
	SimilarityBoost:        0.75,
	FillerInjectionEnabled: false,
}

var DefaultModelConfig = ModelConfig{
	Provider:    "openai",
	Model:       "gpt-4o-mini",
	MaxTokens:   300,
	Temperature: 1.0,
}

var DefaultTranscriber = TranscriberConfig{
	Model:    "nova-2",
	Language: "en",
	Provider: "deepgram",
}

func DefaultAssistant(agentName, prompt, firstMessage, webhook, voicemailMessage, endCallMessage string) (*Assistant, error) {
	// VoicemailConfig{
	// 	Provider: "twilio",
	// 	VoicemailDetectionTypes: []string{
	// 		"machine_start",
	// 		"machine_end_beep",
	// 		"machine_end_silence",
	// 		"machine_end_other",
	// 	},
	// 	Enabled:                            true,
	// 	MachineDetectionTimeout:            25,
	// 	MachineDetectionSpeechThreshold:    2500,
	// 	MachineDetectionSpeechEndThreshold: 2000,
	// 	MachineDetectionSilenceTimeout:     2000,
	// },

	req := &Assistant{
		Name:                   &agentName,
		Voice:                  &DefaultElevenLabsVoiceConfig,
		Model:                  &DefaultModelConfig,
		Transcriber:            &DefaultTranscriber,
		FirstMessage:           &firstMessage,
		ClientMessages:         []string{},
		ServerMessages:         []string{"end-of-call-report", "function-call", "tool-calls"},
		Server:                 nil,
		EndCallFunctionEnabled: true,
		EndCallMessage:         &endCallMessage,
		EndCallPhrases:         []string{"goodbye"},
		VoicemailDetection:     nil,
		VoicemailMessage:       &voicemailMessage,
		StartSpeakingPlan: &SpeakingPlan{
			WaitSeconds:             0.4,
			SmartEndpointingEnabled: true,
			TranscriptionEndpointing: TranscriptionEndpointingPlan{
				OnPunctuationSeconds:   0.1,
				OnNoPunctuationSeconds: 1.5,
				OnNumberSeconds:        0.5,
			},
		},
		AnalysisPlan: nil,
		// SilenceTimeoutSeconds:        30,
		// MaxDurationSeconds:           180,
		// BackgroundSound:              "off",
		// BackchannelingEnabled:        false,
		// BackgroundDenoisingEnabled:   false,
		// ModelOutputInMessagesEnabled: false,
	}

	return req, nil
}
