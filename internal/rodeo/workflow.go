package rodeo

import (
	"bytes"
	"context"
	"fmt"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

func StartWorkflow(ctx context.Context, sessionID, resumeText string, status StatusSender) error {
	if status == nil {
		status = defaultStatus{}
	}

	//
	// The workflow generally consists of:
	// 1. Convert resume to YAML
	// 2. Generate a conversation plan for the recruiter
	// 3. Create a system prompt for the recruiter that includes the plan and resume
	// 4. Call the candidate
	//
	llm, err := gemini.NewProvider(ctx)
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to create Gemini provider")
		return fmt.Errorf("analyzeResume: failed to create Gemini provider: %w", err)
	}

	resumeYAML, err := convertResume(ctx, llm, resumeText, sessionID, status)
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to convert resume")
		return fmt.Errorf("startWorkflow: failed to convert resume: %w", err)
	}

	plan, err := generatePlan(ctx, llm, resumeText)
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to generate plan")
		return fmt.Errorf("startWorkflow: failed to generate plan: %w", err)
	}

	prompt, err := CreateTemplate("prompts/recruiter.system.003.txt")
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to create prompt template")
		return fmt.Errorf("startWorkflow: failed to create prompt template: %w", err)
	}

	systemPrompt := bytes.NewBuffer([]byte(plan))
	data := map[string]any{
		"Plan":   plan,
		"Resume": resumeYAML,
	}
	if err := prompt.Template.Execute(systemPrompt, data); err != nil {
		status.SendStatus(sessionID, "error", "Failed to execute prompt template")
		return fmt.Errorf("startWorkflow: failed to execute prompt template: %w", err)
	}

	// if err := callCandidate(ctx, sessionID, buf.String(), status); err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to call candidate")
	// 	return fmt.Errorf("startWorkflow: failed to call candidate: %w", err)
	// }

	// prompt := "Hello?"

	// text, _ := resp.Text()
	// println("R > " + text)
	tc := minds.NewThreadContext(ctx)
	tc.AppendMessage(minds.Message{
		Role:    minds.RoleSystem,
		Content: systemPrompt.String(),
	})

	return autoChat(tc, 25)
}
