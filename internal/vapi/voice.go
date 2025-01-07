package vapi

// type VoiceConfig struct {
// 	Provider              string       `json:"provider"`
// 	VoiceID               string       `json:"voiceId"`
// 	CallbackURL           string       `json:"callbackUrl"`
// 	ChunkPlan             ChunkPlan    `json:"chunkPlan"`
// 	ConversationName      string       `json:"conversationName"`
// 	ConversationalContext string       `json:"conversationalContext"`
// 	CustomGreeting        string       `json:"customGreeting"`
// 	FallbackPlan          FallbackPlan `json:"fallbackPlan"`
// 	PersonaID             string       `json:"personaId"`
// }

// VoiceConfig contains settings for voice synthesis
type ElevenLabsVoiceConfig struct {
	Provider string `json:"provider"` // required
	VoiceID  string `json:"voiceId"`  // required

	// Defaults to ‘eleven_turbo_v2’
	Model     string     `json:"model" enum:"eleven_multilingual_v2,eleven_turbo_v2,eleven_turbo_v2_5,eleven_flash_v2,eleven_flash_v2_5,eleven_monolingual_v1"` // required
	ChunkPlan *ChunkPlan `json:"chunkPlan,omitempty"`

	OptimizeStreamingLatency float64 `json:"optimizeStreamingLatency,omitempty"`

	// Get the default from the UI
	SimilarityBoost float64 `json:"similarityBoost,omitempty"`

	// Get the default from the UI
	Stability float64 `json:"stability,omitempty"`

	// Defines the style for voice settings. Check this in the UI
	Style float64 `json:"style,omitempty"`

	FillerInjectionEnabled bool `json:"fillerInjectionEnabled,omitempty"`

	UseSpeakerBoost bool `json:"useSpeakerBoost,omitempty"`
}
