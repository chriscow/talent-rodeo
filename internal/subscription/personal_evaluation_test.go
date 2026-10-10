package subscription

import (
	"talent-rodeo/internal"
	"testing"
	"time"
)

func TestPersonalEvaluationVersionTracksInputs(t *testing.T) {
	activation := time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)
	statement := "I prefer remote Go roles"
	statementID := "statement-v1"
	user := internal.User{PersonalSearchStartedAt: &activation, CurrentStatementVersionID: &statementID, Statement: &statement}
	job := internal.JobDetail{Description: "Build services", WorkplaceType: "remote", Location: "Phoenix"}
	baseline := newPersonalEvaluationVersion(user, job, []byte("prompt"), "analysis-v1", "extraction-v1")
	if !personalEvaluationMatches(baseline, baseline) {
		t.Fatal("identical versions do not match")
	}
	tests := []struct {
		name    string
		version *internal.PersonalEvaluationVersion
	}{
		{name: "legacy", version: nil},
		{name: "activation", version: newPersonalEvaluationVersion(internal.User{PersonalSearchStartedAt: ptrTime(activation.Add(time.Hour)), CurrentStatementVersionID: &statementID, Statement: &statement}, job, []byte("prompt"), "analysis-v1", "extraction-v1")},
		{name: "job description", version: newPersonalEvaluationVersion(user, internal.JobDetail{Description: "Build mobile apps", WorkplaceType: "remote", Location: "Phoenix"}, []byte("prompt"), "analysis-v1", "extraction-v1")},
		{name: "model", version: newPersonalEvaluationVersion(user, job, []byte("prompt"), "analysis-v2", "extraction-v1")},
		{name: "prompt", version: newPersonalEvaluationVersion(user, job, []byte("revised prompt"), "analysis-v1", "extraction-v1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if personalEvaluationMatches(tt.version, baseline) {
				t.Errorf("personalEvaluationMatches(%s) = true, want false", tt.name)
			}
		})
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestValidateMatchAnalysis(t *testing.T) {
	valid := internal.JobMatchAnalysis{
		Summary: "A strong match", Strengths: "Go experience", Concerns: "Limited salary information",
		Rating: internal.Rating{NumericRating: 4, RatingExplanation: "Your experience fits the role."},
	}
	tests := []struct {
		name     string
		analysis internal.JobMatchAnalysis
		wantErr  bool
	}{
		{name: "valid", analysis: valid},
		{name: "empty object", analysis: internal.JobMatchAnalysis{}, wantErr: true},
		{name: "rating too high", analysis: internal.JobMatchAnalysis{Summary: valid.Summary, Strengths: valid.Strengths, Concerns: valid.Concerns, Rating: internal.Rating{NumericRating: 6, RatingExplanation: valid.Rating.RatingExplanation}}, wantErr: true},
		{name: "missing rationale", analysis: internal.JobMatchAnalysis{Summary: valid.Summary, Strengths: valid.Strengths, Concerns: valid.Concerns, Rating: internal.Rating{NumericRating: 4}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateMatchAnalysis(tt.analysis); (err != nil) != tt.wantErr {
				t.Errorf("validateMatchAnalysis(%s) error = %v, wantErr %t", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestMayUpdateEvaluatedStatus(t *testing.T) {
	tests := []struct {
		name string
		job  internal.UserJob
		want bool
	}{
		{name: "new", job: internal.UserJob{Status: internal.JobStatusNew}, want: true},
		{name: "automatic archive", job: internal.UserJob{Status: internal.JobStatusArchived, AutoArchivedByEvaluation: true}, want: true},
		{name: "manual archive", job: internal.UserJob{Status: internal.JobStatusArchived, StatusManuallySet: true}},
		{name: "legacy archive", job: internal.UserJob{Status: internal.JobStatusArchived}},
		{name: "saved", job: internal.UserJob{Status: internal.JobStatusSaved, StatusManuallySet: true}},
		{name: "manual new", job: internal.UserJob{Status: internal.JobStatusNew, StatusManuallySet: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mayUpdateEvaluatedStatus(tt.job); got != tt.want {
				t.Errorf("mayUpdateEvaluatedStatus(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
