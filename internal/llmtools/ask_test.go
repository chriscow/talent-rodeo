package llmtools

import (
	"encoding/json"
	"testing"

	"github.com/chriscow/minds"
	openai "github.com/sashabaranov/go-openai"
)

func TestBuildRequestModelFields(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		wantLimit  string
		wantEffort string
	}{
		{name: "luna", model: GPT6Luna, wantLimit: "max_completion_tokens", wantEffort: "none"},
		{name: "flash", model: DeepSeekFlash, wantLimit: "max_tokens", wantEffort: "none"},
		{name: "nano", model: GPT41Nano, wantLimit: "max_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := options{model: tt.model, maxTokens: 8192, messages: minds.Messages{{Role: minds.RoleSystem, Content: "context"}}}
			body, err := json.Marshal(buildRequest("prompt", o, false, nil, ""))
			if err != nil {
				t.Fatalf("marshal request for %s: %v", tt.model, err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode request for %s: %v", tt.model, err)
			}
			if _, ok := got[tt.wantLimit]; !ok {
				t.Errorf("request for %s missing %s: %s", tt.model, tt.wantLimit, body)
			}
			otherLimit := "max_tokens"
			if tt.wantLimit == otherLimit {
				otherLimit = "max_completion_tokens"
			}
			if _, ok := got[otherLimit]; ok {
				t.Errorf("request for %s unexpectedly includes %s: %s", tt.model, otherLimit, body)
			}
			if tt.wantEffort == "" {
				if _, ok := got["reasoning_effort"]; ok {
					t.Errorf("request for %s unexpectedly includes reasoning effort: %s", tt.model, body)
				}
			} else if string(got["reasoning_effort"]) != `"none"` {
				t.Errorf("request for %s reasoning effort = %s, want none", tt.model, got["reasoning_effort"])
			}
			for _, unsupported := range []string{"temperature", "top_p"} {
				if _, ok := got[unsupported]; ok {
					t.Errorf("request for %s unexpectedly includes %s: %s", tt.model, unsupported, body)
				}
			}
			var messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(got["messages"], &messages); err != nil {
				t.Fatalf("decode messages for %s: %v", tt.model, err)
			}
			if len(messages) != 2 || messages[0].Role != "system" || messages[1].Content != "prompt" {
				t.Errorf("request for %s messages = %+v", tt.model, messages)
			}
		})
	}
}

func TestCompletionContentRejectsIncompleteAndRefusal(t *testing.T) {
	tests := []struct {
		name     string
		response openai.ChatCompletionResponse
		want     string
		wantErr  bool
	}{
		{name: "complete", response: openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{FinishReason: "stop", Message: openai.ChatCompletionMessage{Content: "answer"}}}}, want: "answer"},
		{name: "truncated", response: openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{FinishReason: "length", Message: openai.ChatCompletionMessage{Content: "partial"}}}}, wantErr: true},
		{name: "refused", response: openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{FinishReason: "stop", Message: openai.ChatCompletionMessage{Refusal: "cannot comply"}}}}, wantErr: true},
		{name: "missing choice", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completionContent(tt.response, GPT6Luna)
			if (err != nil) != tt.wantErr {
				t.Fatalf("completionContent(%s) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("completionContent(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBuildRequestStructuredFlash(t *testing.T) {
	o := options{model: DeepSeekFlash, maxTokens: 1000}
	body, err := json.Marshal(buildRequest("extract details", o, true, nil, "details"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["response_format"]) != `{"type":"json_object"}` {
		t.Errorf("response format = %s", got["response_format"])
	}
	var messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(got["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "system" || messages[1].Content != "extract details" {
		t.Errorf("structured messages = %+v", messages)
	}
}
