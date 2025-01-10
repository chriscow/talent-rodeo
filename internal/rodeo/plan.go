package rodeo

import (
	"bytes"
	"context"
	"fmt"

	"github.com/chriscow/minds"
)

// generatePlan generates a plan for the LLM recruiter to follow when calling a candidate.
// We have the LLM generate a dynamic plan that is customized to the candidate's resume.
func generatePlan(ctx context.Context, llm minds.ContentGenerator, resumeText string) (string, error) {
	prompt, err := CreateTemplate("prompts/recruiter.call-planning.openai.txt")
	if err != nil {
		return "", fmt.Errorf("generatePlan: failed to create template: %w", err)
	}

	buf := bytes.NewBuffer([]byte(resumeText))
	data := map[string]any{
		"Resume": resumeText,
	}
	if err := prompt.Template.Execute(buf, data); err != nil {
		return "", fmt.Errorf("generatePlan: failed to execute template: %w", err)
	}

	req := minds.NewRequest(minds.Messages{{Role: minds.RoleUser, Content: buf.String()}})
	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("generatePlan: failed to generate content: %w", err)
	}

	plan := resp.String()
	if plan == "" {
		return "", fmt.Errorf("generatePlan: failed to get text from response")
	}

	return plan, nil
}
