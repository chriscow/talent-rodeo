package conversations

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"talent-rodeo/assets"
	"talent-rodeo/cmd/evals/store"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/handlers"
	"github.com/chriscow/minds/middleware"
	"github.com/chriscow/minds/providers/openai"
	"github.com/google/uuid"
)

// RunConversations generates conversations between recruiter and multiple personas
func RunConversations(ctx context.Context) {
	// Load recruiter prompt
	recruiterPrompt, err := utils.CreateTemplate("prompts/recruiter.system.004.txt")
	if err != nil {
		log.Fatal("Failed to load recruiter prompt:", err)
	}

	// Load candidate personas

	personaFiles, err := assets.EmbeddedFiles.ReadDir("prompts/personas") //filepath.Glob("assets/prompts/personas/*.txt")
	if err != nil {
		log.Fatal("Failed to load personas:", err)
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error, len(personaFiles))

	for _, personaFile := range personaFiles {
		if personaFile.IsDir() || !strings.HasSuffix(personaFile.Name(), ".prompt") {
			continue
		}

		wg.Add(1)
		path := filepath.Join("prompts/personas/", personaFile.Name())
		personaPrompt, err := utils.CreateTemplate(path)
		if err != nil {
			fmt.Println("Error loading persona:", personaFile, err)
			continue
		}

		conversationID := uuid.New().String()
		runID := fmt.Sprintf("%d", time.Now().Unix()) // Unique per run

		go eval(ctx, recruiterPrompt, personaPrompt, conversationID, runID, errCh, &wg)
		wg.Wait()
	}

	close(errCh)
	for err := range errCh {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func eval(ctx context.Context, recruiterPrompt, personaPrompt utils.Prompt, conversationID, runID string, errCh chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	resumeFilename := personaPrompt.Header.Extra["filename"].(string) + ".resume.yaml"
	resume, err := assets.EmbeddedFiles.ReadFile(filepath.Join("prompts/personas", resumeFilename))
	if err != nil {
		errCh <- fmt.Errorf("Error loading resume: %w", err)
		return
	}

	recuiterBuffer := bytes.Buffer{}
	data := struct {
		Resume string
	}{
		Resume: string(resume),
	}
	recruiterPrompt.Template.Execute(&recuiterBuffer, data)

	shouldEndCall := false
	endCall := minds.CallableFunc(func(_ context.Context, _ []byte) ([]byte, error) {
		shouldEndCall = true
		return []byte("call ended"), nil
	})

	endCallTool, err := minds.WrapFunction("end_call", "ends the call", struct {
		Reason string `json:"reason"`
	}{}, endCall)
	if err != nil {
		errCh <- fmt.Errorf("Error creating end call tool: %w", err)
		return
	}

	recruiter, err := openai.NewProvider(openai.WithName("Shirely"), openai.WithSystemPrompt(recuiterBuffer.String()), openai.WithTool(endCallTool))
	if err != nil {
		errCh <- fmt.Errorf("Error creating recruiter OpenAI provider: %w", err)
		return
	}

	candidate, err := openai.NewProvider(openai.WithName(personaPrompt.Header.Name), openai.WithSystemPrompt(personaPrompt.Body()), openai.WithTool(endCallTool))
	if err != nil {
		errCh <- fmt.Errorf("Error creating candidate OpenAI provider: %w", err)
		return
	}

	tc := minds.NewThreadContext(ctx)
	tc.AppendMessages(minds.Message{Role: minds.RoleUser, Content: "Hello?"})

	// The conversation flow is a sequence of turns between the recruiter and the candidate
	// The first message is "Hello?" from the user
	flow := handlers.NewThreadFlow(conversationID)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	flow.Use(middleware.Logging("conversation", middleware.WithLogger(logger)))

	turn := handlers.Sequential(conversationID+"-seq", recruiter, reverseRoles("recruiter"), candidate, reverseRoles("candidate"))

	loop := handlers.For(conversationID+"-loop", 50, turn, func(tc minds.ThreadContext, iteration int) bool {
		return !shouldEndCall
	})
	flow.Handle(loop)

	result, err := flow.HandleThread(tc, nil)
	if err != nil {
		errCh <- fmt.Errorf("Error handling thread: %w", err)
		return
	}

	// Store conversation in database
	store.SaveConversation(personaPrompt.Header, conversationID, runID, result.Messages())

	fmt.Println("✅ Generated conversation for ", conversationID)

}

func reverseRoles() minds.ThreadHandlerFunc {
	return func(tc minds.ThreadContext, _ minds.ThreadHandler) (minds.ThreadContext, error) {
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

		return tc.WithMessages(messages...), nil
	}
}
