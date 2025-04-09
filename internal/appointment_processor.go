package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
)

// ProcessCompletedAppointment updates the appointment after a successful call with the transcript summary.
// It is called as a background task after the transcript is fully processed.
//
// # It is called after an an end of call webhook is received in handlers.vapi.webhooks.go
//
// The workflow is:
//
//   - Get all appointments
//   - For each appointment, get the transcript summary
//   - Generate a prompt for the transcript analysis. This is generated from all the call transcripts.
//   - Ask OpenAI to analyze the transcripts
//   - Generate a search criteria based on the analysis
//   - Save the search criteria to the user
//
// Saving a new search criteria triggers a new search.
func ProcessCompletedAppointment(fs *firestore.Client, logger *slog.Logger, appointment *Appointment) error {
	processor, err := NewTranscriptProcessor(fs, logger)
	if err != nil {
		logger.Error("failed to create transcript processor", "error", err)
		return fmt.Errorf("failed to create transcript processor: %w", err)
	}

	go func() {
		user, found, err := GetUser(context.Background(), fs, appointment.UserID)
		if err != nil {
			logger.Error("failed to get user", "error", err)
			return
		}
		if !found {
			logger.Error("user not found", "user_id", appointment.UserID)
			return
		}

		// Initialize user.Search if it is nil
		if user.Search == nil {
			user.Search = &UserSearch{}
		}

		if err := processor.ProcessCompletedAppointment(context.Background(), user, appointment); err != nil {
			logger.Error("failed to process completed appointment", "error", err)
			user.Search.Error = err.Error()
			if err := UpdateUserSearch(context.Background(), fs, user.ID, user.Search); err != nil {
				logger.Error("failed to update user search", "error", err)
			}
		}
	}()

	return nil
}

// TranscriptProcessor handles the processing of appointment transcripts
type TranscriptProcessor struct {
	fs     *firestore.Client
	logger *slog.Logger
	llm    *openai.Provider
}

// NewTranscriptProcessor creates a new TranscriptProcessor
func NewTranscriptProcessor(fs *firestore.Client, logger *slog.Logger) (*TranscriptProcessor, error) {
	llm, err := openai.NewProvider(openai.WithModel("gpt-4o"))
	if err != nil {
		return nil, fmt.Errorf("failed to create openai provider: %w", err)
	}

	return &TranscriptProcessor{
		fs:     fs,
		logger: logger,
		llm:    llm,
	}, nil
}

