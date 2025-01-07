package vapi

// Assistant represents the top-level configuration for a voice interaction
type Assistant struct {
	Name                         string                `json:"name"`
	Voice                        ElevenLabsVoiceConfig `json:"voice"`
	Model                        ModelConfig           `json:"model"`
	Transcriber                  TranscriberConfig     `json:"transcriber"`
	FirstMessage                 string                `json:"firstMessage"`
	ClientMessages               []string              `json:"clientMessages"`
	ServerMessages               []string              `json:"serverMessages"`
	Server                       ServerConfig          `json:"server"`
	EndCallFunctionEnabled       bool                  `json:"endCallFunctionEnabled"`
	EndCallMessage               string                `json:"endCallMessage"`
	EndCallPhrases               []string              `json:"endCallPhrases"`
	VoicemailDetection           VoicemailConfig       `json:"voicemailDetection"`
	VoicemailMessage             string                `json:"voicemailMessage"`
	StartSpeakingPlan            SpeakingPlan          `json:"startSpeakingPlan"`
	AnalysisPlan                 AnalysisPlan          `json:"analysisPlan"`
	SilenceTimeoutSeconds        int                   `json:"silenceTimeoutSeconds"`
	MaxDurationSeconds           int                   `json:"maxDurationSeconds"`
	BackgroundSound              string                `json:"backgroundSound"`
	BackchannelingEnabled        bool                  `json:"backchannelingEnabled"`
	BackgroundDenoisingEnabled   bool                  `json:"backgroundDenoisingEnabled"`
	ModelOutputInMessagesEnabled bool                  `json:"modelOutputInMessagesEnabled"`
}

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TranscriberConfig contains settings for speech-to-text
type TranscriberConfig struct {
	Model    string `json:"model"`
	Language string `json:"language"`
	Provider string `json:"provider"`
}

// ServerConfig contains webhook configuration
type ServerConfig struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
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

// StructuredDataSchema represents the JSON schema for structured data
type StructuredDataSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties"`
	Required   []string                  `json:"required"`
}

// AnalysisPlan contains settings for call analysis
type AnalysisPlan struct {
	SummaryPrompt           string               `json:"summaryPrompt"`
	StructuredDataPrompt    string               `json:"structuredDataPrompt"`
	StructuredDataSchema    StructuredDataSchema `json:"structuredDataSchema"`
	SuccessEvaluationPrompt string               `json:"successEvaluationPrompt"`
	SuccessEvaluationRubric string               `json:"successEvaluationRubric"`
}

func DefaultAssistant(agentName, prompt, firstMessage, webhook, voicemailMessage, endCallMessage string) (*Assistant, error) {
	req := &Assistant{
		Name: agentName,
		Voice: ElevenLabsVoiceConfig{
			Model:                  "eleven_flash_v2_5",
			VoiceID:                "yM93hbw8Qtvdma2wCnJG",
			Provider:               "11labs",
			Stability:              0.5,
			SimilarityBoost:        0.75,
			FillerInjectionEnabled: false,
		},
		Model: ModelConfig{
			Model: "gpt-4o",
			Messages: []Message{{
				Role:    "system",
				Content: prompt,
			}},
			Provider:                  "openai",
			Temperature:               1,
			EmotionRecognitionEnabled: true,
		},
		Transcriber: TranscriberConfig{
			Model:    "nova-2",
			Language: "en",
			Provider: "deepgram",
		},
		FirstMessage:   firstMessage,
		ClientMessages: []string{},
		ServerMessages: []string{"end-of-call-report", "function-call", "tool-calls"},
		Server: ServerConfig{
			URL:            webhook,
			TimeoutSeconds: 20,
		},
		EndCallFunctionEnabled: true,
		EndCallMessage:         endCallMessage,
		EndCallPhrases:         []string{"goodbye"},
		VoicemailDetection: VoicemailConfig{
			Provider: "twilio",
			VoicemailDetectionTypes: []string{
				"machine_start",
				"machine_end_beep",
				"machine_end_silence",
				"machine_end_other",
			},
			Enabled:                            true,
			MachineDetectionTimeout:            25,
			MachineDetectionSpeechThreshold:    2500,
			MachineDetectionSpeechEndThreshold: 2000,
			MachineDetectionSilenceTimeout:     2000,
		},
		VoicemailMessage: voicemailMessage,
		StartSpeakingPlan: SpeakingPlan{
			WaitSeconds:             0.4,
			SmartEndpointingEnabled: true,
			TranscriptionEndpointing: TranscriptionEndpointingPlan{
				OnPunctuationSeconds:   0.1,
				OnNoPunctuationSeconds: 1.5,
				OnNumberSeconds:        0.5,
			},
		},
		AnalysisPlan: AnalysisPlan{
			SummaryPrompt:        "PROMPT_TO_SUMMARIZE_HERE",
			StructuredDataPrompt: "PROMPT_TO_PARSE_DATA_HERE",
			StructuredDataSchema: StructuredDataSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"field_1": {Type: "string"},
					"field_2": {Type: "string"},
					"field_3": {Type: "number"},
					"field_4": {Type: "boolean"},
				},
				Required: []string{"field_1", "field_3"},
			},
			SuccessEvaluationPrompt: "PROMPT_TO_EVAL_CALL_HERE",
			SuccessEvaluationRubric: "NumericScale",
		},
		SilenceTimeoutSeconds:        30,
		MaxDurationSeconds:           180,
		BackgroundSound:              "off",
		BackchannelingEnabled:        false,
		BackgroundDenoisingEnabled:   false,
		ModelOutputInMessagesEnabled: false,
	}

	return req, nil
}
