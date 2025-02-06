package block

import (
	"github.com/chriscow/minds"
)

type ToolType string

// tool types: dtmf, endCall, voicemail, function, ghl, make, transferCall
const (
	// DTMF stands for Dual-Tone Multi-Frequency
	ToolTypeDTMF      ToolType = "dtmf"
	ToolTypeEndCall   ToolType = "endCall"
	ToolTypeVoicemail ToolType = "voicemail"
	ToolTypeFunction  ToolType = "function"
	// GHL refers to GoHighLevel, a comprehensive marketing and CRM platform
	// designed to help businesses manage and automate their marketing efforts,
	// customer relationships, and sales processes.
	ToolTypeGHL          ToolType = "ghl"
	ToolTypeMake         ToolType = "make"
	ToolTypeTransferCall ToolType = "transferCall"
)

type Function struct {
	Name        string            `json:"name"`
	Strict      bool              `json:"strict"`
	Description string            `json:"description,omitempty"`
	Parameters  *minds.Definition `json:"parameters,omitempty"`
}

type ToolServer struct {
	URL            string            `json:"url"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	Secret         string            `json:"secret,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

type ToolCallCondition struct {
	Operator RuleOperator `json:"operator"`
	// This is the name of the parameter that you want to check.
	Param string `json:"param"`
	// This is the value you want to compare against the parameter.
	Value map[string]any `json:"value"`
}

type ToolMessageType string // request-start, request-complete, request-failed, request-response-delayed

const (
	ToolMessageTypeStart    ToolMessageType = "request-start"
	ToolMessageTypeComplete ToolMessageType = "request-complete"
	ToolMessageTypeFailed   ToolMessageType = "request-failed"
	ToolMessageTypeDelayed  ToolMessageType = "request-response-delayed"
)

type ToolMessage struct {
	Type ToolMessageType `json:"type"`

	// This is an optional array of conditions that the tool call arguments must meet in order for this message to be triggered.
	Conditions []ToolCallCondition `json:"conditions,omitempty"`

	// This is the content that the assistant says when this message is triggered.
	Content string `json:"content,omitempty"`
	// This is an alternative to the content property. It allows to specify variants of the same content, one per language.
	Contents []Content `json:"contents"`

	// type: 'request-complete' and 'request-failed' only This is an optional boolean that if true,
	// the call will end after the message is spoken. Default is false. This is
	// ignored if role is set to system.
	EndCallAfterSpokenEnabled bool `json:"endCallAfterSpokenEnabled,omitempty"`

	// type: 'response-complete' only
	// This is the role of the message. Optional, defaults to “assistant”.
	//
	// When role=assistant, content is said out loud.
	//
	// When role=system, content is passed to the model in a system message.
	// Example:
	// 		system: default one
	// 		assistant:
	// 		user:
	// 		assistant:
	// 		user:
	// 		assistant:
	// 		user:
	// 		assistant: tool called
	// 		tool: your server response <--- system prompt as hint --->
	//
	// model generates response which is spoken This is
	// useful when you want to provide a hint to the model about what to say
	// next.
	Role minds.Role `json:"role,omitempty"`

	// type: "request-response-delayed" only
	// The number of milliseconds to wait for the server response before saying this message.
	TimingMilliseconds int `json:"timingMilliseconds,omitempty"`
}

type ToolCall struct {
	Type ToolType `json:"type"`

	// If async, the assistant will move forward without waiting for your server to
	// respond. This is useful if you just want to trigger something on your server.
	//
	// If sync, the assistant will wait for your server to respond. This is useful
	// if want assistant to respond with the result from your server.
	//
	// Defaults to synchronous (false).
	Async bool `json:"async,omitempty"`

	// Function definition of the tool.
	//
	// For endCall, transferCall, and dtmf tools, this is auto-filled based on
	// tool-specific fields like tool.destinations. But, even in those cases, you
	// can provide a custom function definition for advanced use cases.
	//
	// An example of an advanced use case is if you want to customize the message
	// that’s spoken for endCall tool. You can specify a function where it returns
	// an argument “reason”. Then, in messages array, you can have many
	// “request-complete” messages. One of these messages will be triggered if the
	// messages[].conditions matches the “reason” argument.
	Function Function `json:"function,omitempty"`

	// Messages that will be spoken to the user as the tool is running.
	//
	// For some tools, this is auto-filled based on special fields like
	// tool.destinations. For others like the function tool, these can be custom
	// configured.
	Messages []Message `json:"messages,omitempty"`

	// Server that will be hit when this tool is requested by the model.
	//
	// All requests will be sent with the call object among other things. You
	// can find more details in the Server URL documentation.
	//
	// This overrides the serverUrl set on the org and the phoneNumber. Order of
	// precedence: highest tool.server.url, then assistant.serverUrl, then
	// phoneNumber.serverUrl, then org.serverUrl.
	Server ToolServer `json:"server,omitempty"`
}
