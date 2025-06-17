# Nudge Service Specification

## Overview

The **nudge** package will be a periodic service that monitors user states and appointments to send automated, personalized nudge emails at strategic times during the user journey. This service will help increase user engagement and guide users through the talent acquisition process by sending contextually appropriate emails in "Shirley's" friendly, casual tone.

## Service Architecture

### Base Structure
The service will be modeled after the existing `internal/subscription/subscription_processor.go` with the following components:

- **Processor**: Main service struct with configurable settings
- **Settings**: Configurable timing intervals, enabled/disabled state, and operational windows
- **Start/Stop**: Graceful startup and shutdown with context cancellation
- **Process Method**: Core logic that runs every 5 minutes to check user states

### Key Dependencies
- **Firestore Client**: For user and appointment data retrieval
- **LLM Integration**: Using the `github.com/chriscow/minds/tools` library for email generation
- **SMTP Mailer**: For sending emails (similar to transcript processor)
- **Logger**: For monitoring and debugging
- **Cloudflare KV**: For feature flags and configuration (optional)

## Email Generation Pattern

### Template Structure
Each nudge scenario will have:
1. **Email Template** (`assets/emails/shirely-*.html`) - Standard Go template structure with subject, plainBody, and htmlBody sections
2. **Prompt Template** (`assets/prompts/shirely-nudge-*.txt`) - LLM prompt for generating personalized content

### Generation Flow
Following the pattern from `transcript_processor.go:GenerateRecapEmail()`:

1. Load prompt template from assets
2. Generate prompt with user-specific data using `tools.GeneratePrompt()`
3. Call LLM with `tools.Ask()` using DeepSeekChat model
4. Extract email content from LLM response
5. Send email using SMTP mailer with the corresponding HTML template

## Nudge Scenarios

### 1. No Resume Uploaded (Priority: High)
**Trigger**: User signed in but `UserStatus == UserStatusPendingResumeUpload` and `LastAccessedAt > 2 days ago`
**Template**: `shirely-no-resume.html`
**Prompt**: `shirely-nudge-no-resume.txt`
**Content**: Friendly encouragement to upload resume, highlighting the benefits

### 2. No Call Scheduled (Priority: High)
**Trigger**: User uploaded resume but `UserStatus == UserStatusResumeUploaded` and `LastAccessedAt > 3 days ago`
**Template**: `shirely-no-call.html`  
**Prompt**: `shirely-nudge-no-call.txt`
**Content**: Gentle reminder to schedule their consultation call

### 3. Five Star Matches Available (Priority: High)
**Trigger**: User has new 5-star job matches but hasn't accessed dashboard in 2 days
**Template**: `shirely-five-star-matches.html`
**Prompt**: `shirely-nudge-five-star-matches.txt`
**Content**: Notification about high-quality job matches waiting for review
**Implementation Note**: Query `user_jobs` subcollection for jobs with `Score >= 4.5` created within last 7 days

**Note**: Follow-up questions functionality is handled separately by the `internal/feedback` package as a sophisticated engagement system.

## Data Tracking

### Email History
To prevent duplicate emails, the service will track sent emails using simple User struct fields:

```go
type User struct {
    // ... existing fields ...
    
    // Nudge Email Tracking
    LastNudgeEmailSent    *time.Time `firestore:"last_nudge_email_sent,omitempty"`
    LastNudgeEmailType    *string    `firestore:"last_nudge_email_type,omitempty"`
    NudgeEmailsSentCount  int        `firestore:"nudge_emails_sent_count"`
}
```

**Email Types**:
- `"no_resume"` - No resume uploaded nudge
- `"no_call"` - No call scheduled nudge  
- `"five_star_matches"` - Five star matches available nudge

### Rate Limiting
- Maximum 1 nudge email per user per week (check `time.Since(*LastNudgeEmailSent) >= 7*24*time.Hour`)
- Maximum 3 total nudge emails per user lifetime (`NudgeEmailsSentCount < 3`)
- Respect user preferences for email frequency (future enhancement)
- Skip users who have unsubscribed from communications
- Skip users with `SubscriptionEnd` in the past (expired subscriptions)

## Configuration

