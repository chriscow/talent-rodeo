package nudge

import (
	"context"
	"talent-rodeo/internal"
	"time"
)

// NudgeType represents the type of nudge email
type NudgeType string

const (
	NudgeTypeNoResume        NudgeType = "no_resume"
	NudgeTypeNoCall          NudgeType = "no_call"
	NudgeTypeFiveStarMatches NudgeType = "five_star_matches"
	NudgeTypeFollowUp        NudgeType = "follow_up"
)

// NudgeProcessorSettings configures the nudge processor behavior
// Each field controls a specific aspect of nudge email processing, safety, or rollout.
type NudgeProcessorSettings struct {
	// Enabled is the master switch to turn the nudge processor on or off.
	// If false, the processor will not run or process any users.
	Enabled bool

	// Interval is the time between processing cycles (e.g., 5m, 1h).
	// Controls how often the processor wakes up to check for eligible users and send nudges.
	Interval time.Duration

	// Paused temporarily halts processing without disabling the processor entirely.
	// Useful for maintenance or emergency stops. If true, cycles are skipped but the processor remains running.
	Paused bool

	// StartHour and EndHour define the UTC window (hours, 0-23) during which nudges may be sent.
	// If both are 0, the processor operates 24/7. If StartHour > EndHour, the window crosses midnight.
	StartHour int
	EndHour   int

	// MaxEmailsPerRun limits the number of emails sent in a single processing cycle.
	// Prevents accidental mass-emailing and helps throttle rollout.
	MaxEmailsPerRun int

	// NoResumeDelayDays is the minimum number of days to wait after a user is eligible for a "no resume" nudge.
	NoResumeDelayDays int
	// NoCallDelayDays is the minimum number of days to wait after a user is eligible for a "no call" nudge.
	NoCallDelayDays int
	// MatchAlertDelayDays is the minimum number of days to wait before sending a "five star matches" nudge.
	MatchAlertDelayDays int

	// MaxNudgesPerType defines limits for each specific nudge type. This allows for more granular
	// control over the user journey, ensuring that early-stage nudges don't exhaust the budget
	// for later, high-value nudges. The total number of nudges is still capped by MaxNudgesPerUser.
	MaxNudgesPerType MaxNudgesPerType

	// WeeklyRateLimit is the minimum duration between nudges to the same user (e.g., 7*24*time.Hour for weekly).
	WeeklyRateLimit time.Duration

	// LimitOnRecentActivityPerType, if true for a given nudge type, prevents sending that nudge to a user who has recently accessed the application.
	LimitOnRecentActivityPerType LimitOnRecentActivitySettings

	// --- Safety controls for testing and safe rollout ---

	// DryRun, if true, logs what would be sent but does not actually send any emails.
	// Use for production monitoring, silent rollout, or integration testing.
	DryRun bool

	// TestModeEnabled enables strict safety checks for integration or staging environments.
	// When true, the processor will only send emails to explicitly allowed addresses or domains,
	// and will enforce additional limits to prevent accidental real-user contact.
	// Use this for end-to-end tests or when running in a non-production environment.
	TestModeEnabled bool

	// AllowedTestEmails is a whitelist of email addresses that are allowed to receive nudges in test mode.
	// If non-empty, only these addresses will be sent emails when TestModeEnabled is true.
	AllowedTestEmails []string

	// RequireTestDomain, if true, restricts email sending to addresses with domains listed in TestDomains.
	// Use to ensure only test accounts or sandboxes are contacted during testing.
	RequireTestDomain bool

	// TestDomains is a list of allowed email domains for testing (e.g., ["test.com", "example.org"]).
	// Only emails with these domains will be sent when RequireTestDomain is true and/or TestModeEnabled is true.
	TestDomains []string

	// MaxTestEmailsTotal is the absolute maximum number of emails that can be sent during a test session.
	// Prevents runaway email loops or accidental spamming during integration tests or dry runs.
	MaxTestEmailsTotal int
}

// DefaultNudgeSettings provides default configuration

//
// Initial defaults for production monitoring / safe rollout
//
// 	## Phase 1: Silent Mode (Production Monitoring)

