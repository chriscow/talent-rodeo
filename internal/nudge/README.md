# Nudge Service

The **nudge** package is a comprehensive email automation service that monitors user states and sends personalized nudge emails at strategic times during the talent acquisition journey. The service helps increase user engagement by sending contextually appropriate emails in "Shirley's" friendly, casual tone.

## Overview

The nudge service is a background processor that runs periodically (configurable interval), identifies eligible users based on their current state and activity patterns, generates personalized email content using either static templates or LLM-generated content, and sends nudge emails while tracking delivery to prevent duplicates and respect rate limits.

### Key Features

- **Multiple Nudge Types**: Support for different nudge scenarios (no resume, no call, five-star matches, follow-up)
- **Safety-First Design**: Comprehensive test mode, dry run capabilities, and email domain restrictions
- **Flexible Email Generation**: Both static HTML templates and LLM-generated content
- **Rate Limiting**: Per-user and per-type limits to prevent spam
- **Real-time Configuration**: Feature flags via Cloudflare KV for dynamic settings updates
- **Comprehensive Testing**: Extensive test coverage with mocks and integration tests
- **Observability**: Structured logging, error reporting to Sentry, and processing statistics

## Architecture

### Core Components

The nudge service follows a clean, dependency-injected architecture:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Processor     │───▶│ EmailGenerator   │───▶│  EmailSender    │
│                 │    │                  │    │                 │
│ - Main loop     │    │ - Content gen    │    │ - SMTP delivery │
│ - User logic    │    │ - Template/LLM   │    │ - Error handling│
│ - Rate limiting │    │ - Validation     │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                        │
         ▼                        ▼
