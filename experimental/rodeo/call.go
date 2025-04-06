package rodeo

import (
	"context"

	"github.com/chriscow/minds"
)

type EndCallArgs struct {
	CallID      string `json:"call_id"`
	Disposition string `json:"disposition" enum:"completed,missed,failed,other"`
	Notes       string `json:"notes"`
}

type endCallTool struct {
	cancel context.CancelFunc
}

func (t *endCallTool) Type() string {
	return "end_call"
}

func (t *endCallTool) Name() string {
	return "end_call"
}

func (t *endCallTool) Description() string {
	return "End the current phone call and record its disposition"
}

func (t *endCallTool) Parameters() minds.Definition {
	definition, _ := minds.GenerateSchema(EndCallArgs{})
	return *definition
}

func (t *endCallTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	t.cancel()
	return nil, nil
}

func NewEndCallTool(cancel context.CancelFunc) (minds.Tool, error) {
	return &endCallTool{cancel: cancel}, nil
}