// **Goal**: Verify the user eligibility logic and processing cycle in production without sending any emails.

// **Duration**: 1-2 days

// ### Actions:
// 1.  **Deploy Code**: Deploy the nudge processor code to production.
// 2.  **Configure Feature Flags**:
//   - `enabled`: `true` (to run the processor)
//   - `dry_run`: `true` (to prevent email sending)
//   - `test_mode_enabled`: `false`
//   - `paused`: `false`
//
// 3.  **Set a high `Interval`**: Start with a longer processing interval (e.g., `1h`) to reduce noise.
var DefaultNudgeSettings = NudgeProcessorSettings{
	Enabled:             true,
	Interval:            1 * time.Hour,
	Paused:              false,
	StartHour:           0, // 24/7 operation
	EndHour:             0,
	MaxEmailsPerRun:     3, // to start for soft launch
	NoResumeDelayDays:   2,
	NoCallDelayDays:     3,
	MatchAlertDelayDays: 2,
	WeeklyRateLimit:     7 * 24 * time.Hour,
	LimitOnRecentActivityPerType: LimitOnRecentActivitySettings{
		NoResume:        false,
		NoCall:          false,
		FiveStarMatches: false,
		FollowUp:        false,
	},

	MaxNudgesPerType: MaxNudgesPerType{
		NoResume:        2, // Allow up to 2 reminders for a resume
		NoCall:          1, // 1 reminder for a call should be enough
		FiveStarMatches: 5, // Allow a few of these high-value emails
		FollowUp:        1, // 1 follow-up
	},

	DryRun:             false,
	TestModeEnabled:    true, // only send to test emails
	AllowedTestEmails:  []string{"madcow@gmail.com", "mindemann@gmail.com", "wolfgangmeyers@gmail.com", "neal.s.modi@gmail.com"},
	RequireTestDomain:  false,
	TestDomains:        []string{"test.talentrodeo.com", "example.org"},
	MaxTestEmailsTotal: 10,
}

// TestSafeNudgeSettings provides ultra-safe settings for integration testing
var TestSafeNudgeSettings = NudgeProcessorSettings{
	Enabled:             true,
	Interval:            30 * time.Second, // Faster for testing
	Paused:              false,
	StartHour:           0, // 24/7 operation
	EndHour:             0,
	MaxEmailsPerRun:     2,         // Very limited
	NoResumeDelayDays:   0,         // Immediate for testing
	NoCallDelayDays:     0,         // Immediate for testing
	MatchAlertDelayDays: 0,         // Immediate for testing
	WeeklyRateLimit:     time.Hour, // Short for testing
	LimitOnRecentActivityPerType: LimitOnRecentActivitySettings{
		NoResume:        true,
		NoCall:          true,
		FiveStarMatches: true,
		FollowUp:        true,
	},

	MaxNudgesPerType: MaxNudgesPerType{
		NoResume:        1,
		NoCall:          1,
		FiveStarMatches: 1,
		FollowUp:        1,
	},

	// Testing safety controls
	DryRun:             true, // Don't actually send by default
	TestModeEnabled:    true,
	AllowedTestEmails:  []string{"test@talentrodeo.com", "chris@test.com"},
	RequireTestDomain:  true,
	TestDomains:        []string{"test.talentrodeo.com", "example.org"},
	MaxTestEmailsTotal: 5,
}

// NudgeEmailData contains data passed to email templates
type NudgeEmailData struct {
	User         *internal.User
	NudgeType    string
	LastAccessed time.Time

	// Personalization fields
	FirstName string
	LastName  string
	FullName  string

	LastStateChange time.Time

	// Scenario-specific data
	JobMatchCount *int
	TopJobScore   *float64

	// For NudgeTypeFiveStarMatches
	Matches []FiveStarMatchData
}

// GeneratedEmail holds the content of a generated email.
type GeneratedEmail struct {
	Subject   string
	HTMLBody  string
	PlainBody string
}

