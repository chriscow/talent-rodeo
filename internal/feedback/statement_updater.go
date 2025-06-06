package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"talent-rodeo/assets"
)

// StatementUpdateChain handles updating user statements based on feedback analysis.
type StatementUpdateChain struct {
	llmClient LLMClient
	logger    *slog.Logger
}

// newStatementUpdater creates a new StatementUpdateChain.
func newStatementUpdater(llmClient LLMClient, logger *slog.Logger) *StatementUpdateChain {
	return &StatementUpdateChain{
		llmClient: llmClient,
		logger:    logger,
	}
}

// Run processes feedback analysis to update a user statement.
func (suc *StatementUpdateChain) Run(ctx context.Context, currentStatement *string, analysis *SemanticFeedbackAnalysis) (string, error) {
	prompt, err := suc.buildPrompt(currentStatement, analysis)
	if err != nil {
		return "", fmt.Errorf("failed to build prompt: %w", err)
	}

	suc.logger.Debug("calling LLM to update statement", "prompt_length", len(prompt))

	updatedStatement, err := suc.llmClient.UpdateStatement(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to update statement: %w", err)
	}

	// Clean up the statement (remove extra whitespace, ensure reasonable length)
	updatedStatement = strings.TrimSpace(updatedStatement)

	suc.logger.Debug("statement updated successfully",
		"original_length", getStatementLength(currentStatement),
		"updated_length", len(updatedStatement))

	return updatedStatement, nil
}

// buildPrompt constructs the prompt for the LLM to update the user statement.
func (suc *StatementUpdateChain) buildPrompt(currentStatement *string, analysis *SemanticFeedbackAnalysis) (string, error) {
	// Load the prompt template from embedded files
	promptBytes, err := assets.EmbeddedFiles.ReadFile("prompts/statement-update.v001.txt")
	if err != nil {
		return "", fmt.Errorf("failed to read statement update prompt template: %w", err)
	}

	promptTemplate := string(promptBytes)

	// Prepare current statement text
	currentStatementText := "No existing statement."
	if currentStatement != nil && *currentStatement != "" {
		currentStatementText = *currentStatement
	}

	// Build liked themes text
	likedThemesText := suc.buildThemesText(analysis.LikedThemes(), "LIKED")

	// Build disliked themes text
	dislikedThemesText := suc.buildThemesText(analysis.DislikedThemes(), "DISLIKED")

	// Replace placeholders in the template
	prompt := strings.ReplaceAll(promptTemplate, "{{CURRENT_STATEMENT}}", currentStatementText)
	prompt = strings.ReplaceAll(prompt, "{{LIKED_THEMES}}", likedThemesText)
	prompt = strings.ReplaceAll(prompt, "{{DISLIKED_THEMES}}", dislikedThemesText)
	prompt = strings.ReplaceAll(prompt, "{{CONFIDENCE_SCORE}}", fmt.Sprintf("%.2f", analysis.ConfidenceScore))
	prompt = strings.ReplaceAll(prompt, "{{REASONING}}", analysis.Reasoning)

	return prompt, nil
}

// buildThemesText formats semantic themes for the prompt.
func (suc *StatementUpdateChain) buildThemesText(themes []SemanticTheme, preference string) string {
	if len(themes) == 0 {
		return fmt.Sprintf("No %s themes identified.", strings.ToLower(preference))
	}

	var parts []string
	for _, theme := range themes {
		keywords := strings.Join(theme.Keywords, ", ")
		parts = append(parts, fmt.Sprintf("- %s (keywords: %s)", theme.Theme, keywords))
	}

	return strings.Join(parts, "\n")
}

// getStatementLength safely gets the length of a statement pointer.
func getStatementLength(statement *string) int {
	if statement == nil {
		return 0
	}
	return len(*statement)
}
