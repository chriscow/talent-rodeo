package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal/database"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	skillsPrompt = `
		You analyze interview transcripts and extract all technical and professional skills.
		For each skill, determine the level of expertise and years of experience based on the context.

		Provide your response in JSON.`

	TopicSkillsExtracted = "rodeo.transcript.extracted.skills"

	concurrentSkillsProcessing = 10
)

type Skills struct {
	Skills []Skill `json:"skills"`
}

type Skill struct {
	Summary    *string `json:"summary" description:"A summary of the skill as given in the transcript or details not covered in the other fields"`
	Name       string  `json:"name" description:"The name of the skill"`
	Level      string  `json:"level" description:"The level of expertise of the skill" enum:"expert,intermediate,beginner"`
	YearsUsed  float64 `json:"yearsUsed" description:"The number of years of experience with the skill"`
	Confidence string  `json:"confidence" description:"confidence in the extraction" enum:"low,medium,high"`
}

type SkillsHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
	db      *database.DB
}

func NewSkillsHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger, db *database.DB) (*SkillsHandler, error) {
	handler := &SkillsHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "skills_handler")),
		generic: NewGenericHandler(js, logger, llm),
		db:      db,
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-TRANSCRIPT-SKILLS-QUEUE", handler, concurrentSkillsProcessing); err != nil {
		return nil, fmt.Errorf("failed to register skills handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *SkillsHandler) GetSystemPrompt() string {
	return skillsPrompt
}

// GetResponseSchema implements LLMProcessor
func (h *SkillsHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Skills", "Array of extracted skills", Skills{})
}

// GetOutputTopic implements LLMProcessor
func (h *SkillsHandler) GetOutputTopic() string {
	return TopicSkillsExtracted
}

// StoreSkills stores the extracted skills in the database
func (h *SkillsHandler) StoreSkills(ctx context.Context, candidateID string, skills []Skill) error {
	h.logger.Info("storing skills in database",
		slog.String("candidate_id", candidateID),
		slog.Int("skill_count", len(skills)))

	// Convert Skill to CandidateSkill
	dbSkills := make([]database.CandidateSkill, len(skills))
	for i, skill := range skills {
		dbSkills[i] = database.CandidateSkill{
			CandidateID:     candidateID,
			Name:            skill.Name,
			Level:           skill.Level,
			YearsUsed:       skill.YearsUsed,
			Confidence:      skill.Confidence,
			Summary:         skill.Summary,
			PreferenceValue: 3, // Default to neutral
			IsSearchTerm:    false,
		}
	}

	return h.db.StoreSkills(ctx, candidateID, dbSkills)
}

// GetCandidateSkills retrieves all skills for a candidate
func (h *SkillsHandler) GetCandidateSkills(ctx context.Context, candidateID string) ([]database.CandidateSkill, error) {
	h.logger.Info("retrieving skills from database",
		slog.String("candidate_id", candidateID))

	return h.db.GetCandidateSkills(ctx, candidateID)
}

// UpdateSkillPreferences updates the preference values and search term flags for skills
func (h *SkillsHandler) UpdateSkillPreferences(ctx context.Context, candidateID string, updates map[string]struct {
	PreferenceValue int
	IsSearchTerm    bool
}) error {
	h.logger.Info("updating skill preferences",
		slog.String("candidate_id", candidateID),
		slog.Int("update_count", len(updates)))

	return h.db.UpdateSkillPreferences(ctx, candidateID, updates)
}
