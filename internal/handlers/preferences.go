package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	preferencesPrompt = `
		Analyze interview transcripts and extract job preferences.
		Focus on location preferences, work style, company preferences, and salary expectations.

		Provide your response in JSON.`
	TopicPreferencesExtracted = "rodeo.transcript.extracted.preferences"
)

type Preferences struct {
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
		generic: NewGenericHandler(js, logger),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-TRANSCRIPT-PREFERENCES-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register preferences handler: %w", err)
	}

	return handler, nil
}

func (h *PreferencesHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing preferences message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	transcript := string(msg.Data())

	preferences, err := h.extract(ctx, transcript)
	if err != nil {
		return fmt.Errorf("failed to extract preferences: %w", err)
	}

	preferencesJSON, err := json.Marshal(preferences)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	h.logger.Debug("Publishing preferences",
		"locations", len(preferences.PreferredLocations),
		"remote", preferences.RemotePreference,
		"work_style", preferences.WorkStyle,
		"company_size", preferences.CompanySize)

	_, err = h.generic.js.Publish(ctx, TopicPreferencesExtracted, preferencesJSON)
	if err != nil {
		return fmt.Errorf("failed to publish preferences: %w", err)
	}

	return nil
}

func (h *PreferencesHandler) extract(ctx context.Context, transcript string) (*Preferences, error) {
	msgs := minds.Messages{{
		Role:    minds.RoleSystem,
		Content: preferencesPrompt,
	},
		{
			Role:    minds.RoleUser,
			Content: transcript,
		},
	}

	schema, err := minds.NewResponseSchema("Preferences", "Job preferences", Preferences{})
	if err != nil {
		return nil, fmt.Errorf("failed to create preferences schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting preferences from transcript")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract preferences: %w", err)
	}

	var output Preferences
	if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal preferences: %w", err)
	}

	h.logger.Debug("Extracted preferences",
		"locations", len(output.PreferredLocations),
		"remote", output.RemotePreference,
		"work_style", output.WorkStyle,
		"company_size", output.CompanySize)

	return &output, nil
}
