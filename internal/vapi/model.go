package vapi

// ModelConfig contains LLM settings
type ModelConfig struct {
	Provider                  string        `json:"provider"`
	Model                     string        `json:"model"`
	EmotionRecognitionEnabled bool          `json:"emotionRecognitionEnabled"`
	KnowledgeBase             KnowledgeBase `json:"knowledgeBase"`
	KnowledgeBaseID           string        `json:"knowledgeBaseId"`
	MaxTokens                 float64       `json:"maxTokens"`
	Messages                  []Message     `json:"messages"`
	NumFastTurns              float64       `json:"numFastTurns"`
	Temperature               float64       `json:"temperature"`
	ToolIDs                   []string      `json:"toolIds"`
	Tools                     []Tool        `json:"tools"`
}