┌─────────────────┐    ┌──────────────────┐
│ TemplateReader  │    │ PromptProcessor  │
│                 │    │                  │
│ - Asset loading │    │ - LLM integration│
│ - File reading  │    │ - Prompt gen     │
└─────────────────┘    └──────────────────┘
```

### Dependency Injection

All components use interfaces for testability and flexibility:

- **`TemplateReader`**: Abstracts template file access (asset files, file system)
- **`PromptProcessor`**: Handles LLM interaction and prompt generation
- **`EmailSender`**: Abstracts email delivery (SMTP, mock, etc.)
- **`EmailContentGenerator`**: Business logic for email generation and validation

## Nudge Types

The service supports four nudge types, prioritized in order:

### 1. No Resume Uploaded (`NudgeTypeNoResume`) - Priority 1
- **Trigger**: User signed in but status is `UserStatusPendingResumeUpload`
- **Timing**: Configurable delay (default: 2 days after last access)
- **Content**: Friendly encouragement to upload resume
- **Template**: `emails/shirely-no-resume.html`
- **Prompt**: `prompts/shirely-nudge-no-resume.txt`

### 2. No Call Scheduled (`NudgeTypeNoCall`) - Priority 2
- **Trigger**: User uploaded resume but status is `UserStatusResumeUploaded`
- **Timing**: Configurable delay (default: 3 days after last access)
- **Content**: Gentle reminder to schedule consultation call
- **Template**: `emails/shirely-no-call.html`
- **Prompt**: `prompts/shirely-nudge-no-call.txt`

### 3. Five Star Matches (`NudgeTypeFiveStarMatches`) - Priority 3
- **Trigger**: User has new 5-star job matches but hasn't accessed dashboard recently
- **Timing**: Configurable delay (default: 2 days after last access)
- **Content**: Notification about high-quality job matches with job details
- **Template**: `emails/shirely-five-star-matches.html`
- **Prompt**: `prompts/shirely-nudge-five-star-matches.txt`
- **Special Logic**: Tracks which jobs have been included in nudges to avoid duplicates

### 4. Follow Up (`NudgeTypeFollowUp`) - Priority 4
- **Trigger**: Custom follow-up scenarios (extensible)
- **Content**: Varies based on context
- **Template**: `emails/shirely-follow-up-questions.html`
- **Prompt**: `prompts/shirely-follow-up-questions.txt`

## Configuration

### Processor Settings

The `NudgeProcessorSettings` struct provides comprehensive configuration:

```go
type NudgeProcessorSettings struct {
    // Core Operation
    Enabled             bool          // Master enable/disable switch
    Interval            time.Duration // Processing cycle interval (default: 1h)
    Paused              bool          // Temporary pause without stopping
    
    // Operating Hours (UTC)
    StartHour           int           // Operating window start (0-23, 0=24/7)
    EndHour             int           // Operating window end (0-23, 0=24/7)
    
    // Rate Limiting
    MaxEmailsPerRun     int           // Emails per cycle (default: 3 for safety)
    WeeklyRateLimit     time.Duration // Min time between nudges (default: 7 days)
    
    // Timing Controls
    NoResumeDelayDays   int           // Days before no-resume nudge (default: 2)
    NoCallDelayDays     int           // Days before no-call nudge (default: 3)
    MatchAlertDelayDays int           // Days before match alert (default: 2)
    
    // Per-Type Limits
    MaxNudgesPerType    MaxNudgesPerType // Individual limits per nudge type
    
    // Activity-Based Limiting
    LimitOnRecentActivityPerType LimitOnRecentActivitySettings
    
    // Safety Controls
    DryRun              bool          // Log only, don't send emails
    TestModeEnabled     bool          // Enable test restrictions
    AllowedTestEmails   []string      // Whitelist for test mode
    RequireTestDomain   bool          // Restrict to test domains
    TestDomains         []string      // Allowed test domains
    MaxTestEmailsTotal  int           // Hard limit for test sessions
}
```

### Feature Flags

Settings are stored in Cloudflare KV under key `nudge-processor` for dynamic configuration without deployments.

### Default Settings

The service ships with conservative defaults optimized for safe rollout:

```go
var DefaultNudgeSettings = NudgeProcessorSettings{
    Enabled:             true,
    Interval:            1 * time.Hour,  // Slow processing initially
    MaxEmailsPerRun:     3,              // Very conservative limit
    TestModeEnabled:     true,           // Safety first
    AllowedTestEmails:   []string{"madcow@gmail.com", ...}, // Team emails only
    MaxTestEmailsTotal:  10,             // Hard safety cap
    // ... other safe defaults
}
```

## Email Generation

### Dual Generation Strategy

The service supports two email generation methods:

1. **Static Templates**: Pre-designed HTML templates with Go template syntax
2. **LLM Generation**: Dynamic content using AI with structured prompts

### Template-Based Generation

For nudge types with defined template paths:

```go
func (nt NudgeType) EmailTemplatePath() string {
    switch nt {
    case NudgeTypeNoResume:
        return "emails/shirely-no-resume.html"
    // ...
    }
}
```

Templates must define three sections:
- `{{define "subject"}}` - Email subject line
- `{{define "plainBody"}}` - Plain text body
- `{{define "htmlBody"}}` - HTML body

### LLM-Based Generation

For dynamic content generation:

1. Load prompt template from `prompts/shirely-nudge-*.txt`
2. Generate personalized prompt using `tools.GeneratePrompt()`
3. Call DeepSeek LLM with `tools.Ask()` using max 2048 tokens
4. Parse structured response with XML-like tags:
   ```xml
   <email_content>
     <subject>Your Subject Here</subject>
     <body>Your email body here</body>
   </email_content>
   ```

### Email Data Structure

Both generation methods use the same data structure:

```go
type NudgeEmailData struct {
    User            *internal.User
    NudgeType       string
    FirstName       string
    LastName        string
    FullName        string
    LastAccessed    time.Time
    LastStateChange time.Time
    
    // Five-star matches specific
    Matches         []FiveStarMatchData
}
```

### Content Validation

All generated emails undergo validation:
- Subject line presence and length (max 200 chars)
- Body content presence (plain or HTML)
- Body length limits (max 5000 chars)
- Required branding ("Shirley", "Talent Rodeo")

## User Eligibility Logic

### Core Eligibility Criteria

For each nudge type, users must meet:

1. **Status Match**: User state matches nudge type requirements
2. **Timing**: Sufficient time elapsed since trigger event
3. **Rate Limiting**: Not sent recently (weekly rate limit)
4. **Type Limits**: Haven't exceeded per-type limits
5. **Subscription**: Active subscription (not expired)
6. **Safety**: Pass test mode restrictions if enabled

### Detailed Logic

#### No Resume Nudge
```go
// Status check
user.State.Status == UserStatusPendingResumeUpload

