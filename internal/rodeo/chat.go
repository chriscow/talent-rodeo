package rodeo

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
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

		tc.AppendMessage(minds.Message{Role: minds.RoleUser, Content: s.Text()})

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
	var err error

	endCallTool, err := NewEndCallTool(cancel)
	if err != nil {
		return fmt.Errorf("failed to create end call tool: %w", err)
	}

	recruiter, err := gemini.NewProvider(ctx, gemini.WithTool(endCallTool))
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
	tc = tc.AppendMessage(minds.Message{Role: minds.RoleUser, Content: "Hello?"})

	for maxTurns > 0 {
		maxTurns--
		if tc.Context().Err() != nil {
			return fmt.Errorf("chat: context error: %w", tc.Context().Err())
		}

		req := minds.NewRequest(tc.Messages())
		recruiterResp, err := recruiter.GenerateContent(ctx, req)
		if err != nil {
			return fmt.Errorf("startWorkflow: failed to handle thread: %w", err)
		}

		tc.AppendMessage(minds.Message{Role: minds.RoleAssistant, Content: recruiterResp.String()})

		fmt.Printf("R > %s", cyan(recruiterResp.String()))

		candidateReq := minds.NewRequest(reverseRoles(tc))
		candidateResp, err := candidate.GenerateContent(tc.Context(), candidateReq)
		if err != nil {
			return fmt.Errorf("chat: failed to generate content: %w", err)
		}

		fmt.Printf("C > %s", yellow(candidateResp.String()))
		tc = tc.AppendMessage(minds.Message{Role: minds.RoleUser, Content: candidateResp.String()})
	}

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
