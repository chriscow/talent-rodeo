package internal

import (
	"context"
	"log/slog"

	"github.com/chriscow/minds/tools"
	"github.com/sashabaranov/go-openai"
)

func AskBool(ctx context.Context, prompt string, fs *FirestoreClient, userID *string) (bool, error) {
	type booleanResult struct {
		Boolean bool `json:"boolean" description:"The result of the evaluation"`
	}

	resp := openai.ChatCompletionResponse{}
	res, err := tools.StructuredAsk[booleanResult](ctx, prompt, tools.WithModel(tools.GPT41Nano), tools.WantCompletionResponse(&resp))
	if err != nil {
		return false, err
	}

	// Record LLM cost for boolean evaluation
	if err := RecordLLMCost(ctx, fs, userID, string(tools.GPT41Nano),
		resp, "boolean_evaluation", nil); err != nil {
		slog.Default().Error("failed to record LLM cost", "error", err, "userID", userID)
	}
	return res.Boolean, nil
}