// ProcessCompletedAppointment updates the appointment after a successful call with the transcript summary
func (p *TranscriptProcessor) ProcessCompletedAppointment(ctx context.Context, user *User, appointment *Appointment) error {
	p.logger.Info(BGGreen("processing completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)
	defer p.logger.Info(BGGreen("processed completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)

	transcripts, err := p.collectTranscripts(ctx, appointment.UserID)
	if err != nil {
		return fmt.Errorf("failed to collect transcripts: %w", err)
	}

	if _, err := p.analyzeTranscripts(ctx, user, transcripts); err != nil {
		return fmt.Errorf("failed to analyze transcripts: %w", err)
	}

	criteria, err := p.generateSearchCriteria(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to generate search criteria: %w", err)
	}

	if user.Search == nil {
		user.Search = &UserSearch{
			Criteria: *criteria,
		}
	} else {
		user.Search.Criteria = *criteria
	}

	if err := UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
		return fmt.Errorf("failed to update user search: %w", err)
	}

	return nil
}

// collectTranscripts gets all transcript summaries for a user
func (p *TranscriptProcessor) collectTranscripts(ctx context.Context, userID string) (map[string]string, error) {
	p.logger.Debug("collecting transcripts", "user_id", userID)
	defer p.logger.Debug("collected transcripts", "user_id", userID)
	appointments, err := ListAppointments(ctx, p.fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointments for user %s: %w", userID, err)
	}

	transcripts := make(map[string]string)
	for _, a := range appointments {
		if a.CallID == nil {
			continue
		}

		if a.TranscriptSummary != nil {
			transcripts[*a.CallID] = *a.TranscriptSummary
			continue
		}

		call, err := vapi.GetCall(ctx, *a.CallID)
		if err != nil {
			return nil, fmt.Errorf("failed to get call: %w", err)
		}

		p.logger.Debug("converting call result to transcript", "call_id", *call.ID)
		transcript := ConvertCallResultToTranscript(*call)
		summary, err := transcript.Summarize(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to summarize transcript: %w", err)
		}

		transcripts[*call.ID] = summary
	}

	return transcripts, nil
}

// analyzeTranscripts generates an analysis of the transcripts using OpenAI
func (p *TranscriptProcessor) analyzeTranscripts(ctx context.Context, user *User, transcripts map[string]string) (string, error) {
	p.logger.Debug("analyzing transcripts", "user_id", user.ID)
	defer p.logger.Debug("analyzed transcripts", "user_id", user.ID)

	const analysisFile = "prompts/transcript-analysis-cot.001.txt"

	systemTemplate, err := utils.CreateTemplate(analysisFile)
	if err != nil {
		return "", fmt.Errorf("failed to create template: %w", err)
	}

	geoIDs, err := ListGeoIDs(p.fs)
	if err != nil {
		return "", fmt.Errorf("failed to list geo ids: %w", err)
	}

	geoKeys := make([]string, 0, len(geoIDs))
	for _, geoID := range geoIDs {
		geoKeys = append(geoKeys, geoID.ID)
	}

	systemPrompt, err := systemTemplate.Execute(map[string]any{
		"Locations": geoKeys,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleSystem,
			Content: systemPrompt,
		},
	}

	userPrompt := []string{"Here are the transcripts:\n"}
	for callID, transcript := range transcripts {
		userPrompt = append(userPrompt, fmt.Sprintf("Call ID: %s\nTranscript:\n%s\n\n", callID, transcript))
	}

	messages = append(messages, minds.Message{
		Role:    minds.RoleUser,
		Content: strings.Join(userPrompt, "\n"),
	})

	analysis, err := p.llm.GenerateContent(ctx, minds.NewRequest(messages))
	if err != nil {
		return "", fmt.Errorf("failed to ask openai: %w", err)
	}

	// Initialize user.Search if it is nil
	if user.Search == nil {
		user.Search = &UserSearch{}
	}

	user.Search.Statement = analysis.String()

	if err := UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
		return "", fmt.Errorf("failed to update user search: %w", err)
	}

	return analysis.String(), nil
}

// generateSearchCriteria converts the analysis into structured search criteria
func (p *TranscriptProcessor) generateSearchCriteria(ctx context.Context, user *User) (*search.Criteria, error) {
	p.logger.Debug("generating search criteria", "user_id", user.ID)
	defer p.logger.Debug("generated search criteria", "user_id", user.ID)

	// Check if user.Search is nil
	if user.Search == nil {
		return nil, fmt.Errorf("user search is not initialized")
	}

	if user.Search.Statement == "" {
		return nil, fmt.Errorf("no search statement found")
	}

	analysis := user.Search.Statement

	schema, err := minds.NewResponseSchema("analysis", "Structured analysis of the transcript", search.Criteria{})
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	messages := []minds.Message{
		{
			Role: minds.RoleUser,
			Content: `Given the following statement of job search preferences, 
			generate a JSON response with search criteria. 
			Include only keywords, desireable locations (including remote) and relevant job preferences: ` + analysis,
		},
	}

	req := minds.NewRequest(messages, minds.WithResponseSchema(*schema))

	searchResp, err := p.llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var criteria search.Criteria
	if err := json.Unmarshal([]byte(searchResp.String()), &criteria); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search criteria: %w", err)
	}

	// Create UserSearch with the criteria
	return &criteria, nil
}
