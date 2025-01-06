package block

import (
	"time"

	"github.com/chriscow/minds"
)

type Message interface{}

type BlockType string

const (
	TypeConversation BlockType = "conversation"
	TypeToolCall     BlockType = "tool-call"
	TypeWorkflow     BlockType = "workflow"
)

type Block struct {
	Type      BlockType `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UpdatedAt time.Time `json:"updatedAt"`

	// This is the input schema for the block. This is the input the block needs to run. It’s given to the block as steps[0].input
	InputSchema *minds.Definition `json:"inputSchema,omitempty"`

	// These are the pre-configured messages that will be spoken to the user while the block is running.
	Messages []Message `json:"messages,omitempty"`

	// This is the name of the block. This is just for your reference.
	Name *string `json:"name,omitempty"`

	// This is the output schema for the block. This is the output the block will return to the workflow ({{stepName.output}}).
	OutputSchema *minds.Definition `json:"outputSchema,omitempty"`

	// type: "conversation" only: This is the instruction for the conversation.
	Instruction string `json:"instruction,omitempty"`

	// type: "workflow" only: These are the steps in the workflow.
	Steps []Step `json:"steps,omitempty"`

	// type: "tool-call" only: This is the tool to call.
	Tool   *ToolCall `json:"tool,omitempty"`
	ToolID *string   `json:"toolId,omitempty"`
}
