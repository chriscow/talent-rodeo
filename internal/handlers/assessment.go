package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"talent-rodeo/internal/database"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	TopicAssessmentInput  = "rodeo.assessment.input"
	TopicAssessmentResult = "rodeo.assessment.result"
)

type CandidateProfile struct {
	CandidateID string `json:"candidateID" description:"Candidate identifier"`
	Skills      Skills
	Experience  *Experience
	Preferences *Preferences
	Culture     *Culture
}

func NewCandidateProfile(candidateID string, skills Skills, experience *Experience, preferences *Preferences, culture *Culture) CandidateProfile {
	return CandidateProfile{
		CandidateID: candidateID,
		Skills:      skills,
		Experience:  experience,
		Preferences: preferences,
		Culture:     culture,
	}
}

type JobProfile struct {
	JobID        string `json:"jobID" description:"Job identifier"`
	Requirements *Requirements
	Compensation *Compensation
	Workplace    *Workplace
	Culture      string `json:"culture" description:"Description of the company culture"`
}

func NewJobProfile(jobID string, requirements *Requirements, compensation *Compensation, workplace *Workplace, culture string) JobProfile {
	return JobProfile{
		JobID:        jobID,
		Requirements: requirements,
		Compensation: compensation,
	}
}

type MatchAssessment struct {
	Score           float64          `json:"score" description:"0-1 overall match score"` // 0-1 overall match score
	Confidence      string           `json:"confidence" enum:"low,medium,high"`           // 0-1 confidence in the assessment
	SkillsMatch     *SkillsMatch     `json:"skillsMatch"`
	ExperienceMatch *ExperienceMatch `json:"experienceMatch"`
	LocationMatch   *LocationMatch   `json:"locationMatch"`
	CompensationFit *CompensationFit `json:"compensationFit"`
	Analysis        string           `json:"analysis" description:"Detailed analysis of the match"`         // Detailed analysis of the match
	Recommendations []Recommendation `json:"recommendations" description:"Recommendations for improvement"` // Recommendations for improvement
}

type SkillsMatch struct {
	Score         float64  `json:"score" description:"0-1 score for skills match"`
	MatchedSkills []string `json:"matchedSkills" description:"Skills that are matched"`
	MissingSkills []string `json:"missingSkills" description:"Skills that are missing"`
	GapAnalysis   string   `json:"gapAnalysis" description:"Analysis of the skills gap"`
}

type ExperienceMatch struct {
	Score         float64 `json:"score" description:"0-1 score for experience match"`
	YearsMatch    bool    `json:"yearsMatch" description:"Whether the years of experience match"`
	LevelMatch    bool    `json:"levelMatch" description:"Whether the level of experience match"`
	RoleAlignment string  `json:"roleAlignment" description:"Alignment of the role"`
}

type LocationMatch struct {
	Score      float64 `json:"score" description:"0-1 score for location match"`
	Compatible bool    `json:"compatible" description:"Whether the location is compatible"`
	Reason     string  `json:"reason" description:"Reason for the location match"`
}

type CompensationFit struct {
	Score       float64 `json:"score" description:"0-1 score for compensation fit"`
	Compatible  bool    `json:"compatible" description:"Whether the compensation is compatible"`
	Explanation string  `json:"explanation" description:"Explanation for the compensation fit"`
}

type Recommendation struct {
	Area        string `json:"area" enum:"skills,experience,location,compensation" description:"Area to improve"`
	Suggestion  string `json:"suggestion" description:"What to improve"`
	Priority    string `json:"priority" enum:"high,medium,low" description:"Priority of the suggestion"`
	TimeToReady string `json:"timeToReady" description:"Estimated time to implement the suggestion"`
}

// AssessmentInput represents the input for assessment
type AssessmentInput struct {
	Candidate CandidateProfile `json:"candidate" description:"Candidate profile"`
	Job       JobProfile       `json:"job" description:"Job profile"`
}

type AssessmentHandler struct {
	db      *database.DB
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewAssessmentHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger, db *database.DB) (*AssessmentHandler, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	handler := &AssessmentHandler{
		db:      db,
		llm:     llm,
		logger:  logger.With(slog.String("component", "assessment_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicAssessmentInput, "RODEO-ASSESSMENT-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register assessment handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *AssessmentHandler) GetSystemPrompt() string {
	return "You are an expert talent matcher analyzing the fit between candidates and job positions."
}

// GetResponseSchema implements LLMProcessor
func (h *AssessmentHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("MatchAssessment", "Comprehensive match assessment", MatchAssessment{})
}

// GetOutputTopic implements LLMProcessor
func (h *AssessmentHandler) GetOutputTopic() string {
	return TopicAssessmentResult
}

func (h *AssessmentHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing assessment message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input AssessmentInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		return nil
	}

	userContent, err := generateAssessmentPrompt(input)
	if err != nil {
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	h.logger.Debug("Generated assessment prompt", "prompt", userContent)

	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: h.GetSystemPrompt(),
		},
		{
			Role:    minds.RoleUser,
			Content: userContent,
		},
	}

	schema, err := h.GetResponseSchema()
	if err != nil {
		return fmt.Errorf("failed to create assessment schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Generating content with LLM", "request", request)

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to assess match: %w", err)
	}

	h.logger.Debug("Received LLM response", "response", result.String())

	var assessment MatchAssessment
	if err := json.Unmarshal([]byte(result.String()), &assessment); err != nil {
		h.logger.Error("Failed to unmarshal assessment", "error", err, "response", result.String())
		return fmt.Errorf("failed to unmarshal assessment: %w", err)
	}

	h.logger.Debug("Successfully unmarshaled assessment", "score", assessment.Score, "confidence", assessment.Confidence)

	// Store the assessment result
	if err := store(h.db, input.Candidate.CandidateID, input.Job.JobID, assessment.Score, assessment.Confidence, assessment.Analysis); err != nil {
		h.logger.Error("Failed to store assessment",
			"error", err,
			"candidate_id", input.Candidate.CandidateID,
			"job_id", input.Job.JobID)
	}

	return nil
}

// generateAssessmentPrompt creates the prompt for the LLM
func generateAssessmentPrompt(input AssessmentInput) (string, error) {
	candidateJSON, err := json.MarshalIndent(input.Candidate, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal candidate profile: %w", err)
	}

	jobJSON, err := json.MarshalIndent(input.Job, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal job profile: %w", err)
	}

	return fmt.Sprintf("Please assess the match between this candidate and job position:\n\nCandidate Profile:\n%s\n\nJob Profile:\n%s",
		string(candidateJSON), string(jobJSON)), nil
}

// stores the assessment result in the database
func store(db *database.DB, candidateID, jobID string, score float64, confidence string, analysis string) error {
	_, err := db.Exec(`INSERT INTO match_assessments (candidate_id, job_id, score, confidence, analysis) 
                       VALUES ($1, $2, $3, $4, $5)`,
		candidateID, jobID, score, confidence, analysis)
	if err != nil {
		log.Printf("Error inserting assessment result: %v", err)
		return fmt.Errorf("failed to store assessment: %w", err)
	}
	return nil
}
