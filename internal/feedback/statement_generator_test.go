package feedback

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestStatementGenerator_Run(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	t.Run("successful update with existing statement", func(t *testing.T) {
		currentStatement := "I want to work with Go programming and avoid Java."
		expectedUpdate := "I want to work with Go programming and Kubernetes. Avoid: Java, legacy systems."

		mockClient := &mockLLMClient{
			updateStatementFunc: func(ctx context.Context, prompt string) (string, error) {
				// Verify the prompt contains expected elements
				if !strings.Contains(prompt, currentStatement) {
					t.Errorf("prompt should contain current statement")
				}
				if !strings.Contains(prompt, "- kubernetes (keywords: k8s, containers)") {
					t.Errorf("prompt should contain liked themes")
				}
				if !strings.Contains(prompt, "- legacy systems (keywords: old, legacy)") {
					t.Errorf("prompt should contain disliked themes")
				}
				return expectedUpdate, nil
			},
		}

		chain := NewStatementGenerator(nil, mockClient, logger)

		analysis := &SemanticFeedbackAnalysis{
			SemanticThemes: struct {
				Liked    []SemanticTheme `json:"Liked"`
				Disliked []SemanticTheme `json:"Disliked"`
			}{
				Liked: []SemanticTheme{
					{Theme: "kubernetes", Keywords: []string{"k8s", "containers"}},
				},
				Disliked: []SemanticTheme{
					{Theme: "legacy systems", Keywords: []string{"old", "legacy"}},
				},
			},
			ConfidenceScore: 0.8,
			Reasoning:       "User shows clear preferences",
		}

		result, err := chain.Run(ctx, &currentStatement, analysis)
		if err != nil {
			t.Fatalf("Run() unexpected error = %v", err)
		}

		if result != expectedUpdate {
			t.Errorf("Run() got = %v, want %v", result, expectedUpdate)
		}
	})

	t.Run("successful update with no existing statement", func(t *testing.T) {
		expectedUpdate := "I want to work with Python and data science. Avoid: PHP development."

		mockClient := &mockLLMClient{
			updateStatementFunc: func(ctx context.Context, prompt string) (string, error) {
				// Verify the prompt indicates no existing statement
				if !strings.Contains(prompt, "No existing statement") {
					t.Errorf("prompt should indicate no existing statement")
				}
				return expectedUpdate, nil
			},
		}

		chain := NewStatementGenerator(nil, mockClient, logger)

		analysis := &SemanticFeedbackAnalysis{
			SemanticThemes: struct {
				Liked    []SemanticTheme `json:"Liked"`
				Disliked []SemanticTheme `json:"Disliked"`
			}{
				Liked: []SemanticTheme{
					{Theme: "python", Keywords: []string{"python", "data science"}},
				},
				Disliked: []SemanticTheme{
					{Theme: "php", Keywords: []string{"php", "wordpress"}},
				},
			},
			ConfidenceScore: 0.9,
			Reasoning:       "Strong signals from feedback",
		}

		result, err := chain.Run(ctx, nil, analysis)
		if err != nil {
			t.Fatalf("Run() unexpected error = %v", err)
		}

		if result != expectedUpdate {
			t.Errorf("Run() got = %v, want %v", result, expectedUpdate)
		}
	})

	t.Run("LLM returns an error", func(t *testing.T) {
		mockErr := errors.New("llm failed")
		mockClient := &mockLLMClient{
			updateStatementFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", mockErr
			},
		}

		chain := NewStatementGenerator(nil, mockClient, logger)
		statement := "test statement"
		analysis := &SemanticFeedbackAnalysis{}

		_, err := chain.Run(ctx, &statement, analysis)
		if err == nil {
			t.Fatalf("Run() expected an error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to update statement") {
			t.Errorf("error should contain 'failed to update statement', got: %v", err)
		}
	})

	t.Run("handles empty themes correctly", func(t *testing.T) {
		currentStatement := "I want remote work."
		expectedUpdate := "I want remote work with flexible schedule."

		mockClient := &mockLLMClient{
			updateStatementFunc: func(ctx context.Context, prompt string) (string, error) {
				// Should contain "No liked themes" and "No disliked themes"
				if !strings.Contains(prompt, "No liked themes identified.") {
					t.Errorf("prompt should handle empty liked themes")
				}
				if !strings.Contains(prompt, "No disliked themes identified.") {
					t.Errorf("prompt should handle empty disliked themes")
				}
				return expectedUpdate, nil
			},
		}

		chain := NewStatementGenerator(nil, mockClient, logger)

		analysis := &SemanticFeedbackAnalysis{
			SemanticThemes: struct {
				Liked    []SemanticTheme `json:"Liked"`
				Disliked []SemanticTheme `json:"Disliked"`
			}{
				Liked:    []SemanticTheme{}, // empty
				Disliked: []SemanticTheme{}, // empty
			},
			ConfidenceScore: 0.1,
			Reasoning:       "No clear patterns found",
		}

		result, err := chain.Run(ctx, &currentStatement, analysis)
		if err != nil {
			t.Fatalf("Run() unexpected error = %v", err)
		}

		if result != expectedUpdate {
			t.Errorf("Run() got = %v, want %v", result, expectedUpdate)
		}
	})
}

func TestStatementGenerator_buildThemesText(t *testing.T) {
	chain := &StatementGenerator{}

	t.Run("empty themes", func(t *testing.T) {
		result := chain.buildThemesText([]SemanticTheme{}, "LIKED")
		expected := "No liked themes identified."
		if result != expected {
			t.Errorf("buildThemesText() got = %v, want %v", result, expected)
		}
	})

	t.Run("single theme", func(t *testing.T) {
		themes := []SemanticTheme{
			{Theme: "golang", Keywords: []string{"go", "backend"}},
		}
		result := chain.buildThemesText(themes, "LIKED")
		expected := "- golang (keywords: go, backend)"
		if result != expected {
			t.Errorf("buildThemesText() got = %v, want %v", result, expected)
		}
	})

	t.Run("multiple themes", func(t *testing.T) {
		themes := []SemanticTheme{
			{Theme: "golang", Keywords: []string{"go", "backend"}},
			{Theme: "kubernetes", Keywords: []string{"k8s", "containers"}},
		}
		result := chain.buildThemesText(themes, "LIKED")
		expected := "- golang (keywords: go, backend)\n- kubernetes (keywords: k8s, containers)"
		if result != expected {
			t.Errorf("buildThemesText() got = %v, want %v", result, expected)
		}
	})
}

func TestGetStatementLength(t *testing.T) {
	t.Run("nil statement", func(t *testing.T) {
		result := getStatementLength(nil)
		if result != 0 {
			t.Errorf("getStatementLength(nil) got = %v, want 0", result)
		}
	})

	t.Run("empty statement", func(t *testing.T) {
		empty := ""
		result := getStatementLength(&empty)
		if result != 0 {
			t.Errorf("getStatementLength(&empty) got = %v, want 0", result)
		}
	})

	t.Run("non-empty statement", func(t *testing.T) {
		statement := "hello world"
		result := getStatementLength(&statement)
		expected := len(statement)
		if result != expected {
			t.Errorf("getStatementLength() got = %v, want %v", result, expected)
		}
	})
}
