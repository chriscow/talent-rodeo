package main

import (
	"bytes"
	"fmt"
)

func (app *application) startWorkflow(sessionID, resumeText string) error {

	app.sendStatus(sessionID, "info", "Starting workflow...")
	resumeYAML, err := app.convertResume(resumeText, sessionID)
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to convert resume")
		return fmt.Errorf("startWorkflow: failed to convert resume: %w", err)
	}

	plan, err := app.generatePlan(resumeText)
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to generate plan")
		return fmt.Errorf("startWorkflow: failed to generate plan: %w", err)
	}

	prompt, err := CreateTemplate("prompts/recruiter.system.003.txt")
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to create prompt template")
		return fmt.Errorf("startWorkflow: failed to create prompt template: %w", err)
	}

	buf := bytes.NewBuffer([]byte(plan))
	data := map[string]any{
		"Plan":   plan,
		"Resume": resumeYAML,
	}
	if err := prompt.Template.Execute(buf, data); err != nil {
		app.sendStatus(sessionID, "error", "Failed to execute prompt template")
		return fmt.Errorf("startWorkflow: failed to execute prompt template: %w", err)
	}

	if err := app.callCandidate(sessionID, buf.String()); err != nil {
		app.sendStatus(sessionID, "error", "Failed to call candidate")
		return fmt.Errorf("startWorkflow: failed to call candidate: %w", err)
	}

	return nil
}
