package block

import (
	"time"

	"github.com/chriscow/minds"
)

type Message any

type BlockType string

const (
	TypeConversation BlockType = "conversation"
	TypeToolCall     BlockType = "tool-call"
	TypeWorkflow     BlockType = "workflow"
)

type Block struct {
	Type      BlockType `json:"type" enum:"conversation,tool-call,workflow"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UpdatedAt time.Time `json:"updatedAt"`

	// This is the input schema for the block. This is the input the block needs
	// to run. It’s given to the block as steps[0].input
	InputSchema *minds.Definition `json:"inputSchema,omitempty"`

	// These are the pre-configured messages that will be spoken to the user
	// while the block is running.
	Messages []Message `json:"messages,omitempty"`

	// This is the name of the block. This is just for your reference.
	Name *string `json:"name,omitempty"`

	// This is the output schema for the block. This is the output the block
	// will return to the workflow ({{stepName.output}}).
	//
	// These are accessible as variables:
	//
	// ({{output.propertyName}}) in context of the block execution (step)
	// ({{stepName.output.propertyName}}) in context of the workflow (read
	// caveat #1) ({{blockName.output.propertyName}}) in context of the workflow
	// (read caveat #2)
	//
	//Caveats:
	//
	// a workflow can execute a step multiple times. example, if a loop is used
	// in the graph. {{stepName.output.propertyName}} will reference the latest
	// usage of the step.
	//
	// a workflow can execute a block multiple times. example, if a step is
	// called multiple times or if a block is used in multiple steps.
	// {{blockName.output.propertyName}} will reference the latest usage of the
	// block. this liquid variable is just provided for convenience when
	// creating blocks outside of a workflow with steps.
	OutputSchema *minds.Definition `json:"outputSchema,omitempty"`

	// type: "conversation" only: This is the instruction for the LLM conversation.
	Instruction string `json:"instruction,omitempty"`

	// type: "workflow" only: These are the steps in the workflow.
	Steps []Step `json:"steps,omitempty"`

	// type: "tool-call" only: This is the tool to call.
	Tool   *ToolCall `json:"tool,omitempty"`
	ToolID *string   `json:"toolId,omitempty"`
}