// Activity check (if enabled)
if settings.LimitOnRecentActivityPerType.NoResume {
    user.LastAccessedAt.Before(cutoffTime) // Where cutoffTime = now - NoResumeDelayDays
}

// Rate limiting
if user.LastNudgeEmailSent != nil {
    time.Since(*user.LastNudgeEmailSent) >= settings.WeeklyRateLimit
}

// Type limits
user.NudgeTypeCount["no_resume"] < settings.MaxNudgesPerType.NoResume
```

#### Five Star Matches (Special Logic)
This nudge type includes additional checks:

1. Query `user_jobs` subcollection for jobs with `Score >= 4.5` created within 90 days
2. Filter out jobs already included in previous nudges (`FiveStarNudgeSentAt != nil`)
3. Fetch job details for remaining matches
4. Only send if new matches exist
5. Update job records with nudge timestamp to prevent re-sending

### Priority Processing

When users qualify for multiple nudges, they're sent in priority order:
1. No Resume (highest priority)
2. No Call
3. Five Star Matches
4. Follow Up (lowest priority)

## Database Operations

### User Tracking Fields

The service tracks nudge activity using User struct fields:

```go
type User struct {
    // Nudge tracking
    LastNudgeEmailSent    *time.Time            `firestore:"last_nudge_email_sent,omitempty"`
    LastNudgeEmailType    *string               `firestore:"last_nudge_email_type,omitempty"`
    NudgeEmailsSentCount  int                   `firestore:"nudge_emails_sent_count"`
    NudgeTypeCount        map[string]int        `firestore:"nudge_type_count,omitempty"`
    
    // Existing fields...
}
```

### Atomic Transactions

Email sending and database updates are atomic:

```go
func (p *Processor) sendNudgeWithTracking(ctx context.Context, user *User, nudgeType NudgeType, additionalData map[string]any) bool {
    return p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        // 1. Generate and send email
        // 2. Update user tracking fields
        // 3. Update job-specific tracking (for five-star matches)
        return nil
    })
}
```

### Key Database Functions

```go
// Get users eligible for specific nudge type
func GetNudgeEligibleUsers(ctx context.Context, logger *slog.Logger, fs *FirestoreClient, nudgeType NudgeType, settings NudgeProcessorSettings) ([]*User, error)

// Update user nudge tracking within transaction
func UpdateUserNudgeTrackingTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, userID string, nudgeType string) error

// Get processing statistics
func GetNudgeStats(ctx context.Context, fs *FirestoreClient) (map[string]int, error)
```

## Safety Features

### Test Mode Protection

When `TestModeEnabled` is true:

1. **Email Whitelist**: Only send to addresses in `AllowedTestEmails`
2. **Domain Restriction**: If `RequireTestDomain` is true, only send to `TestDomains`
3. **Session Limits**: Hard cap via `MaxTestEmailsTotal`
4. **Enhanced Logging**: Additional safety checks and warnings

### Dry Run Mode

When `DryRun` is true:
- Process all logic normally
- Generate email content
- Log what would be sent
- Skip actual email delivery
- Still update processing statistics

### Operating Hours

Configure time windows for email sending:
- `StartHour` and `EndHour` define UTC operating window
- If both are 0, operates 24/7
- Supports overnight windows (e.g., StartHour=22, EndHour=6)

### Error Handling

Comprehensive error handling with Sentry integration:

```go
func (p *Processor) handleError(ctx context.Context, err error, operation string, userID string) {
    internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
        "operation": operation,
        "userID":    userID,
        "component": "nudge_processor",
    })
    p.logger.Error("nudge processor error", "operation", operation, "user_id", userID, "error", err)
}
```

## Usage

### Basic Setup

```go
import (
    "talent-rodeo/internal/nudge"
    "talent-rodeo/internal/smtp"
)

// Create processor with default implementations
processor := nudge.NewProcessorWithDefaults(
    firestoreClient,
    smtpConfig,
    logger,
)

// Start the processor
err := processor.Start(ctx)
if err != nil {
    log.Fatal("Failed to start nudge processor:", err)
}