### Processor Settings
```go
type NudgeProcessorSettings struct {
    Enabled             bool          `json:"enabled"`              // Master enable/disable switch
    Interval            time.Duration `json:"interval"`             // Processing interval (default: 5 minutes)
    Paused              bool          `json:"paused"`               // Temporary pause processing
    StartHour           int           `json:"start_hour"`           // Operating window start (UTC, default: 0)
    EndHour             int           `json:"end_hour"`             // Operating window end (UTC, default: 0)
    MaxEmailsPerRun     int           `json:"max_emails_per_run"`   // Limit emails sent per cycle (default: 50)
    NoResumeDelayDays   int           `json:"no_resume_delay_days"` // Days before no-resume email (default: 2)
    NoCallDelayDays     int           `json:"no_call_delay_days"`   // Days before no-call email (default: 3)
    MatchAlertDelayDays int           `json:"match_alert_delay_days"` // Days before match alert email (default: 2)
    MaxNudgesPerUser    int           `json:"max_nudges_per_user"`  // Max lifetime nudges per user (default: 3)
    WeeklyRateLimit     time.Duration `json:"weekly_rate_limit"`    // Min time between nudges (default: 7*24*time.Hour)
}

var DefaultNudgeSettings = NudgeProcessorSettings{
    Enabled:             true,
    Interval:            5 * time.Minute,
    Paused:              false,
    StartHour:           0,  // 24/7 operation
    EndHour:             0,
    MaxEmailsPerRun:     50,
    NoResumeDelayDays:   2,
    NoCallDelayDays:     3,
    MatchAlertDelayDays: 2,
    MaxNudgesPerUser:    3,
    WeeklyRateLimit:     7 * 24 * time.Hour,
}
```

### Feature Flags
Store settings in Cloudflare KV under key `nudge-processor` for dynamic configuration without deployments.

## Technical Implementation

### Database Queries
Create `internal/nudge_users.go` for nudge-specific database operations:

```go
// Get users eligible for nudge emails
func GetNudgeEligibleUsers(ctx context.Context, fs *FirestoreClient, scenario string, settings NudgeProcessorSettings) ([]*User, error)

// Update user nudge tracking fields
func UpdateUserNudgeTracking(ctx context.Context, fs *FirestoreClient, userID string, emailType string) error

// Check if user has five-star matches
func UserHasFiveStarMatches(ctx context.Context, fs *FirestoreClient, userID string, daysSince int) (bool, error)

// Get user's latest job match scores
func GetUserTopJobScores(ctx context.Context, fs *FirestoreClient, userID string, limit int) ([]float64, error)
```

### Error Handling Strategy
```go
// Error handling pattern for nudge processor
func handleNudgeError(ctx context.Context, err error, operation string, userID string) error {
    internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
        "operation": operation,
        "userID":    userID,
        "component": "nudge_processor",
    })
    return fmt.Errorf("nudge operation failed for %s: %w", operation, err)
}

// Usage examples:
// - LLM generation failure → Report to Sentry, skip user, retry next cycle
// - SMTP send failure → Report to Sentry, skip user, do NOT mark as sent
// - Database update failure → Report to Sentry, retry up to 3 times
```

### Email Template Data Structure
```go
type NudgeEmailData struct {
    User           *User    `json:"user"`
    NudgeType      string   `json:"nudge_type"`      // "no_resume", "no_call", "five_star_matches"
    DaysSinceEvent int      `json:"days_since_event"` // Days since last activity
    
    // Personalization fields
    FirstName      string   `json:"first_name"`
    FullName       string   `json:"full_name"`
    
    // Scenario-specific data
    JobMatchCount  *int     `json:"job_match_count,omitempty"`  // For five-star matches
    TopJobScore    *float64 `json:"top_job_score,omitempty"`    // For five-star matches
}
```

## Implementation Files

### Core Service
- `internal/nudge/nudge_processor.go` - Main processor service
- `internal/nudge/email_generator.go` - Email generation logic
- `internal/nudge/types.go` - Data structures and constants

### Database Layer
- `internal/nudge_users.go` - All nudge-related database operations

### Templates
- `assets/emails/shirely-no-resume.html` (existing)
- `assets/emails/shirely-no-call.html` (existing)
- `assets/emails/shirely-five-star-matches.html` (existing)

### Prompts (to be created)
- `assets/prompts/shirely-nudge-no-resume.txt`
- `assets/prompts/shirely-nudge-no-call.txt`
- `assets/prompts/shirely-nudge-five-star-matches.txt`

