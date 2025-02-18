package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAssessMatch(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input profiles
	candidate := CandidateProfile{
		CandidateID: "test-candidate",
		Skills: Skills{
			Skills: []Skill{
				{
					Name:       "Go",
					Level:      "expert",
					YearsUsed:  5,
					Confidence: "high",
				},
				{
					Name:       "React",
					Level:      "intermediate",
					YearsUsed:  2,
					Confidence: "high",
				},
			},
		},
		Experience: &Experience{
			TotalYears: 7,
			Roles: []Role{
				{
					Title:       "Senior Developer",
					Company:     "TechCorp",
					YearsInRole: 3,
					Skills:      []string{"Go", "React"},
				},
			},
		},
		Preferences: &Preferences{
			PreferredLocations: []string{"San Francisco", "Remote"},
			RemotePreference:   "remote",
			WorkStyle:          "collaborative",
			CompanySize:        "startup",
			Industry:           []string{"tech"},
			SalaryRange: &SalaryRange{
				Min:      120000,
				Max:      180000,
				Currency: "USD",
				Period:   "yearly",
			},
		},
		Culture: &Culture{
			Description:  "Values innovation and collaboration",
			Values:       []string{"Innovation", "Collaboration"},
			TeamDynamics: "Highly collaborative",
			WorkStyle:    "Fast-paced",
		},
	}

	job := JobProfile{
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
			NiceToHave: []Requirement{
				{
					Skill:   "React",
					Level:   "intermediate",
					Context: "Frontend development",
				},
			},
			MinYearsExperience: 5,
		},
		Compensation: &Compensation{
			SalaryRange: &SalaryRange{
				Min:      130000,
				Max:      170000,
				Currency: "USD",
				Period:   "yearly",
			},
		},
		Workplace: &Workplace{
			Location:    "Remote",
			Type:        "remote",
			Schedule:    "Flexible",
			Environment: "Collaborative startup environment",
		},
		Culture: "We are a fast-paced startup focused on innovation and collaboration",
	}

	input := AssessmentInput{
		Candidate: candidate,
		Job:       job,
	}

	// Expected assessment result
	want := &MatchAssessment{
		Score:      0.85,
		Confidence: "high",
		SkillsMatch: &SkillsMatch{
			Score:         0.9,
			MatchedSkills: []string{"Go", "React"},
			MissingSkills: []string{},
			GapAnalysis:   "Strong match on required skills",
		},
		ExperienceMatch: &ExperienceMatch{
			Score:         0.8,
			YearsMatch:    true,
			LevelMatch:    true,
			RoleAlignment: "Strong alignment with senior developer role",
		},
		LocationMatch: &LocationMatch{
			Score:      1.0,
			Compatible: true,
			Reason:     "Perfect match for remote work preference",
		},
		CompensationFit: &CompensationFit{
			Score:       0.9,
			Compatible:  true,
			Explanation: "Salary expectations align well with offered range",
		},
		Analysis: "Strong overall match with excellent skill alignment and cultural fit",
		Recommendations: []Recommendation{
			{
				Area:        "skills",
				Suggestion:  "Consider learning additional cloud technologies",
				Priority:    "low",
				TimeToReady: "3-6 months",
			},
		},
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal assessment: %v", err)
	}

	// Create handler
	handler, err := NewAssessmentHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Set the LLM response for the assessment prompt
	env.llm.responses["You are an expert talent matcher analyzing the fit between candidates and job positions."] = string(wantJSON)

	// Test AssessMatch
	got, err := handler.assessMatch(context.Background(), input)
	if err != nil {
		t.Fatalf("assessMatch failed: %v", err)
	}

	// Validate assessment result
	if got.Score != want.Score {
		t.Errorf("got score %v, want %v", got.Score, want.Score)
	}
	if got.Confidence != want.Confidence {
		t.Errorf("got confidence %q, want %q", got.Confidence, want.Confidence)
	}

	// Validate skills match
	if got.SkillsMatch.Score != want.SkillsMatch.Score {
		t.Errorf("got skills score %v, want %v", got.SkillsMatch.Score, want.SkillsMatch.Score)
	}
	if len(got.SkillsMatch.MatchedSkills) != len(want.SkillsMatch.MatchedSkills) {
		t.Errorf("got %d matched skills, want %d", len(got.SkillsMatch.MatchedSkills), len(want.SkillsMatch.MatchedSkills))
	}

	// Validate experience match
	if got.ExperienceMatch.Score != want.ExperienceMatch.Score {
		t.Errorf("got experience score %v, want %v", got.ExperienceMatch.Score, want.ExperienceMatch.Score)
	}
	if got.ExperienceMatch.YearsMatch != want.ExperienceMatch.YearsMatch {
		t.Errorf("got years match %v, want %v", got.ExperienceMatch.YearsMatch, want.ExperienceMatch.YearsMatch)
	}

	// Validate location match
	if got.LocationMatch.Score != want.LocationMatch.Score {
		t.Errorf("got location score %v, want %v", got.LocationMatch.Score, want.LocationMatch.Score)
	}
	if got.LocationMatch.Compatible != want.LocationMatch.Compatible {
		t.Errorf("got location compatible %v, want %v", got.LocationMatch.Compatible, want.LocationMatch.Compatible)
	}

	// Validate compensation fit
	if got.CompensationFit.Score != want.CompensationFit.Score {
		t.Errorf("got compensation score %v, want %v", got.CompensationFit.Score, want.CompensationFit.Score)
	}
	if got.CompensationFit.Compatible != want.CompensationFit.Compatible {
		t.Errorf("got compensation compatible %v, want %v", got.CompensationFit.Compatible, want.CompensationFit.Compatible)
	}
}

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

	env.llm.responses["You are an expert talent matcher analyzing the fit between candidates and job positions."] = string(wantJSON)

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewAssessmentHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

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
