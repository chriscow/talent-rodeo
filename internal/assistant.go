package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/chriscow/vapi-go"
)

const (
	defaultAssistantModel          = "gpt-4.1"
	defaultAssistantPromptTemplate = "prompts/shirely.general.004.md"
)

func GetAssistantID(userID string) string {
	assistantID := os.Getenv("VAPI_ASSISTANT_ID")
	if assistantID == "" {
		panic(fmt.Errorf("VAPI_ASSISTANT_ID is not set"))
	}
	return assistantID
}

func GetAssistantPromptTemplate(userID string) (*utils.Prompt, error) {
	// TODO: Get the custom template for the user
	prompt, err := utils.CreateTemplate(defaultAssistantPromptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to create prompt template: %w", err)
	}
	return &prompt, nil
}

// BuildAssistantOverrides creates assistant overrides for a given user ID.
// It returns the assistant request response which includes the overrides and any error encountered.
func BuildAssistantOverrides(ctx context.Context, user *User, fs *FirestoreClient, logger *slog.Logger) (*vapi.Assistant, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}

	if os.Getenv("VAPI_WEBHOOK_URL") == "" {
		return nil, fmt.Errorf("VAPI_WEBHOOK_URL is not set")
	}

	assistantID := GetAssistantID(user.ID)

	// Create template and data structure
	tmpl, err := GetAssistantPromptTemplate(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get assistant template: %w", err)
	}

	resume, found, err := GetLatestResume(ctx, fs, user.ID)
	if err != nil || !found || resume == nil {
		return nil, fmt.Errorf("failed to get latest resume (userID:%s) (found:%v) : %w", user.ID, found, err)
	}

	resumeText := ""
	if resume != nil {
		resumeText = resume.RawText
	}

	fullName := ""
	if user != nil {
		fullName = user.Name
	}
	firstName := ""
	if user != nil {
		firstName = user.FirstName
	}

	searchStrategy := ""
	if resume.SearchAnalysis != nil {
		searchStrategy = *resume.SearchAnalysis
	}

	// Get previous discussions if needed
	phoneCalls, err := ListAppointments(ctx, fs, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list appointments: %w", err)
	}

	type previousDiscussion struct {
		Summary string
		When    string
		Status  string
	}
	previousDiscussions := []previousDiscussion{}

	for _, phoneCall := range phoneCalls {
		endReason := ""
		if phoneCall.EndReason != nil {
			endReason = *phoneCall.EndReason
		}

		when := ""
		if phoneCall.CompletedAt != nil {
			when = funcs.ApproxDuration(-time.Until(*phoneCall.CompletedAt)) + " ago"
		}
		if phoneCall.Summary != nil {
			previousDiscussions = append(previousDiscussions, previousDiscussion{
				Summary: *phoneCall.Summary,
				// time.Until returns negative for past times, so we negate it to get a positive duration
				When:   when,
				Status: endReason,
			})
		}
	}

	data := struct {
		FirstName          string
		FullName           string
		Resume             string
		SearchStrategy     string
		PreviousDiscussion []previousDiscussion
	}{
		FirstName:          firstName,
		FullName:           fullName,
		Resume:             resumeText,
		SearchStrategy:     searchStrategy,
		PreviousDiscussion: previousDiscussions,
	}

	prompt, err := tmpl.Execute(data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	model := vapi.DefaultModelConfig
	model.Model = defaultAssistantModel
	model.Messages = []vapi.ModelMessage{
		{
			Role:    "system",
			Content: prompt,
		},
	}

	firstMessage := ""
	if len(data.PreviousDiscussion) > 0 {
		firstMessage = fmt.Sprintf("Hey! Hi there! I'm Shirley from Talent Rodeo. Thanks for calling back, %s! How is your job search going?", user.FirstName)
	} else {
		firstMessage = "Hey! Hi there! I'm Shirley from Talent Rodeo. Am I speaking with " + user.FirstName + "?"
	}

	logger.Info("built assistant overrides",
		"userID", user.ID,
		"assistantId", assistantID,
		"firstMessage", firstMessage,
		"webhookURL", os.Getenv("VAPI_WEBHOOK_URL"),
	)

	return &vapi.Assistant{
		FirstMessage: &firstMessage,
		Model:        &model,
		Server: &vapi.ServerConfig{
			URL:            os.Getenv("VAPI_WEBHOOK_URL"),
			TimeoutSeconds: 30,
		},
	}, nil
}
