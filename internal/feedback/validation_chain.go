package feedback

import (
	"context"
	"log/slog"

	"talent-rodeo/internal"
)

type ValidationChain struct {
	llm    LLMClient
	logger *slog.Logger
}

func NewValidationChain(llm LLMClient, logger *slog.Logger) *ValidationChain {
	return &ValidationChain{
		llm:    llm,
		logger: logger.With("chain", "validation"),
	}
}

func (c *ValidationChain) Run(ctx context.Context, updatedSQ *internal.SteeringQuery) error {
	// TODO: Implement the validation logic using LLM calls.
	// This will involve creating a prompt and using the llm client to validate the steering query.
	return updatedSQ.Validate()
}
