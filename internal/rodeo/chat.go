package rodeo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"talent-rodeo/internal/rodeo/block"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
	"github.com/chriscow/minds/providers/openai"
	"github.com/fatih/color"
)

// NOTE: could probaby make this a ThreadHandler if you think about it
func chat(llm minds.ThreadHandler, tc minds.ThreadContext) error {
	cyan := color.New(color.FgCyan).SprintFunc()
	var err error
	fmt.Println("Conversation")
	fmt.Println("---------------------")
	fmt.Print("> ")
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		if s.Text() == "exit" || s.Text() == "quit" {
			break
		}

		tc.AppendMessages(minds.Message{Role: minds.RoleUser, Content: s.Text()})

		tc, err = llm.HandleThread(tc, nil)
		if err != nil {
			return fmt.Errorf("startWorkflow: failed to handle thread: %w", err)
		}

		fmt.Printf("R > %s", cyan(tc.Messages().Last().Content))
		fmt.Print("> ")
	}

	return nil
}

func autoChatOld(tc minds.ThreadContext, maxTurns int) error {
	ctx := tc.Context()
	ctx, cancel := context.WithCancel(ctx)
	tc.WithContext(ctx)
	defer cancel()

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()
	var err error

	registry := minds.NewToolRegistry()

	endCallTool, err := NewEndCallTool(cancel)
	if err != nil {
		return fmt.Errorf("failed to create end call tool: %w", err)
	}

	if err := registry.Register(endCallTool); err != nil {
		return fmt.Errorf("failed to register end call tool: %w", err)
	}

	state := RecruiterTurn{}

	schema, err := minds.NewResponseSchema("CollectedInformation",
		"Information collected about the candidate during the call",
		state)
	if err != nil {
		return fmt.Errorf("chat: failed to create response schema: %w", err)
	}

	recruiter, err := openai.NewProvider(openai.WithResponseSchema(*schema))
	if err != nil {
		return fmt.Errorf("chat: failed to create Gemini provider: %w", err)
	}

	candidatePrompt, err := utils.CreateTemplate("prompts/candidate1.system.txt")
	if err != nil {
		return fmt.Errorf("chat: failed to create template: %w", err)
	}

	candidate, err := gemini.NewProvider(tc.Context(), gemini.WithSystemPrompt(candidatePrompt.Body()))
	if err != nil {
		return fmt.Errorf("chat: failed to create Gemini provider: %w", err)
	}

	fmt.Println("Conversation")
	fmt.Println("---------------------")
	tc = tc.AppendMessages(minds.Message{Role: minds.RoleUser, Content: "Hello?"})

	for maxTurns > 0 {
		maxTurns--
		if ctx.Err() != nil {
			return fmt.Errorf("chat: context error: %w", tc.Context().Err())
		}

		req := minds.NewRequest(tc.Messages())
		// req.Options.ToolChoice = "required"
		recruiterResp, err := recruiter.GenerateContent(ctx, req)
		if err != nil {
			return fmt.Errorf("startWorkflow: failed to handle thread: %w", err)
		}

		if recruiterResp.String() == "" {
			continue
		}

		update := RecruiterTurn{}
		if err := json.Unmarshal([]byte(recruiterResp.String()), &update); err != nil {
			return fmt.Errorf("chat: failed to unmarshal response: %w", err)
		}

		state.Merge(update)

		prettyJSON, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return fmt.Errorf("chat: failed to marshal JSON: %w", err)
		}

		fmt.Printf("[State] %s\n", blue(string(prettyJSON)))
		fmt.Printf("R > %s\n", cyan(update.Response))
		tc.AppendMessages(minds.Message{Role: minds.RoleSystem, Content: string(prettyJSON)})
		tc.AppendMessages(minds.Message{Role: minds.RoleAssistant, Content: update.Response})

		candidateReq := minds.NewRequest(reverseRoles(tc).Exclude(minds.RoleSystem, minds.RoleTool, minds.RoleFunction))
		candidateResp, err := candidate.GenerateContent(tc.Context(), candidateReq)
		if err != nil {
			return fmt.Errorf("chat: failed to generate content: %w", err)
		}

		fmt.Printf("C > %s\n", yellow(candidateResp.String()))
		tc = tc.AppendMessages(minds.Message{Role: minds.RoleUser, Content: candidateResp.String()})
	}

	// Marshal the state to JSON and print it to the console formatted pretty
	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("chat: failed to marshal state to JSON: %w", err)
	}

	fmt.Println(string(stateJSON))

	return nil

}

