package feedback

import (
	"context"
	"log/slog"

	"talent-rodeo/internal"
)

type validator struct {
	llm    LLMClient
	logger *slog.Logger
}

func newValidator(llm LLMClient, logger *slog.Logger) *validator {
	return &validator{
		llm:    llm,
		logger: logger.With("chain", "validation"),
	}
}

func (c *validator) Run(ctx context.Context, updatedSQ *internal.SteeringQuery) error {
	// TODO: Implement the validation logic using LLM calls.
	// This will involve creating a prompt and using the llm client to validate the steering query.
	return updatedSQ.Validate()
}