// FiveStarMatchData holds the data for a single job match in the 5-star email.
type FiveStarMatchData struct {
	Title    string
	Summary  string
	Analysis string
}

// MaxNudgesPerType defines the maximum number of nudges a user can receive for each nudge type.
type MaxNudgesPerType struct {
	NoResume        int
	NoCall          int
	FiveStarMatches int
	FollowUp        int
}

// LimitOnRecentActivitySettings defines whether to limit nudges based on recent activity for each nudge type.
type LimitOnRecentActivitySettings struct {
	NoResume        bool
	NoCall          bool
	FiveStarMatches bool
	FollowUp        bool
}

// Interfaces for dependency injection and testing

// TemplateReader reads template files (abstracts file system access)
type TemplateReader interface {
	ReadTemplate(nudgeType NudgeType) ([]byte, error)
}

// PromptProcessor handles prompt generation and LLM interaction
type PromptProcessor interface {
	GeneratePrompt(template []byte, data *NudgeEmailData) (string, error)
	ProcessPrompt(ctx context.Context, prompt string) (string, error)
}

// EmailSender handles actual email delivery (abstracts SMTP)
type EmailSender interface {
	SendEmail(ctx context.Context, recipient string, email *GeneratedEmail, nudgeType NudgeType) error
}

// EmailContentGenerator handles the business logic of email generation
type EmailContentGenerator interface {
	BuildEmailData(user *internal.User, nudgeType NudgeType, additionalData map[string]any) *NudgeEmailData
	GenerateEmailContent(ctx context.Context, user *internal.User, nudgeType NudgeType, emailData *NudgeEmailData) (*GeneratedEmail, error)
	ValidateEmail(email *GeneratedEmail) error
}

// String returns the string representation of NudgeType
func (nt NudgeType) String() string {
	return string(nt)
}

// IsValid checks if the nudge type is valid
func (nt NudgeType) IsValid() bool {
	switch nt {
	case NudgeTypeNoResume, NudgeTypeNoCall, NudgeTypeFiveStarMatches, NudgeTypeFollowUp:
		return true
	default:
		return false
	}
}

// Priority returns the priority order for nudge types (lower number = higher priority)
func (nt NudgeType) Priority() int {
	switch nt {
	case NudgeTypeNoResume:
		return 1
	case NudgeTypeNoCall:
		return 2
	case NudgeTypeFiveStarMatches:
		return 3
	case NudgeTypeFollowUp:
		return 4
	default:
		return 999
	}
}

// PromptTemplateFilename returns the filename for the prompt template
func (nt NudgeType) PromptTemplateFilename() string {
	switch nt {
	case NudgeTypeNoResume:
		return "shirely-nudge-no-resume.txt"
	case NudgeTypeNoCall:
		return "shirely-nudge-no-call.txt"
	case NudgeTypeFiveStarMatches:
		return "shirely-nudge-five-star-matches.txt"
	case NudgeTypeFollowUp:
		return "shirely-follow-up-questions.txt"
	default:
		return ""
	}
}

// TemplateFilename returns the filename for the Go template.
// If it returns an empty string, it implies an LLM should be used.
func (nt NudgeType) TemplateFilename() string {
	switch nt {
	case NudgeTypeNoResume:
		return "shirely-no-resume.html"
	case NudgeTypeNoCall:
		return "shirely-no-call.html"
	case NudgeTypeFiveStarMatches:
		return "shirely-five-star-matches.html"
	default:
		return ""
	}
}

// NudgeSettings defines the settings for a specific nudge type
type NudgeSettings struct {
	Enabled             bool
	Interval            time.Duration
	Paused              bool
	StartHour           int
	EndHour             int
	MaxEmailsPerRun     int
	NoResumeDelayDays   int
	NoCallDelayDays     int
	MatchAlertDelayDays int
	MaxNudgesPerUser    int
	MaxNudgesPerType    MaxNudgesPerType
	WeeklyRateLimit     time.Duration
	DryRun              bool
	TestModeEnabled     bool
	AllowedTestEmails   []string
	RequireTestDomain   bool
	TestDomains         []string
	MaxTestEmailsTotal  int
}
