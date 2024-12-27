package main

import (
	"text/template"
	"time"

	"github.com/chriscow/minds"
)

// Enums as Go constants
const (
	ConversationStagePlanningContact = "planning_contact"
	ConversationStageInitialContact  = "initial_contact"
	ConversationStageDiscovery       = "discovery"
	ConversationStageDeepDive        = "deep_dive"
	ConversationStageFollowUp        = "follow_up"
	ConversationStageClosing         = "closing"

	EngagementLevelHigh   = "high"
	EngagementLevelMedium = "medium"
	EngagementLevelLow    = "low"
)

// Structs for state management
type CandidateInfo struct {
	KnownFacts          []string       `json:"known_facts"`
	InferredPreferences []string       `json:"inferred_preferences"`
	PotentialConcerns   []string       `json:"potential_concerns"`
	EngagementLevel     string         `json:"engagement_level"`
	Resume              map[string]any `json:"resume"`
	Name                string         `json:"name"`
	PhoneNumber         string         `json:"phone_number"`
}

type GoalProgress struct {
	GoalName    string   `json:"goal"`
	Status      bool     `json:"status"`
	RelatedInfo []string `json:"related_info"`
	Priority    int      `json:"priority"`
}

type CallCompletionCriteria struct {
	BasicInfoCollected   bool `json:"basic_info_collected"`
	CareerGoalsDiscussed bool `json:"career_goals_discussed"`
	SkillsAssessed       bool `json:"skills_assessed"`
	NextStepsPlanned     bool `json:"next_steps_planned"`
}

type CallContext struct {
	Today              string                 `json:"today"`
	CallID             string                 `json:"call_id"`
	StartTime          *time.Time             `json:"start_time"`
	Duration           *time.Duration         `json:"duration"`
	CompletionCriteria CallCompletionCriteria `json:"completion_criteria"`
	GoalsProgress      []GoalProgress         `json:"goals_progress"`
}

type ConversationState struct {
	Stage         string        `json:"stage" yaml:"stage"`
	CandidateInfo CandidateInfo `json:"candidate_info"`
	CallContext   CallContext   `json:"call_context"`
}

func (s *ConversationState) InitializeCall() {
	s.CallContext.GoalsProgress = []GoalProgress{
		{GoalName: "establish_rapport", Priority: 1},
		{GoalName: "gather_background", Priority: 2},
		{GoalName: "understand_motivations", Priority: 3},
		{GoalName: "discuss_skills", Priority: 4},
		{GoalName: "plan_next_steps", Priority: 5},
	}
	s.CallContext.CompletionCriteria = CallCompletionCriteria{}
}

func (s *ConversationState) ShouldEndCall() bool {
	for _, goal := range s.CallContext.GoalsProgress {
		if !goal.Status {
			return false
		}
	}
	return true
}

func (s *ConversationState) GetNextGoal() *string {
	var nextGoal *string
	for _, goal := range s.CallContext.GoalsProgress {
		if !goal.Status {
			nextGoal = &goal.GoalName
			break
		}
	}
	return nextGoal
}

// System template using text/template
const systemTemplate = `
Today: {{ .Today }}
State: {{ .State }}
`

type Recruiter struct {
	llm            minds.ContentGenerator
	state          ConversationState
	systemTemplate *template.Template
}

func NewRecruiter() *Recruiter {
	tmpl, err := template.New("systemTemplate").Parse(systemTemplate)
	if err != nil {
		panic(err)
	}

	return &Recruiter{
		state: ConversationState{
			Stage: ConversationStageInitialContact,
			CandidateInfo: CandidateInfo{
				EngagementLevel: EngagementLevelMedium,
			},
			CallContext: CallContext{
				Today:  time.Now().Format(time.RFC3339),
				CallID: "initial_call",
			},
		},
		systemTemplate: tmpl,
	}
}

func (r *Recruiter) String() string {
	return "Sequential: Recruiter"
}

func (r *Recruiter) HandleThread(tc minds.ThreadContext, next minds.ThreadHandler) (minds.ThreadContext, error) {
	return next.HandleThread(tc, nil)
}

// func (r *Recruiter) Respond(userInput string) string {
// 	// Render the template with the state
// 	var rendered string
// 	templateData := map[string]any{
// 		"Today": time.Now().Format(time.RFC3339),
// 		"State": r.State,
// 	}
// 	err := r.SystemTemplate.Execute(&rendered, templateData)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// Prepare the request for LLM (e.g., OpenAI)
// 	// Simulated response as this example doesn't actually call the LLM
// 	response := fmt.Sprintf("Responding to: %s with state: %s", userInput, rendered)
// 	return response
// }
