package block

type RuleOperator string

const (
	RuleOperatorEqual              RuleOperator = "eq"
	RuleOperatorNotEqual           RuleOperator = "neq"
	RuleOperatorGreaterThan        RuleOperator = "gt"
	RuleOperatorLessThan           RuleOperator = "lt"
	RuleOperatorGreaterThanOrEqual RuleOperator = "gte"
	RuleOperatorLessThanOrEqual    RuleOperator = "lte"
)

type Content struct {
	Type     string `json:"type" enum:"text,markdown,code,html"`
	Text     string `json:"text"`
	Language string `json:"language"`
}

type ConditionType string

const (
	ConditionTypeModelBased ConditionType = "model-based"
	ConditionTypeRuleBased  ConditionType = "rule–-based"
)

type Condition struct {
	Type ConditionType `json:"type" enum:"model-based,rule-based"`
	// This is the instruction which should output a boolean value when passed to a model.
	// You can reference any variable in the context of the current  execution (step):
	//
	// “{{output.your-property-name}}” for current step’s output
	// ”{{input.your-property-name}}” for current step’s input
	// ”{{your-step-name.output.your-property-name}}” for another step’s output (in the same workflow; read caveat #1)
	//
	// You can also talk about the current step’s output or input directly:
	//
	// “{{output.your-property-name}} is greater than 10"
	// “{{input.your-property-name}} is not empty”
	Instruction string `json:"instruction"`

	// For rule-based conditions
	// You can reference any variable in the context of the current  execution (step):
	LeftSide  string       `json:"leftSide"`
	Operator  RuleOperator `json:"operator" enum:"eq,neq,gt,lt,gte,lte"`
	RightSide string       `json:"rightSide"`
}
