package internal

import (
	"context"

	"github.com/chriscow/minds/tools"
)

func AskBool(ctx context.Context, prompt string) (bool, error) {
	type booleanResult struct {
		Boolean bool `json:"boolean" description:"The result of the evaluation"`
	}

	res, err := tools.StructuredAsk[booleanResult](ctx, prompt, tools.WithModel(tools.GPT41Nano))
	return res.Boolean, err
}
