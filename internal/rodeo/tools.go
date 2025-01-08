package rodeo

import (
	"context"
	"encoding/json"

	"github.com/chriscow/minds"
)

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
		&addKnownFactsTool{state: state},
		&addInferredPreferenceTool{state: state},
		// &updateStageTool{state: state},
		// Add other tools
	}
}
