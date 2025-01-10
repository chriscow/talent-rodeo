package rodeo

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

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

func autoChat(tc minds.ThreadContext, maxTurns int) error {
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

	candidatePrompt, err := CreateTemplate("prompts/candidate1.system.txt")
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
