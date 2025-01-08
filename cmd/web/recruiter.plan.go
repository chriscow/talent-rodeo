package main

import (
	"bytes"
	"fmt"
)

// generatePlan generates a plan for the LLM recruiter to follow when calling a candidate.
// We have the LLM generate a dynamic plan that is customized to the candidate's resume.
func (app *application) generatePlan(resumeText string) (string, error) {
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

	return buf.String(), nil
}
