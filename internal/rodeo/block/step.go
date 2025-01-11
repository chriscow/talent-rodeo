package block

type StepType string

const (
	StepTypeHandoff  StepType = "handoff"
	StepTypeCallback StepType = "callback"
)

type StepDestination struct {
	Type       string      `json:"type"`
	StepName   string      `json:"stepName"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// StepMutation is a mutation to apply to the context after the step is done.
type StepMutation struct {
	Type       string      `json:"type" enum:"assignment"`
	Variable   string      `json:"variable"`
	Value      string      `json:"value"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// A step contains a Block and the input to the block as well as the destinations
// for the step.  The destination is determined by the output of the block.
type Step struct {
	Type         StepType          `json:"type" enum:"handoff,callback"`
	Name         string            `json:"name"`
	Block        Block             `json:"block"`
	BlockID      *string           `json:"blockId,omitempty"`
	Destinations []StepDestination `json:"destinations,omitempty"`

	// This is the input to the block. You can use any key-value map as input to
	// the block.
	//
	// Example: { “name”: “John Doe”, “age”: 20 }
	//
	// You can reference any variable in the context of the current block:
	//
	// “{{your-step-name.output.your-property-name}}” for another step’s output
	// (in the same workflow; read caveat #1)
	// “{{your-step-name.input.your-property-name}}” for another step’s input
	// (in the same workflow; read caveat #1)
	// “{{your-block-name.output.your-property-name}}” for another block’s
	// output (in the same workflow; read caveat #2)
	// “{{your-block-name.input.your-property-name}}” for another block’s input
	// (in the same workflow; read caveat #2)
	// “{{workflow.input.your-property-name}}” for the current workflow’s input
	// ”{{global.your-property-name}}” for the global context
	//
	// Example: { “name”: “{{my-tool-call-step.output.name}}”, “age”:
	// “{{my-tool-call-step.input.age}}”, “date”: “{{workflow.input.date}}” }
	//
	// You can dynamically change the key name.
	//
	// Example: { “{{my-tool-call-step.output.key-name-for-name}}”: “{{name}}”,
	// “{{my-tool-call-step.input.key-name-for-age}}”: “{{age}}”,
	// “{{workflow.input.key-name-for-date}}”: “{{date}}” }
	//
	// You can represent the value as a string, number, boolean, array, or
	// object.
	//
	// Example: { “name”: “john”, “age”: 20, “date”: “2021-01-01”, “metadata”: {
	// “unique-key”: “{{my-tool-call-step.output.unique-key}}” }, “array”: [“A”,
	// “B”, “C”], }
	//
	// Caveats:
	//
	// a workflow can execute a step multiple times. example, if a loop
	//   is used in the graph. {{stepName.input/output.propertyName}} will
	//   reference the latest usage of the step.
	//
	// a workflow can execute a block multiple times. example, if a step is
	//   called multiple times or if a block is used in multiple steps.
	//   {{blockName.input/output.propertyName}} will reference the latest usage
	//   of the block. this liquid variable is just provided for convenience when
	//   creating blocks outside of a workflow.
	Input map[string]any `json:"input,omitempty"`

	// This is the mutations to apply to the context after the step is done.
	// This is only for a `callback` step
	Mutations []StepMutation `json:"mutations,omitempty"`
}