func reverseRoles(tc minds.ThreadContext) minds.Messages {
	messages := tc.Messages()
	for i := 0; i < len(messages); i++ {
		switch messages[i].Role {
		case minds.RoleUser:
			messages[i].Role = minds.RoleAssistant
		case minds.RoleAssistant:
			messages[i].Role = minds.RoleUser
		}

		if messages[i].Content == "" {
			panic("empty message content")
		}
	}

	return messages
}

// findDestination checks the step.Destinations in order; if all conditions match, returns StepName.
func findDestination(st block.Step, state RecruiterTurn) (string, error) {
	if len(st.Destinations) == 0 {
		return "", nil // no destinations => end
	}
	for _, dest := range st.Destinations {
		ok, err := allConditionsMet(dest.Conditions, state)
		if err != nil {
			return "", err
		}
		if ok {
			return dest.StepName, nil
		}
	}
	return "", nil
}

// Placeholder that checks your Condition set. In reality, you’d parse each
// condition, compare the left and right sides, etc.
func allConditionsMet(conditions []block.Condition, state RecruiterTurn) (bool, error) {
	for _, c := range conditions {
		if c.Type == block.ConditionTypeRuleBased {
			// Example: c.LeftSide = "output.job_title", c.Operator = "neq", c.RightSide = ""
			// In practice, you'd reflect on 'state' or parse c.LeftSide, c.RightSide, etc.
			// For simplicity, let's just do a naive check:
			val := getStateValue(state, c.LeftSide)
			switch c.Operator {
			case block.RuleOperatorNotEqual:
				if val == c.RightSide {
					return false, nil
				}
			case block.RuleOperatorEqual:
				if val != c.RightSide {
					return false, nil
				}
				// ... handle the other operators
			default:
				return false, fmt.Errorf("allConditionsMet: unimplemented operator %q", c.Operator)
			}
		}
	}
	return true, nil
}

// getStateValue is a naive example returning a string from the state
func getStateValue(state RecruiterTurn, key string) string {
	switch key {
	case "output.inferred_preferences":
		return strings.Join(state.InferredPreferences, ", ")
	case "output.potential_concerns":
		return strings.Join(state.PotentialConcerns, ", ")
	}
	return ""
}

