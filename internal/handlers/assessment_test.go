package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleAssessmentMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input
	input := AssessmentInput{
		Candidate: CandidateProfile{
			CandidateID: "test-candidate",
			Skills: Skills{
				Skills: []Skill{
					{
						Name:       "Go",
						Level:      "expert",
						YearsUsed:  5,
						Confidence: "high",
					},
				},
			},
		},
		Job: JobProfile{
			JobID: "test-job",
			Requirements: &Requirements{
				MustHave: []Requirement{
					{
						Skill:       "Go",
						YearsNeeded: 4,
						Level:       "expert",
						Context:     "Backend development",
					},
				},
			},
		},
	}

	// Expected assessment result
	want := &MatchAssessment{
		Score:      0.85,
		Confidence: "high",
		SkillsMatch: &SkillsMatch{
			Score:         0.9,
			MatchedSkills: []string{"Go"},
			MissingSkills: []string{},
			GapAnalysis:   "Strong match on required skills",
		},
		Analysis: "Strong overall match with excellent skill alignment",
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	// Create handler with a background context since we don't need to control its lifecycle.
	handler, err := NewAssessmentHandler(context.Background(), env.js, env.llm, env.logger, env.db)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set the LLM response for the system prompt
	env.llm.responses[handler.GetSystemPrompt()] = string(wantJSON)

	// Send test message and verify response
	got, err := SendAndReceiveMessage[AssessmentInput, MatchAssessment](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicAssessmentInput,
		TopicAssessmentResult,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Validate the assessment result
	if got.Score != want.Score {
		t.Errorf("got score %v, want %v", got.Score, want.Score)
	}
	if got.Confidence != want.Confidence {
		t.Errorf("got confidence %q, want %q", got.Confidence, want.Confidence)
	}
	if got.SkillsMatch.Score != want.SkillsMatch.Score {
		t.Errorf("got skills score %v, want %v", got.SkillsMatch.Score, want.SkillsMatch.Score)
	}
}
