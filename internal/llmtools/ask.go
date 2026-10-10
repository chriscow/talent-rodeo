// Package llmtools keeps the application's chat-completion API stable while
// sending model-specific requests through the existing OpenAI-compatible SDK.
package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/chriscow/minds"
	legacytools "github.com/chriscow/minds/tools"
	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

const (
	OpenAIAPIURLv1 = "https://api.openai.com/v1"
	DeepSeekAPIURL = "https://api.deepseek.com"

	GPT6Luna         = "gpt-6-luna"
	DeepSeekFlash    = "deepseek-flash"
	GPT41Nano        = "gpt-4.1-nano"
	GPT41            = "gpt-4.1"
	GPT4oMini        = "gpt-4o-mini"
	O4Mini           = "o4-mini"
	O3Mini           = "o3-mini"
	DeepSeekReasoner = "deepseek-reasoner"
	MockModel        = "mock-model"

	// Compatibility names resolve to the replacement models, including at old call sites.
	GPT41Mini        = GPT6Luna
	DeepSeekChat     = DeepSeekFlash
	defaultMaxTokens = 1000
)

// MockLLMResponse is the response returned when MockModel is selected.
var MockLLMResponse = "mock-llm-response"

// MockLLMError is the error returned when MockModel is selected.
var MockLLMError error

type options struct {
	model     string
	baseURL   string
	apiKey    string
	maxTokens int
	messages  minds.Messages
	response  *openai.ChatCompletionResponse
}

// Option configures an Ask or StructuredAsk request.
type Option func(*options)

// WithModel selects a model for the request.
func WithModel(model string) Option { return func(o *options) { o.model = model } }

// WithMaxTokens sets the output-token limit.
func WithMaxTokens(n int) Option { return func(o *options) { o.maxTokens = n } }

// WithBaseURL overrides the provider endpoint, primarily for local transports.
func WithBaseURL(url string) Option { return func(o *options) { o.baseURL = url } }

// WithAPIKey overrides the provider credential.
func WithAPIKey(key string) Option { return func(o *options) { o.apiKey = key } }

// WithMessages prepends conversation messages before the current prompt.
func WithMessages(messages minds.Messages) Option { return func(o *options) { o.messages = messages } }

// WantCompletionResponse copies the complete provider response to dst.
func WantCompletionResponse(dst *openai.ChatCompletionResponse) Option {
	return func(o *options) { o.response = dst }
}

// IsDeepSeekModel reports whether the model uses the DeepSeek endpoint.
func IsDeepSeekModel(model string) bool {
	return model == DeepSeekFlash || model == DeepSeekReasoner || model == "deepseek-chat"
}

// IsOpenAIModel reports whether the model uses the OpenAI endpoint.
func IsOpenAIModel(model string) bool {
	switch model {
	case GPT6Luna, GPT41Nano, GPT41, GPT4oMini, O4Mini, O3Mini, "gpt-4.1-mini":
		return true
	}
	return false
}

func normalizeModel(model string) string {
	switch model {
	case "gpt-4.1-mini":
		return GPT6Luna
	case "deepseek-chat":
		return DeepSeekFlash
	default:
		return model
	}
}

func requestOptions(opts ...Option) (options, error) {
	model := os.Getenv("LLM_DEFAULT_MODEL")
	if model == "" {
		model = os.Getenv("OPENAI_DEFAULT_MODEL")
	}
	if model == "" {
		model = GPT41Nano
	}
	o := options{model: model, maxTokens: defaultMaxTokens}
	for _, opt := range opts {
		if opt == nil {
			return o, fmt.Errorf("nil LLM option")
		}
		opt(&o)
	}
	o.model = normalizeModel(o.model)
	if o.maxTokens <= 0 {
		return o, fmt.Errorf("max tokens must be positive")
	}
	if o.model != MockModel && !IsOpenAIModel(o.model) && !IsDeepSeekModel(o.model) {
		return o, fmt.Errorf("unknown model: %s", o.model)
	}
	if IsDeepSeekModel(o.model) {
		if o.baseURL == "" {
			o.baseURL = DeepSeekAPIURL
		}
		if o.apiKey == "" {
			o.apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
	} else {
		if o.baseURL == "" {
			o.baseURL = os.Getenv("OPENAI_BASE_URL")
		}
		if o.baseURL == "" {
			o.baseURL = OpenAIAPIURLv1
		}
		if o.apiKey == "" {
			o.apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}
	return o, nil
}

func buildRequest(prompt string, o options, structured bool, schema *jsonschema.Definition, name string) openai.ChatCompletionRequest {
	messages := make([]openai.ChatCompletionMessage, 0, len(o.messages)+2)
	for _, msg := range o.messages {
		m := openai.ChatCompletionMessage{Role: string(msg.Role), Content: msg.Content, Name: msg.Name, ToolCallID: msg.ToolCallID}
		for _, tc := range msg.ToolCalls {
			m.ToolCalls = append(m.ToolCalls, openai.ToolCall{ID: tc.ID, Type: openai.ToolType(tc.Type), Function: openai.FunctionCall{Name: tc.Function.Name, Arguments: string(tc.Function.Parameters)}})
		}
		messages = append(messages, m)
	}
	if structured && IsDeepSeekModel(o.model) {
		messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: "Return a valid JSON object matching the requested structure. Do not add explanatory text."})
	}
	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})
	req := openai.ChatCompletionRequest{Model: o.model, Messages: messages}
	switch o.model {
	case GPT6Luna:
		req.MaxCompletionTokens = o.maxTokens
		req.ReasoningEffort = "none"
	case DeepSeekFlash:
		req.MaxTokens = o.maxTokens
		req.ReasoningEffort = "none"
	case O4Mini, O3Mini:
		req.MaxCompletionTokens = o.maxTokens
	default:
		req.MaxTokens = o.maxTokens
	}
	if structured {
		if IsDeepSeekModel(o.model) {
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
		} else {
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONSchema, JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{Name: name, Schema: schema, Strict: true}}
		}
	}
	return req
}

