package onboarding

import "time"

// Step represents the current step in the user onboarding process
type Step string

const (
	// StepInitial is the starting state
	StepInitial Step = "initial"

	// StepResumeUploaded indicates the resume has been uploaded and processed
	StepResumeUploaded Step = "resume_uploaded"

	// StepPhoneCallScheduled indicates we've scheduled the phone call
	StepPhoneCallScheduled Step = "phone_call_scheduled"

	// StepPhoneCallCompleted indicates the phone call is done
	StepPhoneCallCompleted Step = "phone_call_completed"

	// StepPreferencesExtracted indicates we've processed the call and extracted preferences
	StepPreferencesExtracted Step = "preferences_extracted"

	// StepJobSearchStarted indicates we've begun searching for matching jobs
	StepJobSearchStarted Step = "job_search_started"

	// StepComplete indicates the full process is complete
	StepComplete Step = "complete"
)

// State contains all onboarding-related data for a user
type State struct {
	CurrentStep       Step       `db:"onboarding_step"`
	StartedAt         *time.Time `db:"onboarding_started_at"`
	CompletedAt       *time.Time `db:"onboarding_completed_at"`
	StepLastUpdatedAt *time.Time `db:"onboarding_step_updated_at"`
}

// IsComplete returns true if the onboarding process is complete
func (s *State) IsComplete() bool {
	return s.CurrentStep == StepComplete && s.CompletedAt != nil
}

// IsInProgress returns true if onboarding has started but is not complete
func (s *State) IsInProgress() bool {
	return s.StartedAt != nil && !s.IsComplete()
}

// NextStep returns the next logical step in the onboarding process
func (s *State) NextStep() Step {
	switch s.CurrentStep {
	case StepInitial:
		return StepResumeUploaded
	case StepResumeUploaded:
		return StepPhoneCallScheduled
	case StepPhoneCallScheduled:
		return StepPhoneCallCompleted
	case StepPhoneCallCompleted:
		return StepPreferencesExtracted
	case StepPreferencesExtracted:
		return StepJobSearchStarted
	case StepJobSearchStarted:
		return StepComplete
	default:
		return s.CurrentStep
	}
}
