package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"talent-rodeo/internal/database"
	"time"

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
	ctx      context.Context
	db       *database.DB
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewAssessmentHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*AssessmentHandler, error) {
	const assessmentConsumerName = "RODEO-ASSESSMENT-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, assessmentConsumerName, TopicTranscriptInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &AssessmentHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

func (h *AssessmentHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting assessment handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping assessment handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *AssessmentHandler) Close() error {
	return nil
}

func (h *AssessmentHandler) processNextMessage(ctx context.Context) error {
	msgs, err := h.consumer.Fetch(1, jetstream.FetchMaxWait(250*time.Millisecond))
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return nil
		}
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	for msg := range msgs.Messages() {
		if err := h.handleMessage(ctx, msg); err != nil {
			h.logger.Error("Failed to handle message", "error", err)
			msg.Term()
			return err
		}
	}
	return nil
}

func (h *AssessmentHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	var input AssessmentInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		msg.Ack()
		return nil
	}

	assessment, err := h.assessMatch(ctx, input)
	if err != nil {
		h.logger.Error("Failed to assess match", "error", err)
		msg.Ack()
		return fmt.Errorf("failed to assess match: %w", err)
	}

	h.logger.Debug("Assessment result", "assessment", assessment)

	assessmentJSON, err := json.Marshal(assessment)
	if err != nil {
		h.logger.Error("Failed to marshal assessment", "error", err)
		msg.Ack()
		return fmt.Errorf("failed to marshal assessment: %w", err)
	}

	_, err = h.js.Publish(ctx, TopicAssessmentResult, assessmentJSON)
	if err != nil {
		h.logger.Error("Failed to publish assessment", "error", err)
		msg.Ack()
		return fmt.Errorf("failed to publish assessment: %w", err)
	}

	msg.Ack()
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

// AssessMatch performs the actual assessment using the LLM
func (h *AssessmentHandler) assessMatch(ctx context.Context, input AssessmentInput) (*MatchAssessment, error) {
	userContent, err := generateAssessmentPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("failed to generate prompt: %w", err)
	}

	h.logger.Debug("Generated assessment prompt", "prompt", userContent)

	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: "You are an expert talent matcher analyzing the fit between candidates and job positions.",
		},
		{
			Role:    minds.RoleUser,
			Content: userContent,
		},
	}

	schema, err := minds.NewResponseSchema("MatchAssessment", "Comprehensive match assessment", MatchAssessment{})
	if err != nil {
		return nil, fmt.Errorf("failed to create assessment schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Generating content with LLM", "request", request)

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to assess match: %w", err)
	}

	h.logger.Debug("Received LLM response", "response", result.String())

	var assessment MatchAssessment
	if err := json.Unmarshal([]byte(result.String()), &assessment); err != nil {
		h.logger.Error("Failed to unmarshal assessment", "error", err, "response", result.String())
		return nil, fmt.Errorf("failed to unmarshal assessment: %w", err)
	}

	h.logger.Debug("Successfully unmarshaled assessment", "score", assessment.Score, "confidence", assessment.Confidence)

	// store the assessment in the database if we received a database connection
	if h.db != nil {
		store(h.db, input.Candidate.CandidateID, input.Job.JobID, assessment.Score, assessment.Confidence, assessment.Analysis)
	}

	return &assessment, nil
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
