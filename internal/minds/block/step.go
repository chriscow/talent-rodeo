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

type Step struct {
	Type         StepType          `json:"type"`
	Name         string            `json:"name"`
	Block        Block             `json:"block"`
	BlockID      *string           `json:"blockId,omitempty"`
	Destinations []StepDestination `json:"destinations,omitempty"`
	Input        map[string]any    `json:"input,omitempty"`

	// This is the mutations to apply to the context after the step is done.
	// This is only for a `callback` step
	Mutations []StepMutation `json:"mutations,omitempty"`
}