### Integration Points
- Add nudge processor to main application startup in `cmd/web/main.go`
- Wire SMTP configuration through existing email infrastructure from transcript processor
- Use existing `internal.GetUser()` and `internal.UpdateUser()` functions
- Query `user_jobs` subcollection for job match data
- Respect existing `User.SubscriptionEnd` field for eligibility
- Use existing Sentry integration for error reporting

## Testing Strategy

### Unit Tests
- Test email generation logic with mock LLM responses
- Verify trigger conditions for different user states
- Test rate limiting and duplicate prevention

### Integration Tests
- Use Firestore emulator for database operations
- Mock SMTP calls to prevent actual email sending
- Test end-to-end flow with sample user data

### UI Tests (if applicable)
- Test email unsubscribe functionality
- Verify email content renders correctly across email clients

## Processing Logic

### User Eligibility Criteria
For each processing cycle, check users in priority order:

1. **No Resume Nudge**:
   - `UserStatus == UserStatusPendingResumeUpload`
   - `LastAccessedAt` is null OR `> NoResumeDelayDays ago`
   - `LastNudgeEmailType != "no_resume"` OR `LastNudgeEmailSent` is null OR `> WeeklyRateLimit ago`
   - `NudgeEmailsSentCount < MaxNudgesPerUser`
   - `SubscriptionEnd` is null OR in future

2. **No Call Nudge**:
   - `UserStatus == UserStatusResumeUploaded`
   - `LastAccessedAt > NoCallDelayDays ago`
   - `LastNudgeEmailType != "no_call"` OR `LastNudgeEmailSent` is null OR `> WeeklyRateLimit ago`
   - `NudgeEmailsSentCount < MaxNudgesPerUser`
   - `SubscriptionEnd` is null OR in future

3. **Five Star Matches Nudge**:
   - `UserStatus >= UserStatusCallCompleted`
   - Has job matches with `Score >= 4.5` created within last 7 days
   - `LastAccessedAt > MatchAlertDelayDays ago`
   - `LastNudgeEmailType != "five_star_matches"` OR `LastNudgeEmailSent` is null OR `> WeeklyRateLimit ago`
   - `NudgeEmailsSentCount < MaxNudgesPerUser`
   - `SubscriptionEnd` is null OR in future

### Priority Order
When user qualifies for multiple nudges, send in this order:
1. No Resume (highest priority)
2. No Call 
3. Five Star Matches (lowest priority)

### Transaction Handling
```go
// Critical: Email send + database update must be atomic
func (p *Processor) sendNudgeWithTracking(ctx context.Context, user *User, nudgeType string) error {
    return p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        // 1. Send email first (outside transaction for performance)
        emailData := p.buildEmailData(user, nudgeType)
        if err := p.emailService.Send(ctx, emailData); err != nil {
            return fmt.Errorf("failed to send nudge email: %w", err)
        }
        
        // 2. Update user tracking fields within transaction
        now := time.Now()
        userUpdates := []firestore.Update{
            {Path: "LastNudgeEmailSent", Value: now},
            {Path: "LastNudgeEmailType", Value: nudgeType},
            {Path: "NudgeEmailsSentCount", Value: firestore.Increment(1)},
        }
        
        if err := tx.Update(p.fs.Collection("users").Doc(user.ID), userUpdates); err != nil {
            return fmt.Errorf("failed to update user nudge tracking: %w", err)
        }
        
        return nil
    })
}
```

## Clarified Requirements

### Resolved Technical Decisions
- **Email Tracking**: Use simple User struct fields (implemented above)
- **Error Handling**: Report to Sentry, skip user, retry next cycle (no fallback templates)
- **Batch Size**: Process up to `MaxEmailsPerRun` users per cycle (default: 50)
- **Rate Limiting**: 1 nudge per week, max 3 lifetime nudges per user
- **Priority**: No Resume > No Call > Five Star Matches
- **Job Match Query**: Score >= 4.5 within last 7 days for five-star nudges

### Testing Environment
- Service disabled by default in development (check `NUDGE_ENABLED` env var)
- Use Firestore emulator for database tests
- Mock SMTP calls in tests

### Analytics Integration
- Track nudge email delivery success/failure in existing logging
- Monitor user conversion rates after nudge emails
- Track unsubscribe rates by nudge type

### Call-to-Action Specification
- **No Resume**: "Upload your resume now" → link to resume upload page
- **No Call**: "Schedule your consultation" → link to appointment booking
- **Five Star Matches**: "View your matches" → link to dashboard with filtered results