func complete(ctx context.Context, req openai.ChatCompletionRequest, o options) (openai.ChatCompletionResponse, string, error) {
	if o.apiKey == "" {
		return openai.ChatCompletionResponse{}, "", fmt.Errorf("API key is not set for %s", o.model)
	}
	config := openai.DefaultConfig(o.apiKey)
	config.BaseURL = o.baseURL
	resp, err := openai.NewClientWithConfig(config).CreateChatCompletion(ctx, req)
	if err != nil {
		return resp, "", fmt.Errorf("chat completion for %s: %w", o.model, err)
	}
	if o.response != nil {
		*o.response = resp
	}
	content, err := completionContent(resp, o.model)
	return resp, content, err
}

func completionContent(resp openai.ChatCompletionResponse, model string) (string, error) {
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completion for %s returned no choices", model)
	}
	choice := resp.Choices[0]
	if choice.Message.Refusal != "" {
		return "", fmt.Errorf("chat completion for %s refused: %s", model, choice.Message.Refusal)
	}
	if choice.FinishReason != "stop" {
		return "", fmt.Errorf("chat completion for %s finished with %q", model, choice.FinishReason)
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return "", fmt.Errorf("chat completion for %s returned empty content", model)
	}
	return choice.Message.Content, nil
}

// Ask sends prompt and returns a complete text answer.
func Ask(ctx context.Context, prompt string, opts ...Option) (string, error) {
	o, err := requestOptions(opts...)
	if err != nil {
		return "", err
	}
	if o.model == MockModel {
		return MockLLMResponse, MockLLMError
	}
	_, content, err := complete(ctx, buildRequest(prompt, o, false, nil, ""), o)
	return content, err
}

// StructuredAsk sends prompt and decodes a complete JSON answer into T.
func StructuredAsk[T any](ctx context.Context, prompt string, opts ...Option) (T, error) {
	var result T
	o, err := requestOptions(opts...)
	if err != nil {
		return result, err
	}
	if o.model == MockModel {
		if MockLLMError != nil {
			return result, MockLLMError
		}
		if err := json.Unmarshal([]byte(MockLLMResponse), &result); err != nil {
			return result, fmt.Errorf("decode mock response: %w", err)
		}
		return result, nil
	}
	schema, err := GenerateJSONSchema(result)
	if err != nil {
		return result, fmt.Errorf("generate JSON schema: %w", err)
	}
	t := reflect.TypeOf(result)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := "response"
	if t != nil && t.Name() != "" {
		name = t.Name()
	}
	_, content, err := complete(ctx, buildRequest(prompt, o, true, schema, name), o)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(content) == "null" {
		return result, fmt.Errorf("structured response for %s returned null", o.model)
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return result, fmt.Errorf("decode structured response for %s: %w", o.model, err)
	}
	return result, nil
}

// GeneratePrompt renders a versioned minds prompt template.
func GeneratePrompt(content []byte, data any) (string, error) {
	return legacytools.GeneratePrompt(content, data)
}

// GenerateJSONSchema returns the JSON schema used by StructuredAsk.
func GenerateJSONSchema(v any) (*jsonschema.Definition, error) {
	return legacytools.GenerateJSONSchema(v)
}
