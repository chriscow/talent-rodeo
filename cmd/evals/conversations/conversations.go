package conversations

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"talent-rodeo/assets"
	"talent-rodeo/cmd/evals/store"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/handlers"
	"github.com/chriscow/minds/providers/openai"
	"github.com/fatih/color"
	"github.com/google/uuid"
)

var (
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
)

// RunConversations generates conversations between recruiter and multiple personas
func RunConversations(ctx context.Context) {
	// Load recruiter prompt
	recruiterPrompt, err := utils.CreateTemplate("prompts/recruiter.system.005.txt")
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
		break
	}

	close(errCh)
	for err := range errCh {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

type evalConfig struct {
	recruiterPrompt utils.Prompt
	personaPrompt   utils.Prompt
	conversationID  string
	runID           string
}

func eval(ctx context.Context, recruiterPrompt, personaPrompt utils.Prompt, conversationID, runID string, errCh chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	cfg := evalConfig{
		recruiterPrompt: recruiterPrompt,
		personaPrompt:   personaPrompt,
		conversationID:  conversationID,
		runID:           runID,
	}

	// Load and prepare the resume
	resume, err := loadResume(cfg.personaPrompt)
	if err != nil {
		errCh <- fmt.Errorf("failed to load resume: %w", err)
		return
	}

	// Create the providers
	recruiter, candidate, candidateName, shouldEndCall, systemPrompt, providerName, model, err := createProviders(cfg, resume)
	if err != nil {
		errCh <- fmt.Errorf("failed to create providers: %w", err)
		return
	}

	// Initialize the database
	db, err := store.InitDB()
	if err != nil {
		errCh <- fmt.Errorf("failed to initialize database: %w", err)
		return
	}
	defer db.Close()

	// Save initial conversation record
	if err := store.SaveConversation(cfg.personaPrompt.Header, cfg.conversationID, cfg.runID, providerName, model); err != nil {
		errCh <- fmt.Errorf("failed to save conversation: %w", err)
		return
	}

	// Create and configure the conversation handler
	handler, err := createConversationHandler(cfg, recruiter, candidate, shouldEndCall, systemPrompt, db)
	if err != nil {
		errCh <- fmt.Errorf("failed to create conversation handler: %w", err)
		return
	}

	// Run the conversation
	if err := runConversation(ctx, handler, cfg, candidateName); err != nil {
		errCh <- fmt.Errorf("failed to run conversation: %w", err)
		return
	}

	fmt.Println("✅ Generated conversation for ", cfg.conversationID)
}

func loadResume(personaPrompt utils.Prompt) (string, error) {
	resumeFilename := personaPrompt.Header.Extra["filename"].(string) + ".resume.yaml"
	resume, err := assets.EmbeddedFiles.ReadFile(filepath.Join("prompts/personas", resumeFilename))
	if err != nil {
		return "", fmt.Errorf("failed to read resume file: %w", err)
	}
	return string(resume), nil
}

func createProviders(cfg evalConfig, resume string) (recruiter, candidate *openai.Provider, candidateName string, shouldEndCall *bool, systemPrompt string, providerName, model string, err error) {
	// Prepare recruiter prompt with resume
	recuiterBuffer := bytes.Buffer{}
	data := struct {
		Resume string
	}{
		Resume: resume,
	}
	cfg.recruiterPrompt.Template.Execute(&recuiterBuffer, data)
	systemPrompt = recuiterBuffer.String()

	// Create end call tool
	shouldEndCall = new(bool)
	endCall := minds.CallableFunc(func(_ context.Context, input []byte) ([]byte, error) {
		*shouldEndCall = true

		var b struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(input, &b); err != nil {
			return nil, fmt.Errorf("failed to unmarshal end call reason: %w", err)
		}

		println(red.Sprintf("[CALL ENDED] reason: %s", b.Reason))
		return []byte("call ended"), nil
	})

	endCallTool, err := minds.WrapFunction("end_call", "ends the call", struct {
		Reason string `json:"reason"`
	}{}, endCall)
	if err != nil {
		return nil, nil, "", nil, "", "", "", fmt.Errorf("failed to create end call tool: %w", err)
	}

	// Create recruiter provider
	recruiterName := funcs.Slugify(cfg.recruiterPrompt.Header.Name)
	providerName = "openai"
	model = "gpt-4"
	recruiter, err = openai.NewProvider(
		openai.WithName(recruiterName),
		openai.WithSystemPrompt(recuiterBuffer.String()),
		openai.WithTool(endCallTool),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, nil, "", nil, "", "", "", fmt.Errorf("failed to create recruiter provider: %w", err)
	}

	// Create candidate provider
	candidateName = funcs.Slugify(cfg.personaPrompt.Header.Name)
	candidate, err = openai.NewProvider(
		openai.WithName(candidateName),
		openai.WithSystemPrompt(cfg.personaPrompt.Body()),
		openai.WithModel(model),
		// openai.WithTool(endCallTool),
	)
	if err != nil {
		return nil, nil, "", nil, "", "", "", fmt.Errorf("failed to create candidate provider: %w", err)
	}

	return recruiter, candidate, candidateName, shouldEndCall, systemPrompt, providerName, model, nil
}

func createConversationHandler(cfg evalConfig, recruiter, candidate *openai.Provider, shouldEndCall *bool, systemPrompt string, db *sql.DB) (minds.ThreadHandler, error) {
	// Create the sequence handler
	turn := handlers.NewSequence(cfg.conversationID+"-seq", recruiter, candidate)

	// Add logging middleware
	// logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))
	// turn.Use(middleware.Logging("turn", middleware.WithLogger(logger)))

	// Add conversation logger middleware
	turn.Use(ConversationLogger())

	// Add persistence middleware
	turn.Use(PersistMessage(PersistenceConfig{
		DB:             db,
		ConversationID: cfg.conversationID,
		RunID:          cfg.runID,
	}))

	turn.Use(ReverseRoles())
	turn.Use(ConversationSummarizer(recruiter, systemPrompt))

	// Create the loop handler
	loop := handlers.NewFor(cfg.conversationID+"-loop", 50, turn, func(tc minds.ThreadContext, iteration int) bool {
		println(green.Sprintf("[ITERATION %d] end call: %t", iteration, *shouldEndCall))
		return !*shouldEndCall
	})

	return loop, nil
}

func runConversation(ctx context.Context, handler minds.ThreadHandler, cfg evalConfig, candidateName string) error {
	tc := minds.NewThreadContext(ctx)
	tc.AppendMessages(minds.Message{
		Role:    minds.RoleUser,
		Name:    candidateName,
		Content: "Hello?",
	})

	_, err := handler.HandleThread(tc, nil)
	return err
}
