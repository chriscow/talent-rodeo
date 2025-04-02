package vapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const (
	VoiceMailDetectionProviderTwilio = "twilio"
)

// CreateCall creates a new call with the given configuration
func CreateCall(ctx context.Context, call Call) (*CallResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	b, err := json.Marshal(call)
	if err != nil {
		return nil, err
	}

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
