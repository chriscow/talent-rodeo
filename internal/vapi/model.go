package vapi

// ModelConfig contains LLM settings
type ModelConfig struct {
	Provider                  string         `json:"provider"`
	Model                     string         `json:"model"`
	EmotionRecognitionEnabled bool           `json:"emotionRecognitionEnabled"`
	KnowledgeBase             *KnowledgeBase `json:"knowledgeBase"`
	KnowledgeBaseID           *string        `json:"knowledgeBaseId"`
	MaxTokens                 float64        `json:"maxTokens"`
	Messages                  []ModelMessage `json:"messages"`
	NumFastTurns              *float64       `json:"numFastTurns"`
	Temperature               float64        `json:"temperature"`
	ToolIDs                   []string       `json:"toolIds"`
	Tools                     []Tool         `json:"tools"`
}

// ModelMessage represents a single message in the model conversation
type ModelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
