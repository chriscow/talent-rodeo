package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"talent-rodeo/internal/database"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	workplacePrompt = `You are an expert workplace analyst. Your task is to extract and structure workplace information from job descriptions.
		Focus on identifying:
		- Location details (remote, hybrid, onsite)
		- Work schedule and hours
		- Office environment
		- Travel requirements

		Provide structured output with clear categorization of all workplace aspects.`
)

// Workplace defines the workplace details for a job
type Workplace struct {
	Summary     string   `json:"summary" description:"A summary of the workplace details or details not covered in the other fields"`
	Location    string   `json:"location" description:"Location details (remote, hybrid, onsite)"`
	Type        string   `json:"type" description:"Type of workplace (office, remote, hybrid)"`
	Schedule    string   `json:"schedule" description:"Work schedule and hours"`
	Environment string   `json:"environment" description:"Office environment description"`
	Travel      []string `json:"travel" description:"Travel requirements"`
}

type WorkplaceHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
	db      *database.DB
}

func NewWorkplaceHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger, db *database.DB) (*WorkplaceHandler, error) {
	handler := &WorkplaceHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "workplace_handler")),
		generic: NewGenericHandler(js, logger, llm),
		db:      db,
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-WORKPLACE-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register workplace handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *WorkplaceHandler) GetSystemPrompt() string {
	return workplacePrompt
}

// GetResponseSchema implements LLMProcessor
func (h *WorkplaceHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Workplace", "Workplace details", Workplace{})
}

// GetOutputTopic implements LLMProcessor
func (h *WorkplaceHandler) GetOutputTopic() string {
	return TopicWorkplaceExtracted
}

// StoreWorkplacePreference stores the workplace preference in the database
func (h *WorkplaceHandler) StoreWorkplacePreference(ctx context.Context, candidateID string, workplace *Workplace) error {
	h.logger.Info("storing workplace preference",
		slog.String("candidate_id", candidateID),
		slog.String("type", workplace.Type),
		slog.String("location", workplace.Location))

	return h.db.WithTx(ctx, func(tx *database.DB) error {
		// Store the work location type preference
		_, err := tx.ExecContext(ctx, `
			INSERT INTO candidate_preferences (
				candidate_id, category, name, value, is_search_term
			) VALUES (
				$1, 'work_location', $2, $3, true
			)
			ON CONFLICT (candidate_id, category, name) DO UPDATE SET
				value = EXCLUDED.value,
				is_search_term = true,
				updated_at = NOW()`,
			candidateID,
			workplace.Type,
			h.mapWorkplaceTypeToValue(workplace.Type))
		if err != nil {
			return fmt.Errorf("failed to store work location preference: %w", err)
		}

		// Store schedule preference if present
		if workplace.Schedule != "" {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO candidate_preferences (
					candidate_id, category, name, value
				) VALUES (
					$1, 'schedule', $2, 3
				)
				ON CONFLICT (candidate_id, category, name) DO UPDATE SET
					value = EXCLUDED.value,
					updated_at = NOW()`,
				candidateID,
				workplace.Schedule)
			if err != nil {
				return fmt.Errorf("failed to store schedule preference: %w", err)
			}
		}

		// Store travel requirements if present
		for _, travel := range workplace.Travel {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO candidate_preferences (
					candidate_id, category, name, value
				) VALUES (
					$1, 'travel', $2, 3
				)
				ON CONFLICT (candidate_id, category, name) DO UPDATE SET
					value = EXCLUDED.value,
					updated_at = NOW()`,
				candidateID,
				travel)
			if err != nil {
				return fmt.Errorf("failed to store travel preference: %w", err)
			}
		}

		return nil
	})
}

// GetWorkplacePreferences retrieves workplace preferences for a candidate
func (h *WorkplaceHandler) GetWorkplacePreferences(ctx context.Context, candidateID string) (*Workplace, error) {
	h.logger.Info("retrieving workplace preferences",
		slog.String("candidate_id", candidateID))

	workplace := &Workplace{}

	// Get work location preference
	var workLocPref struct {
		Name  string
		Value int
	}
	err := h.db.GetContext(ctx, &workLocPref, `
		SELECT name, value
		FROM candidate_preferences
		WHERE candidate_id = $1
		AND category = 'work_location'
		ORDER BY updated_at DESC
		LIMIT 1`,
		candidateID)
	if err != nil {
		if err == sql.ErrNoRows {
			h.logger.Debug("no work location preference found",
				slog.String("candidate_id", candidateID))
		} else {
			return nil, fmt.Errorf("failed to get work location preference: %w", err)
		}
	} else {
		workplace.Type = workLocPref.Name
		workplace.Location = workLocPref.Name
	}

	// Get schedule preference
	err = h.db.GetContext(ctx, &workplace.Schedule, `
		SELECT name
		FROM candidate_preferences
		WHERE candidate_id = $1
		AND category = 'schedule'
		ORDER BY updated_at DESC
		LIMIT 1`,
		candidateID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get schedule preference: %w", err)
	}

	// Get travel requirements
	err = h.db.SelectContext(ctx, &workplace.Travel, `
		SELECT name
		FROM candidate_preferences
		WHERE candidate_id = $1
		AND category = 'travel'
		ORDER BY updated_at DESC`,
		candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get travel preferences: %w", err)
	}

	return workplace, nil
}

// mapWorkplaceTypeToValue converts workplace type to preference value
func (h *WorkplaceHandler) mapWorkplaceTypeToValue(workType string) int {
	switch workType {
	case "remote":
		return 1 // Fully Remote
	case "mostly_remote":
		return 2 // Mostly Remote
	case "hybrid":
		return 3 // Hybrid
	case "mostly_onsite":
		return 4 // Mostly On Site
	case "onsite":
		return 5 // Fully On Site
	default:
		h.logger.Warn("unknown workplace type, defaulting to hybrid",
			slog.String("type", workType))
		return 3 // Default to hybrid
	}
}

// mapValueToWorkplaceType converts preference value to workplace type
func (h *WorkplaceHandler) mapValueToWorkplaceType(value int) string {
	switch value {
	case 1:
		return "remote"
	case 2:
		return "mostly_remote"
	case 3:
		return "hybrid"
	case 4:
		return "mostly_onsite"
	case 5:
		return "onsite"
	default:
		h.logger.Warn("invalid workplace value, defaulting to hybrid",
			slog.Int("value", value))
		return "hybrid"
	}
}
