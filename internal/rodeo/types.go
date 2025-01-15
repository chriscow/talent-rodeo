package rodeo

import (
	"text/template"
	"time"

	"github.com/chriscow/minds"
)

type ConversationStage int
type EngagementLevel string

const (
	ConversationStageUnknown ConversationStage = iota
	ConversationStagePlanningContact
	ConversationStageInitialContact
	ConversationStageDiscovery
	ConversationStageDeepDive
	ConversationStageFollowUp
	ConversationStageClosing
)

const (
	EngagementLevelUnknown EngagementLevel = "unknown"
	EngagementLevelHigh    EngagementLevel = "high"
	EngagementLevelMedium  EngagementLevel = "medium"
	EngagementLevelLow     EngagementLevel = "low"
)

func (cs ConversationStage) String() string {
	switch cs {
	case ConversationStagePlanningContact:
		return "planning_contact"
	case ConversationStageInitialContact:
		return "initial_contact"
	case ConversationStageDiscovery:
		return "discovery"
	case ConversationStageDeepDive:
		return "deep_dive"
	case ConversationStageFollowUp:
		return "follow_up"
	case ConversationStageClosing:
		return "closing"
	default:
		return "unknown"
	}
}

type RecruiterTurn struct {
	// KnownFacts are facts about the candidate that are known or discovered during the call
	KnownFacts          []string        `json:"known_facts" description:"Facts about the candidate that are known or discovered during the call"`
	InferredPreferences []string        `json:"inferred_preferences" description:"Career preferences inferred from the candidate's responses"`
	PotentialConcerns   []string        `json:"potential_concerns" description:"Potential concerns about the candidate"`
	EngagementLevel     EngagementLevel `json:"engagement_level" enum:"unknown,high,medium,low"  description:"The candidate's engagement level during the call"`
	Response            string          `json:"response" description:"The recruiter's response to the candidate"`
}

func (r *RecruiterTurn) Merge(other RecruiterTurn) {
	r.KnownFacts = append(r.KnownFacts, other.KnownFacts...)
	r.InferredPreferences = append(r.InferredPreferences, other.InferredPreferences...)
	r.PotentialConcerns = append(r.PotentialConcerns, other.PotentialConcerns...)
	r.EngagementLevel = other.EngagementLevel
	r.Response = other.Response
}

// Structs for state management
//

type CandidateInfo struct {
	// Basic Information
	Name            string `json:"name" description:"The candidate's name"`
	Email           string `json:"email" description:"The candidate's email address"`
	PhoneNumber     string `json:"phone_number" description:"The candidate's phone number"`
	Timezone        string `json:"timezone" description:"The candidate's timezone"`
	LinkedInProfile string `json:"linkedin_profile" description:"The candidate's LinkedIn profile URL if available"`
	PortfolioURL    string `json:"portfolio_url" description:"URL to the candidate's portfolio or personal website"`

	// Resume Information
	// Resume            string   `json:"resume" description:"The candidate's resume in markdown format"`
	Skills            []string `json:"skills" description:"Specific skills extracted or inferred from the resume and call"`
	ExperienceSummary string   `json:"experience_summary" description:"High-level summary of the candidate's experience"`

	// Insights from the Call
	KnownFacts          []string `json:"known_facts" description:"Facts about the candidate that are known or discovered during the call"`
	InferredPreferences []string `json:"inferred_preferences" description:"Career preferences inferred from the candidate's responses"`
	UnlistedSkills      []string `json:"unlisted_skills" description:"Skills or qualifications the candidate has but didn't include on the resume"`
	CareerAspirations   string   `json:"career_aspirations" description:"Candidate's stated career goals and aspirations"`
	WorkEnvironment     string   `json:"work_environment" description:"Preferred work environment (e.g., remote, hybrid, in-office)"`
	IndustryPreferences []string `json:"industry_preferences" description:"Preferred industries or sectors the candidate is interested in"`

	// Potential Concerns and Challenges
	PotentialConcerns []string `json:"potential_concerns" description:"Potential concerns about the candidate"`
	DealBreakers      []string `json:"deal_breakers" description:"Non-negotiable factors for the candidate (e.g., salary, location, work style)"`

	// Engagement Metrics
	EngagementLevel EngagementLevel `json:"engagement_level" enum:"high,medium,low" description:"The candidate's engagement level during the call"`
	CallNotes       string          `json:"call_notes" description:"General notes or observations from the call"`

	// Recommendations and Next Steps
	RecommendedRoles    []string `json:"recommended_roles" description:"Roles or job titles that align with the candidate's skills and preferences"`
	TrainingSuggestions []string `json:"training_suggestions" description:"Suggested areas for upskilling or certification"`
	NextSteps           string   `json:"next_steps" description:"Recommended actions or follow-ups for the candidate"`
}

type GoalProgress struct {
	GoalName    string   `json:"goal"`
	Status      bool     `json:"status"`
	RelatedInfo []string `json:"related_info"`
	Priority    int      `json:"priority"`
}

type CallContext struct {
	Today         string         `json:"today"`
	CallID        string         `json:"call_id"`
	StartTime     *time.Time     `json:"start_time"`
	Duration      *time.Duration `json:"duration"`
	GoalsProgress []GoalProgress `json:"goals_progress"`
}

type ConversationState struct {
	Stage     ConversationStage `json:"stage" enum:"planning_contact,initial_contact,discovery,deep_dive,follow_up,closing" description:"The stage of the conversation"`
	Candidate CandidateInfo     `json:"candidate"`
	Context   CallContext       `json:"context"`
}

func (s *ConversationState) InitializeCall() {
	s.Context.GoalsProgress = []GoalProgress{
		{GoalName: "establish_rapport", Priority: 1},
		{GoalName: "gather_background", Priority: 2},
		{GoalName: "understand_motivations", Priority: 3},
		{GoalName: "discuss_skills", Priority: 4},
		{GoalName: "plan_next_steps", Priority: 5},
	}
}

func (s *ConversationState) ShouldEndCall() bool {
	for _, goal := range s.Context.GoalsProgress {
		if !goal.Status {
			return false
		}
	}
	return true
}

func (s *ConversationState) GetNextGoal() *string {
	for _, goal := range s.Context.GoalsProgress {
		if !goal.Status {
			return &goal.GoalName
		}
	}
	return nil
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
			Candidate: CandidateInfo{
				EngagementLevel: EngagementLevelMedium,
			},
			Context: CallContext{
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
