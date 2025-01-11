package rodeo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/chriscow/minds"
)

const SessionIDKey = "sessionID"

func StartWorkflow(ctx context.Context, resumeText string, status StatusSender) error {
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
	// llm, err := openai.NewProvider()
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to create Gemini provider")
	// 	return fmt.Errorf("analyzeResume: failed to create Gemini provider: %w", err)
	// }

	// resumeYAML, err := convertResume(ctx, llm, resumeText, sessionID, status)
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to convert resume")
	// 	return fmt.Errorf("startWorkflow: failed to convert resume: %w", err)
	// }

	// plan, err := generatePlan(ctx, llm, resumeText)
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to generate plan")
	// 	return fmt.Errorf("startWorkflow: failed to generate plan: %w", err)
	// }

	// recruiterTemplate, err := CreateTemplate("prompts/recruiter.system.003.txt")
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to create prompt template")
	// 	return fmt.Errorf("startWorkflow: failed to create prompt template: %w", err)
	// }

	// recruiterPrompt := bytes.NewBuffer([]byte(plan))
	// data := map[string]any{
	// 	"Plan":   plan,
	// 	"Resume": resumeYAML,
	// }
	// if err := recruiterTemplate.Template.Execute(recruiterPrompt, data); err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to execute prompt template")
	// 	return fmt.Errorf("startWorkflow: failed to execute prompt template: %w", err)
	// }

	// bodyBytes, err := callCandidate(ctx, sessionID, recruiterPrompt.String(), status)
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to call candidate")
	// 	return fmt.Errorf("startWorkflow: failed to call candidate: %w", err)
	// }

	// status.SendStatus(sessionID, "success", "Workflow completed successfully!")

	// println("----------------------")
	// println("----------------------")
	// println("----------------------")
	// fmt.Println(string(bodyBytes))

	// Generate the workflow
	workflow, err := PlanConversationWorkflow(ctx, resumeText, status)
	if err != nil {
		return fmt.Errorf("StartWorkflow: failed to create block: %w", err)
	}

	// save the workflow to json file
	b, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return fmt.Errorf("StartWorkflow: failed to marshal workflow: %w", err)
	}

	f, err := os.Create("workflow.json")
	if err != nil {
		return fmt.Errorf("StartWorkflow: failed to create file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("StartWorkflow: failed to write to file: %w", err)
	}

	tc := minds.NewThreadContext(context.Background())
	if err := autoChatWorkflow(tc, workflow, 10); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Workflow: %v\n", workflow)

	return nil

}