// Graceful shutdown
defer processor.Stop()
```

### Custom Implementation

```go
// Custom dependencies
templateReader := nudge.NewAssetTemplateReader()
promptProcessor := nudge.NewLLMPromptProcessor()
emailGenerator := nudge.NewEmailGenerator(templateReader, promptProcessor)
emailSender := nudge.NewSMTPEmailSender(smtpConfig, logger)

// Create processor
processor := nudge.NewProcessor(
    firestoreClient,
    emailGenerator,
    emailSender,
    logger,
)

// Update settings
processor.UpdateSettings(customSettings)
```

### Dynamic Configuration

```go
// Refresh settings from Cloudflare KV
err := processor.RefreshSettings()
if err != nil {
    logger.Error("Failed to refresh nudge settings", "error", err)
}

// Get current statistics
stats, err := processor.GetStats(ctx)
if err != nil {
    logger.Error("Failed to get nudge stats", "error", err)
}
```

## Testing

### Test Structure

The package includes comprehensive tests:

- **Unit Tests**: Component-level testing with mocks
- **Integration Tests**: End-to-end testing with Firestore emulator
- **Business Logic Tests**: Complex eligibility logic validation
- **Safety Tests**: Test mode and dry run validation

### Mock Implementations

```go
// Create test processor with mocks
func createTestProcessor(logger *slog.Logger) *Processor {
    mockTemplateReader := NewMockTemplateReader()
    mockPromptProcessor := NewMockPromptProcessor()
    mockEmailGenerator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)
    mockEmailSender := NewMockEmailSender()
    return NewProcessor(nil, mockEmailGenerator, mockEmailSender, logger)
}
```

### Test Configuration

```go
var TestSafeNudgeSettings = NudgeProcessorSettings{
    Enabled:             true,
    Interval:            30 * time.Second,  // Fast for testing
    MaxEmailsPerRun:     2,                 // Very limited
    DryRun:              true,             // Safe by default
    TestModeEnabled:     true,
    AllowedTestEmails:   []string{"test@talentrodeo.com"},
    RequireTestDomain:   true,
    TestDomains:         []string{"test.talentrodeo.com", "example.org"},
    MaxTestEmailsTotal:  5,
}
```

## Extending to Other Nudge Types

### Adding a New Nudge Type

1. **Define the Type**:
   ```go
   const NudgeTypeNewFeature NudgeType = "new_feature"
   ```

2. **Update Priority**:
   ```go
   func (nt NudgeType) Priority() int {
       switch nt {
       case NudgeTypeNewFeature:
           return 5  // Set appropriate priority
       // ...
       }
   }
   ```

3. **Add Template Paths**:
   ```go
   func (nt NudgeType) EmailTemplatePath() string {
       switch nt {
       case NudgeTypeNewFeature:
           return "emails/shirely-new-feature.html"
       // ...
       }
   }
   
   func (nt NudgeType) PromptTemplatePath() string {
       switch nt {
       case NudgeTypeNewFeature:
           return "prompts/shirely-nudge-new-feature.txt"
       // ...
       }
   }
   ```

4. **Update Configuration**:
   ```go
   type MaxNudgesPerType struct {
       // Existing fields...
       NewFeature        int
   }
   
   type LimitOnRecentActivitySettings struct {
       // Existing fields...
       NewFeature        bool
   }
   ```

5. **Add Eligibility Logic**:
   ```go
   func isUserEligibleForNudge(user *internal.User, nudgeType NudgeType, cutoffTime time.Time, settings NudgeProcessorSettings) (bool, string) {
       // Add case for NudgeTypeNewFeature
       switch nudgeType {
       case NudgeTypeNewFeature:
           // Custom eligibility logic
           return checkNewFeatureEligibility(user, cutoffTime, settings)
       // ...
       }
   }
   ```

6. **Create Templates**:
   - Create `assets/emails/shirely-new-feature.html`
   - Create `assets/prompts/shirely-nudge-new-feature.txt`

7. **Add to Processing**:
   ```go
   nudgeTypes := []NudgeType{
       NudgeTypeNoResume,
       NudgeTypeNoCall,
       NudgeTypeFiveStarMatches,
       NudgeTypeNewFeature,  // Add to processing list
   }
   ```

8. **Update Tests**:
   - Add test cases for the new nudge type
   - Update mock implementations if needed

### Custom Data Requirements

For nudge types requiring additional data:

1. **Extend Email Data**:
   ```go
   type NudgeEmailData struct {
       // Existing fields...
       
       // New feature specific
       CustomData *CustomDataStruct `json:"custom_data,omitempty"`
   }
   ```

2. **Update Data Builder**:
   ```go
   func (eg *EmailGenerator) BuildEmailData(user *internal.User, nudgeType NudgeType, additionalData map[string]any) *NudgeEmailData {
       data := &NudgeEmailData{
           // Standard fields...
       }
       
       if nudgeType == NudgeTypeNewFeature {
           if customData, ok := additionalData["custom"].(CustomDataStruct); ok {
               data.CustomData = &customData
           }
       }
       
       return data
   }
   ```

3. **Update Processing Logic**:
   ```go
   func (p *Processor) processUser(ctx context.Context, user *internal.User, nudgeType NudgeType) (sent bool, err error) {
       var additionalData map[string]any
       
       if nudgeType == NudgeTypeNewFeature {
           // Gather custom data
           customData, err := gatherCustomData(ctx, p.fs, user.ID)
           if err != nil {
               return false, err
           }
           additionalData = map[string]any{"custom": customData}
       }
       
       // Continue with sending...
   }
   ```

## Dependencies

### Core Dependencies
- **Firestore**: Primary database for user data and nudge tracking
- **SMTP**: Email delivery via configurable SMTP provider
- **LLM Integration**: `github.com/chriscow/minds/tools` for AI-generated content
- **Sentry**: Error reporting and monitoring
- **Cloudflare KV**: Feature flags and dynamic configuration

### Internal Dependencies
- `internal.FirestoreClient`: Database operations
- `internal.User`: User data structure
- `internal.smtp`: SMTP configuration and sending
- `assets`: Embedded template files

### Testing Dependencies
- Firestore emulator for integration tests
- Mock implementations for unit tests
- Standard library testing framework

## Observability

### Logging

Structured logging with contextual information:

```go
p.logger.Info("completed nudge processing cycle",
    "duration", stats.Duration,
    "total_emails", stats.TotalEmailsSent,
    "total_errors", stats.TotalErrors,
    "no_resume", stats.TypeStats[NudgeTypeNoResume].EmailsSent,
    "no_call", stats.TypeStats[NudgeTypeNoCall].EmailsSent,
    "five_star", stats.TypeStats[NudgeTypeFiveStarMatches].EmailsSent,
)
```

### Statistics

Processing statistics available via `GetStats()`:

```go
type ProcessingStats struct {
    StartTime       time.Time
    EndTime         time.Time
    Duration        time.Duration
    TotalEmailsSent int
    TotalErrors     int
    TypeStats       map[NudgeType]*TypeStats
}

