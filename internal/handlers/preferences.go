package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const preferencesPrompt = `
	Analyze interview transcripts and extract job preferences.
	Focus on location preferences, work style, company preferences, and salary expectations.

	Provide your response in JSON.`

type Preferences struct {
	Summary            string       `json:"summary" description:"A summary of the preferences of the candidate or details not covered in the other fields"`
	PreferredLocations []string     `json:"preferredLocations" description:"The preferred locations of the candidate"`
	RemotePreference   string       `json:"remotePreference" description:"The preferred remote preference of the candidate" enum:"remote,hybrid,onsite"`
	WorkStyle          string       `json:"workStyle" description:"The preferred work style of the candidate" enum:"collaborative,independent"`
	CompanySize        string       `json:"companySize" description:"The preferred company size of the candidate" enum:"startup,mid-size,enterprise"`
	Industry           []string     `json:"industry" description:"The preferred industry of the candidate"`
	SalaryRange        *SalaryRange `json:"salaryRange,omitempty" description:"The preferred salary range of the candidate"`
}

type PreferencesHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewPreferencesHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*PreferencesHandler, error) {
	handler := &PreferencesHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "preferences_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-TRANSCRIPT-PREFERENCES-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register preferences handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *PreferencesHandler) GetSystemPrompt() string {
	return preferencesPrompt
}

// GetResponseSchema implements LLMProcessor
func (h *PreferencesHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Preferences", "Job preferences", Preferences{})
}

// GetOutputTopic implements LLMProcessor
func (h *PreferencesHandler) GetOutputTopic() string {
	return TopicPreferencesExtracted
}
