package vapi

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
	Message struct {
		Type string `json:"type"`
	} `json:"message"`
}

type AssistantRequest struct {
	Type string `json:"type"` // "assistant-request"

}