type TypeStats struct {
    EligibleUsers int
    EmailsSent    int
    Errors        int
}
```

### Error Reporting

All errors are reported to Sentry with contextual tags:
- `operation`: Specific operation that failed
- `userID`: User being processed
- `component`: Always "nudge_processor"

## Safe Launch Strategy

The package includes a comprehensive safe launch plan (`SAFE_LAUNCH_PLAN.md`) with four phases:

1. **Phase 1: Silent Mode** - Production monitoring without email sending
2. **Phase 2: Internal Testing** - Controlled testing with team accounts
3. **Phase 3: Limited Beta** - Small percentage of real users
4. **Phase 4: Full Launch** - Gradual rollout to all users

This phased approach ensures safety and allows for monitoring at each stage.

## Performance Considerations

### Batch Processing
- Users are processed in configurable batches (`MaxEmailsPerRun`)
- Database queries are optimized for large user sets
- Processing cycles are spread over time via `Interval` setting

### Resource Management
- LLM calls are limited by token count (2048 max)
- SMTP connections are created per email (stateless)
- Firestore transactions are kept minimal and atomic

### Monitoring
- Processing duration tracking
- Email delivery success/failure rates
- Error frequency and types
- User engagement metrics post-nudge

The nudge service provides a robust, safe, and extensible foundation for automated user engagement emails while maintaining strict safety controls and comprehensive observability.