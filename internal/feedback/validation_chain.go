package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

// Run validates an updated steering query with basic checks.
// For Phase 1, we implement basic validation. Advanced LLM validation can be added later.
func (c *validator) Run(ctx context.Context, updatedSQ *internal.SteeringQuery) error {
	if updatedSQ == nil {
		return fmt.Errorf("steering query cannot be nil")
	}

	// Use the built-in validation first
	if err := updatedSQ.Validate(); err != nil {
		return fmt.Errorf("steering query validation failed: %w", err)
	}

	// Additional basic checks
	if len(updatedSQ.MoveToward) > 2000 {
		c.logger.Warn("steering query MoveToward field is very long", "length", len(updatedSQ.MoveToward))
	}

	if len(updatedSQ.MoveAway) > 2000 {
		c.logger.Warn("steering query MoveAway field is very long", "length", len(updatedSQ.MoveAway))
	}

	// Check for empty content
	if strings.TrimSpace(updatedSQ.MoveToward) == "" && strings.TrimSpace(updatedSQ.MoveAway) == "" {
		return fmt.Errorf("steering query cannot be empty")
	}

	// Validate ForceAway range
	if updatedSQ.ForceAway < 0.0 || updatedSQ.ForceAway > 1.0 {
		c.logger.Warn("ForceAway value is out of recommended range", "value", updatedSQ.ForceAway)
	}

	c.logger.Debug("steering query validation passed",
		"move_toward_length", len(updatedSQ.MoveToward),
		"move_away_length", len(updatedSQ.MoveAway),
		"force_away", updatedSQ.ForceAway)

	return nil
}
