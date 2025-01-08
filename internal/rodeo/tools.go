package rodeo

import (
	"context"
	"encoding/json"

	"github.com/chriscow/minds"
)

// Add Known Fact
type AddKnownFactArgs struct {
	Fact string `json:"fact" description:"A new fact learned about the candidate"`
}

type addKnownFactTool struct {
	state *ConversationState
}

func (t *addKnownFactTool) Type() string { return "function" }
func (t *addKnownFactTool) Name() string { return "add_known_fact" }
func (t *addKnownFactTool) Description() string {
	return "Add a newly discovered fact about the candidate"
}
func (t *addKnownFactTool) Parameters() minds.Definition {
	def, _ := minds.GenerateSchema(AddKnownFactArgs{})
	return *def
}
func (t *addKnownFactTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	var args AddKnownFactArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, err
	}
	t.state.Candidate.KnownFacts = append(t.state.Candidate.KnownFacts, args.Fact)
	return nil, nil
}

// Update Stage
type UpdateStageArgs struct {
	Stage string `json:"stage" enum:"planning_contact,initial_contact,discovery,deep_dive,follow_up,closing"`
}

type updateStageTool struct {
	state *ConversationState
}

func (t *updateStageTool) Type() string { return "function" }
func (t *updateStageTool) Name() string { return "update_stage" }
func (t *updateStageTool) Description() string {
	return "Update the current conversation stage"
}
func (t *updateStageTool) Parameters() minds.Definition {
	def, _ := minds.GenerateSchema(UpdateStageArgs{})
	return *def
}
func (t *updateStageTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	var args UpdateStageArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, err
	}
	// Map string to ConversationStage enum
	switch args.Stage {
	case "planning_contact":
		t.state.Stage = ConversationStagePlanningContact
	case "initial_contact":
		t.state.Stage = ConversationStageInitialContact
		// Add other cases
	}
	return nil, nil
}

// Factory function
func NewConversationTools(state *ConversationState) []minds.Tool {
	return []minds.Tool{
		&addKnownFactTool{state: state},
		&updateStageTool{state: state},
		// Add other tools
	}
}
