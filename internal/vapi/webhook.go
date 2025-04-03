package vapi

import "encoding/json"

// Message types
const (
	MsgTypeAssistantRequest   = "assistant-request"
	MsgTypeToolCalls          = "tool-calls"
	MsgTypeTransferDestReq    = "transfer-destination-request"
	MsgTypeConversationUpdate = "conversation-update"
	MsgTypeEndOfCallReport    = "end-of-call-report"
	MsgTypeFunctionCall       = "function-call"
	MsgTypeHang               = "hang"
	MsgTypeSpeechUpdate       = "speech-update"
	MsgTypeStatusUpdate       = "status-update"
	MsgTypeUserInterrupted    = "user-interrupted"
)

// Message represents the incoming message from VAPI webhooks
type WebhookMessage struct {
	Message WebhookMessageContent `json:"message"`
}

// MessageContent contains the actual content of the webhook message
type WebhookMessageContent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// UnmarshalJSON custom unmarshaler to capture type and raw data
// func (m *WebhookMessageContent) UnmarshalJSON(data []byte) error {
// 	// Use a temporary struct to avoid recursion
// 	type Temp struct {
// 		Type string          `json:"type"`
// 		Data json.RawMessage `json:"data"`
// 	}
// 	var temp Temp
// 	if err := json.Unmarshal(data, &temp); err != nil {
// 		return err
// 	}
// 	m.Type = temp.Type
// 	m.Data = temp.Data
// 	return nil
// }