func findStepIndexByName(steps []block.Step, name string) int {
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// autoChatWorkflow simulates a conversation using a workflow block.
func autoChatWorkflow(tc minds.ThreadContext, wf block.Block, maxTurns int) error {
	// Validate that our top-level block is actually a workflow
	if wf.Type != block.TypeWorkflow {
		return fmt.Errorf("autoChatWorkflow: provided block is not a workflow")
	}
	ctx, cancel := context.WithCancel(tc.Context())
	defer cancel()

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()

	// Prepare LLM providers:
	// 1. Recruiter: interprets the step’s instructions and returns structured JSON
	schema, err := minds.NewResponseSchema("CollectedInformation",
		"Information collected about the candidate during the call",
		RecruiterTurn{})
	if err != nil {
		return fmt.Errorf("autoChatWorkflow: failed to create response schema: %w", err)
	}
	recruiter, err := openai.NewProvider(openai.WithResponseSchema(*schema))
	if err != nil {
		return fmt.Errorf("autoChatWorkflow: failed to create recruiter provider: %w", err)
	}

	// 2. Candidate: simulates user replies
	candidate, err := gemini.NewProvider(ctx, gemini.WithSystemPrompt("You are the job candidate..."))
	if err != nil {
		return fmt.Errorf("autoChatWorkflow: failed to create candidate provider: %w", err)
	}

	// This will hold conversation state extracted from the JSON outputs
	state := RecruiterTurn{}

	fmt.Println("Conversation")
	fmt.Println("---------------------")

	// Keep track of the current step index
	var currentStepIndex int
	for maxTurns > 0 {
		if ctx.Err() != nil {
			return fmt.Errorf("autoChatWorkflow: context error: %w", ctx.Err())
		}
		maxTurns--
		if currentStepIndex >= len(wf.Steps) {
			fmt.Println("No more steps in workflow. Ending conversation.")
			break
		}

		step := wf.Steps[currentStepIndex]
		// If step.Type is handoff, transfer, or other, handle accordingly
		if step.Block.Type != block.TypeConversation {
			return fmt.Errorf("autoChatWorkflow: unsupported block type in step %q", step.Name)
		}

		// 1) ---- RECRUITER LLM: Generate system message from the step’s Instruction
		instruction := step.Block.Instruction

		// The conversation so far is in tc.Messages().
		// Append the step's Instruction as a system message so the Recruiter has context:
		tc = tc.AppendMessages(minds.Message{
			Role:    minds.RoleSystem,
			Content: instruction,
		})

		// Recruiter "speaks": interpret instructions, produce JSON output
		recruiterResp, err := recruiter.GenerateContent(ctx, minds.NewRequest(tc.Messages()))
		if err != nil {
			return fmt.Errorf("autoChatWorkflow: failed to generate recruiter response: %w", err)
		}

		if recruiterResp.String() == "" {
			return errors.New("autoChatWorkflow: empty recruiter response")
		}

		// Unmarshal the JSON into your RecruiterTurn struct
		update := RecruiterTurn{}
		if err := json.Unmarshal([]byte(recruiterResp.String()), &update); err != nil {
			return fmt.Errorf("autoChatWorkflow: failed to unmarshal recruiter JSON: %w\nRESPONSE:\n%s",
				err, recruiterResp.String())
		}

		// Merge into the existing conversation state
		state.Merge(update)

		// Log the updated state
		prettyJSON, _ := json.MarshalIndent(state, "", "  ")
		fmt.Printf("[State] %s\n", blue(string(prettyJSON)))
		fmt.Printf("R > %s\n", cyan(update.Response))

		// Add the recruiter’s final text to the conversation as an assistant message
		tc = tc.AppendMessages(minds.Message{Role: minds.RoleAssistant, Content: update.Response})

		// 2) ---- CANDIDATE LLM: Simulate user’s reply
		userResp, err := candidate.GenerateContent(ctx,
			minds.NewRequest(reverseRoles(tc).Exclude(minds.RoleSystem, minds.RoleTool, minds.RoleFunction)))
		if err != nil {
			return fmt.Errorf("autoChatWorkflow: failed to generate candidate response: %w", err)
		}

		fmt.Printf("C > %s\n", yellow(userResp.String()))
		tc = tc.AppendMessages(minds.Message{Role: minds.RoleUser, Content: userResp.String()})

		// 3) ---- DETERMINE NEXT STEP
		// Evaluate the conditions for each destination in step.Destinations
		nextStepName, err := findDestination(step, state)
		if err != nil {
			return fmt.Errorf("autoChatWorkflow: %w", err)
		}
		if nextStepName == "" {
			// No valid next step found; end the conversation
			fmt.Println("No valid next step condition met. Ending.")
			break
		}

		// Move to the step with StepName=nextStepName
		idx := findStepIndexByName(wf.Steps, nextStepName)
		if idx == -1 {
			fmt.Printf("Destination step %q not found. Ending.\n", nextStepName)
			break
		}
		currentStepIndex = idx
	} // end for loop

	// Print final state in JSON
	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("autoChatWorkflow: failed to marshal state: %w", err)
	}
	fmt.Printf("\nFinal Collected State: %s\n", string(stateJSON))
	return nil
}
