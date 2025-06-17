# Follow-up Questions Feature Specification

## Overview

The **Follow-up Questions** feature is a sophisticated engagement system that sends personalized questions to users about their job search progress at strategic intervals after their initial call. This feature is implemented as part of the `internal/feedback` package and requires careful tracking to avoid duplicate questions while maintaining meaningful engagement.

## Key Requirements

### Prerequisites
- User must have completed their initial call (`UserStatus >= UserStatusCallCompleted`)
- User must have job matches in their `user_jobs` subcollection 

### Timing Cadence (Relative Timing)
The cadence uses **relative timing** to prevent email blasts and handle system downtime:

- **Day 1**: Send ≥24 hours after call completion (if no Day 1 email sent yet)
- **Day 3**: Send ≥48 hours after Day 1 email was sent
  - Check for email response via `internal/inbox` integration
  - If NO response: Resend Day 1 email
  - If YES response: Send Day 3 follow-up email
- **Day 5**: Send ≥48 hours after Day 3 email was sent
  - Always send marketing email warning about search ending
- **Day 7**: Set `SubscriptionEnd` field, stop all processing for this user

**Important**: Always check `User.SubscriptionEnd` before processing - skip expired users.

### Question Management
Every UserJob in the `user_jobs` subcollection contains a `JobMatchAnalysis` struct with a `FollowUpQuestions` field. These questions were generated when the LLM analyzed the job match and could help determine if the job is a good fit if answered.

**Question Aggregation** (handled by `internal/feedback` package):
- The feedback system aggregates questions from all UserJobs that haven't been processed before
- Uses LLM to normalize, deduplicate, and rank questions by frequency/importance
- Stores aggregated questions in `user_followup_questions` collection
- Sets `User.FollowupQuestionsAggregatedAt` timestamp when complete

**Question Delivery** (handled by the feedback package's followup questions processor):
- Retrieves pre-aggregated questions from `user_followup_questions` collection
- Sends top 3-5 questions via email
- Marks sent questions with `SentToUserAt` timestamp
- Updates `User.FollowupQuestionsSentAt` when email sent successfully

**Error Handling**:
- If email send fails: Report to Sentry using `internal.SentryReportError()` and stop processing
- Failed sends require manual intervention/utility to resend

**Response Tracking**:
- Integrates with `internal/inbox` system to detect email responses
- Response status determines Day 3 email behavior (resend vs. new content)

## Data Structure Changes

### User Struct Additions
```go
type User struct {
    // ... existing fields ...
    
    // Question Management
    FollowupQuestionsAggregatedAt *time.Time // When feedback system last aggregated questions
    
    // Email Tracking (relative timing)
    FollowupDay1SentAt *time.Time // When Day 1 email was sent
    FollowupDay3SentAt *time.Time // When Day 3 email was sent  
    FollowupDay5SentAt *time.Time // When Day 5 marketing email was sent
    
    // Subscription Management
    SubscriptionEnd *time.Time // When user's search subscription expires (Day 7)
}
```

### New Collection: user_followup_questions
```go
type UserFollowupQuestion struct {
    ID                 string    `firestore:"id"`
    UserID             string    `firestore:"user_id"`
    QuestionText       string    `firestore:"question_text"`
    QuestionType       string    `firestore:"question_type"`    // e.g., "interest_level", "application_status", "concerns"
    EstimatedCount     int       `firestore:"estimated_count"`  // LLM's estimate of how many jobs asked this question
    Priority           int       `firestore:"priority"`         // Ranking from aggregation process (updated daily)
    
    // Email Delivery Tracking
    SentToUserAt       *time.Time `firestore:"sent_to_user_at,omitempty"` // When this question was emailed
    EmailType          *string   `firestore:"email_type,omitempty"`      // "day1", "day3_resend", "day3_new"
    
    // Response Tracking (via internal/inbox integration + LLM parsing)
    Answer             *string   `firestore:"answer,omitempty"`           // User's answer extracted by LLM
    ResponseReceivedAt *time.Time `firestore:"response_received_at,omitempty"`
    AnswerEmailID      *string   `firestore:"answer_email_id,omitempty"`  // Reference to inbox email
    
    // Metadata
    CreatedAt          time.Time `firestore:"created_at"`          // When aggregated by feedback system
    UpdatedAt          time.Time `firestore:"updated_at"`          // Last updated by daily aggregation
}
```

## Email Flow Process

### Step-by-Step Flow
1. **User Qualification Check**
   - Verify `UserStatus >= UserStatusCallCompleted`
   - Check `User.SubscriptionEnd` is null or future date (skip if expired)
   - Ensure user has job matches in `user_jobs` subcollection

2. **Day 1 Email** (≥24 hours after call completion)
   - Check if `FollowupDay1SentAt` is null
   - Query `user_followup_questions` for top priority questions where `SentToUserAt` is null
   - Send top 3-5 questions via email
   - On success: Update `User.FollowupDay1SentAt` and mark questions with `SentToUserAt` + `EmailType="day1"`
   - On failure: Report to Sentry and stop processing for this user

3. **Day 3 Email** (≥48 hours after Day 1 sent)
   - Check if `FollowupDay3SentAt` is null
   - Check questions sent on Day 1: count how many have `Answer` field populated (via inbox processing)
   
   **If NO responses received** (all Day 1 questions still have `Answer=null`):
   - Query current top 3-5 priority questions where `Answer` is null (may be different from Day 1 due to daily re-ranking)
   - Resend email with current highest-priority unanswered questions
   - Mark with `EmailType="day3_resend"`
   - **Note**: User may respond to both Day 1 and Day 3 emails - use LLM to merge answers if duplicates exist
   
   **If ANY responses received** (at least one Day 1 question has `Answer` populated):
   - Send acknowledgment + next 3 top-priority questions where `SentToUserAt` is null
   - Mark new questions with `EmailType="day3_new"`
   
   - Update `User.FollowupDay3SentAt` on successful send

4. **Day 5 Email** (≥48 hours after Day 3 sent)
   - Check if `FollowupDay5SentAt` is null
   - Send marketing email warning about search ending
   - Update `User.FollowupDay5SentAt` on successful send

**Note**: `User.SubscriptionEnd` is managed in `internal/appointment.go` when appointments complete to define the follow-up window. The followup service checks this field to determine when to stop processing users.

### Email Types
- **Day 1**: Personalized questions from job match analysis (top 3-5 priority questions)
- **Day 3 (No Response)**: Re-evaluated top questions that still need answers (may be different from Day 1)
- **Day 3 (With Response)**: Acknowledgment + next 3 highest-priority unsent questions
- **Day 5**: Marketing email about search ending, final opportunity messaging

### Email Tracking and Identification
Each follow-up email includes tracking identifiers for response processing:

**Subject Line Format**: `[Talent Rodeo] Follow-up Questions [FAQ-{userID}-{timestamp}]`
- Example: `[Talent Rodeo] Follow-up Questions [FAQ-abc123-1703876400]`
- Timestamp format: Unix timestamp (e.g., 1703876400)
- Users are instructed to keep the subject line intact when replying

**Email Body Identifier**: 
```
Follow-up ID: FAQ-{userID}-{timestamp}
Please keep this ID and subject line when replying so we can process your responses.
```

**Question Identifiers**: Each question in the email includes a unique reference using the Firestore document ID:
```
Q1-{firestoreDocID}: What interests you most about remote work opportunities?
Q2-{firestoreDocID}: How important is company culture in your job search?
```

### User Identification for Email Responses
When processing email responses, identify users using:

1. **Primary Method**: `GetUserByEmail(ctx, fs, senderEmailAddress)` from `internal/user.go`
2. **Fallback Method**: Extract userID from subject line identifier `[FAQ-{userID}-{timestamp}]` if sender email doesn't match any user record
3. **Subject Line Detection**: Use regex pattern `\[FAQ-([^-]+)-(\d+)\]` to extract userID and timestamp
4. **Use Case**: Handles situations where users reply from different email addresses than their account email

### Daily Question Aggregation Process
The `internal/feedback` package runs daily aggregation:
1. Scans all `user_jobs` for new `FollowUpQuestions` since last aggregation
2. Uses LLM to merge, normalize, and re-rank ALL questions (new + existing)
3. Updates `EstimatedCount` field based on how often questions are asked across jobs
4. Updates `Priority` field on existing questions based on new analysis
5. Creates new `UserFollowupQuestion` documents for new questions
6. **Important**: Questions with populated `Answer` fields are excluded from future job analysis since we have the user's response
7. Updates `User.FollowupQuestionsAggregatedAt` timestamp

**Key Point**: Question priorities can change daily as new jobs are added, so "top questions" on Day 3 might be different than Day 1, even for unanswered questions.

## Integration Points

### Feedback System Integration
The follow-up questions feature is implemented within the `internal/feedback` package:

1. **Daily Aggregation**: Feedback system handles question aggregation from UserJob.JobMatchAnalysis.FollowUpQuestions
2. **Question Storage**: Feedback system maintains the `user_followup_questions` collection
3. **Priority Updates**: Feedback system re-ranks questions daily as new jobs are added
4. **Response Integration**: User answers feed back into job matching analysis
5. **Email Processing**: Feedback package includes the followup questions processor for email delivery and response handling

### Inbox System Integration
The `internal/inbox` package processes email responses via a dispatch system:

1. **Inbox Dispatch**: `internal/inbox_dispatch.go` implements email event dispatching (similar to `job_event_dispatch.go`)
2. **Handler Registration**: Main app registers inbox dispatch with inbox processor
3. **Event Distribution**: Inbox dispatch forwards emails to registered handlers including feedback package's followup processor
4. **Response Processing**: Feedback package's followup processor processes emails matching subject line patterns

### LLM Answer Extraction
When processing email responses, use LLM to create structured output with XML-style tags:

**LLM Prompt Output Format**:
```xml
<followup_response>
  <question_answer>
    <question_id>Q1-{firestoreDocID}</question_id>
    <question_text>What interests you most about remote work opportunities?</question_text>
    <answer>I'm interested in the flexibility and work-life balance that remote work offers.</answer>
  </question_answer>
  <question_answer>
    <question_id>Q2-{firestoreDocID}</question_id>
    <question_text>How important is company culture in your job search?</question_text>
    <answer>Very important - I need a collaborative and supportive environment.</answer>
  </question_answer>
  <additional_feedback>
    // TODO: Handle additional user feedback/questions
  </additional_feedback>
</followup_response>
```

**Question ID Extraction**: Parse the question_id field to extract the Firestore document ID (everything after "Q1-", "Q2-", etc.) and use it to update the corresponding `UserFollowupQuestion` document.

**Answer Processing**:
- Parse structured response to extract answers for each question
- Update `UserFollowupQuestion.Answer` field for matched questions
- If user answers the same question multiple times (Day 1 + Day 3), use LLM to merge into final answer
- Set `ResponseReceivedAt` and `AnswerEmailID` for tracking

### Database Operations
Implemented within `internal/feedback/followup_questions.go` for all database operations (following pattern of `internal/user_feedback.go`):

```go
// Core functions for managing follow-up questions
func GetUserFollowupQuestions(ctx context.Context, fs *FirestoreClient, userID string) ([]*UserFollowupQuestion, error)
func GetTopUnsentQuestions(ctx context.Context, fs *FirestoreClient, userID string, limit int) ([]*UserFollowupQuestion, error)
func GetDay1Questions(ctx context.Context, fs *FirestoreClient, userID string) ([]*UserFollowupQuestion, error)
func GetUnansweredQuestions(ctx context.Context, fs *FirestoreClient, userID string, limit int) ([]*UserFollowupQuestion, error)
func GetQuestionsForDay(ctx context.Context, fs *FirestoreClient, userID string, emailType string) ([]*UserFollowupQuestion, error)
func MarkQuestionsSent(ctx context.Context, fs *FirestoreClient, questionIDs []string, emailType string, timestamp time.Time) error
func UpdateQuestionAnswer(ctx context.Context, fs *FirestoreClient, questionID string, answer string, emailID string) error
func UpdateUserFollowupTimestamp(ctx context.Context, fs *FirestoreClient, userID string, field string, timestamp time.Time) error
func HasUserRespondedToDay1(ctx context.Context, fs *FirestoreClient, userID string) (bool, error)
func MergeQuestionAnswers(ctx context.Context, fs *FirestoreClient, questionID string, newAnswer string, existingAnswer string) error
```

### Email Template Data Structure
Define the data structure passed to email templates:

```go
type EmailTemplateData struct {
    User        *User                   `json:"user"`
    Questions   []*UserFollowupQuestion `json:"questions"`
    TrackingID  string                  `json:"tracking_id"`  // FAQ-{userID}-{timestamp}
    FollowupDay int                     `json:"followup_day"` // 1, 3, or 5
    
    // Personalization fields
    FirstName   string                  `json:"first_name"`
    FullName    string                  `json:"full_name"`
    
    // Email type specific data
    IsResend        bool                `json:"is_resend"`        // true for Day 3 resends
    HasResponses    bool                `json:"has_responses"`    // true if user responded to previous emails
    ResponseCount   int                 `json:"response_count"`   // number of questions answered
}
```

## Configuration and Settings

### Followup Processor Settings
```go
type FollowupSettings struct {
    Enabled              bool          `json:"enabled"`                // Master enable/disable switch
    Interval             time.Duration `json:"interval"`               // Processing interval (default: 5 minutes)
    Paused               bool          `json:"paused"`                 // Temporary pause processing
    
    // Timing Configuration
    Day1DelayHours       int           `json:"day1_delay_hours"`       // Hours after call completion (default: 24)
    Day3DelayHours       int           `json:"day3_delay_hours"`       // Hours after Day 1 email (default: 48)
    Day5DelayHours       int           `json:"day5_delay_hours"`       // Hours after Day 3 email (default: 48)
    
    // Email Configuration
    MaxQuestionsPerEmail int           `json:"max_questions_per_email"` // Max questions per email (default: 5)
    MinQuestionsPerEmail int           `json:"min_questions_per_email"` // Min questions per email (default: 3)
    
    // Operating Window
    StartHour            int           `json:"start_hour"`             // Operating hours start (UTC, default: 0)
    EndHour              int           `json:"end_hour"`               // Operating hours end (UTC, default: 0)
    
    // Error Handling
    MaxRetries           int           `json:"max_retries"`            // Max retries for failed operations (default: 3)
    RetryDelay           time.Duration `json:"retry_delay"`            // Delay between retries (default: 5 minutes)
}

var DefaultFollowupSettings = FollowupSettings{
    Enabled:              true,
    Interval:             5 * time.Minute,
    Paused:               false,
    Day1DelayHours:       24,
    Day3DelayHours:       48,
    Day5DelayHours:       48,
    MaxQuestionsPerEmail: 5,
    MinQuestionsPerEmail: 3,
    StartHour:            0,  // 24/7 operation
    EndHour:              0,
    MaxRetries:           3,
    RetryDelay:           5 * time.Minute,
}
```

## Technical Implementation

### Error Handling Strategy
For all LLM integration points, implement consistent error handling:

```go
// LLM Error Handling Pattern
func handleLLMError(ctx context.Context, err error, operation string, userID string) error {
    internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
        "operation": operation,
        "userID":    userID,
        "component": "feedback_followup_processor_llm",
    })
    return fmt.Errorf("LLM operation failed for %s: %w", operation, err)
}

// Usage examples:
// - Email content generation failure → Report to Sentry, skip user, retry next cycle
// - Response parsing failure → Report to Sentry, skip email, retry next cycle  
// - Answer merging failure → Report to Sentry, use new answer only
```

### Timing Logic with GetLatestCompletedAppointment
```go
func (p *Processor) getCallCompletionTime(ctx context.Context, userID string) (*time.Time, error) {
    appointment, found, err := internal.GetLatestCompletedAppointment(ctx, p.fs, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to get latest completed appointment: %w", err)
    }
    if !found {
        return nil, nil // No completed appointment
    }
    if appointment.CompletedAt == nil {
        return nil, fmt.Errorf("completed appointment has nil CompletedAt field")
    }
    return appointment.CompletedAt, nil
}

func (p *Processor) processFollowupQuestions(ctx context.Context, user *User) error {
    // Get call completion time from latest completed appointment
    completedAt, err := p.getCallCompletionTime(ctx, user.ID)
    if err != nil {
        return fmt.Errorf("failed to get call completion time: %w", err)
    }
    if completedAt == nil {
        return nil // No completed appointment yet
    }
    
    // Calculate timing based on actual appointment completion
    hoursSinceCall := time.Since(*completedAt).Hours()
    
    // Implement relative timing logic here...
}
```

## Transaction Handling

### Critical Transaction Boundaries
To ensure data consistency, the following operations must be wrapped in transactions:

#### 1. Email Send + Database Update Transaction
```go
func (p *Processor) sendEmailWithTracking(ctx context.Context, user *User, questions []*UserFollowupQuestion, emailType string) error {
    return p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        // 1. First attempt to send email (outside transaction for performance)
        emailData := p.buildEmailData(user, questions, emailType)
        if err := p.emailService.Send(ctx, emailData); err != nil {
            return fmt.Errorf("failed to send email: %w", err)
        }
        
        // 2. If email successful, update database within transaction
        now := time.Now()
        for _, question := range questions {
            question.SentToUserAt = &now
            question.EmailType = &emailType
            if err := tx.Set(p.fs.Collection("user_followup_questions").Doc(question.ID), question); err != nil {
                return fmt.Errorf("failed to update question %s: %w", question.ID, err)
            }
        }
        
        // 3. Update user timestamp
        timestampField := fmt.Sprintf("FollowupDay%sSentAt", emailType)
        userUpdates := []firestore.Update{{Path: timestampField, Value: now}}
        if err := tx.Update(p.fs.Collection("users").Doc(user.ID), userUpdates); err != nil {
            return fmt.Errorf("failed to update user timestamp: %w", err)
        }
        
        return nil
    })
}
```

#### 2. Response Processing Transaction  
```go
func (p *Processor) processEmailResponse(ctx context.Context, userID string, questionAnswers map[string]string, emailID string) error {
    return p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        now := time.Now()
        for questionID, answer := range questionAnswers {
            // Check if question already has an answer (for merging)
            questionRef := p.fs.Collection("user_followup_questions").Doc(questionID)
            doc, err := tx.Get(questionRef)
            if err != nil {
                continue // Skip invalid question IDs
            }
            
            var question UserFollowupQuestion
            if err := doc.DataTo(&question); err != nil {
                continue
            }
            
            // Merge or set answer
            if question.Answer != nil && *question.Answer != "" {
                // Use LLM to merge answers (outside transaction, then update)
                mergedAnswer, err := p.mergeAnswersWithLLM(ctx, *question.Answer, answer)
                if err != nil {
                    // Fallback: use new answer
                    question.Answer = &answer
                } else {
                    question.Answer = &mergedAnswer
                }
            } else {
                question.Answer = &answer
            }
            
            question.ResponseReceivedAt = &now
            question.AnswerEmailID = &emailID
            question.UpdatedAt = now
            
            if err := tx.Set(questionRef, question); err != nil {
                return fmt.Errorf("failed to update question answer: %w", err)
            }
        }
        return nil
    })
}
```

### Transaction Error Handling
- **Email Send Failure**: Report to Sentry, skip user, retry next processing cycle
- **Database Update Failure**: Log error, attempt transaction retry up to 3 times
- **Partial Transaction Failure**: Full rollback, report to Sentry with detailed context

## Response Processing

### Email Reply Handling with Auto-Reply Filtering
```go
func (p *Processor) isValidResponse(email *inbox.Email) bool {
    // Filter out auto-replies and system messages
    subject := strings.ToLower(email.Subject)
    body := strings.ToLower(email.Body)
    
    // Auto-reply indicators
    autoReplyKeywords := []string{
        "out of office", "automatic reply", "vacation", "away message",
        "delivery failure", "undelivered", "bounce", "mailer-daemon",
    }
    
    for _, keyword := range autoReplyKeywords {
        if strings.Contains(subject, keyword) || strings.Contains(body, keyword) {
            return false
        }
    }
    
    // Check for FAQ pattern in subject
    faqPattern := regexp.MustCompile(`\[FAQ-([^-]+)-(\d+)\]`)
    if !faqPattern.MatchString(email.Subject) {
        return false
    }
    
    return true
}
```

### Dashboard Integration (Future)
- Display follow-up questions in user dashboard
- Allow users to respond directly through web interface
- Show question history and responses
- Provide analytics on engagement rates

## Analytics and Metrics

### Tracking Metrics
- Question delivery rates by day (1, 3, 5)
- Response rates by question type
- Time to response after question delivery
- Correlation between responses and job application activity
- User engagement scores based on question participation

### Database Queries for Analytics
```go
// Response rate by followup day
func GetResponseRateByDay(ctx context.Context, fs *FirestoreClient, followupDay int) (float64, error)

// Most effective question types
func GetQuestionEffectiveness(ctx context.Context, fs *FirestoreClient) (map[string]float64, error)

// User engagement scores
func CalculateUserEngagement(ctx context.Context, fs *FirestoreClient, userID string) (float64, error)
```

## Implementation Files

### Core Service (within internal/feedback package)
- `internal/feedback/followup_questions_processor.go` - Main processor service with relative timing logic
- `internal/feedback/followup_email_generator.go` - Email generation with tracking identifiers
- `internal/feedback/followup_response_handler.go` - Email response processing and LLM answer extraction
- `internal/feedback/followup_types.go` - Data structures and constants

### Database Layer
- `internal/feedback/followup_questions.go` - All followup-related database operations (integrated with existing feedback operations)

### Email Event System  
- `internal/inbox_dispatch.go` - Email event dispatching system (pattern: `internal/job_event_dispatch.go`)

### Email Templates
- `assets/emails/shirely-followup-day1.html` - Day 1 questions email
- `assets/emails/shirely-followup-day3-resend.html` - Day 3 resend (no response)
- `assets/emails/shirely-followup-day3-new.html` - Day 3 new questions (with response)
- `assets/emails/shirely-followup-day5.html` - Day 5 marketing email

### LLM Prompts
- `assets/prompts/shirely-followup-day1.txt` - Day 1 question email generation
- `assets/prompts/shirely-followup-day3-resend.txt` - Day 3 resend email generation
- `assets/prompts/shirely-followup-day3-new.txt` - Day 3 new questions email generation
- `assets/prompts/shirely-followup-day5.txt` - Day 5 marketing email generation
- `assets/prompts/followup-answer-extraction.txt` - LLM prompt for parsing email responses

### Inbox Dispatch Interface
```go
// InboxEventHandler defines the interface for handling inbox events
type InboxEventHandler interface {
    HandleEmail(ctx context.Context, email *inbox.Email) error
}

// InboxEventDispatch dispatches email events to registered handlers
type InboxEventDispatch struct {
    mu       sync.RWMutex
    handlers map[string]InboxEventHandler
}

func NewInboxEventDispatch() *InboxEventDispatch
func (ied *InboxEventDispatch) Register(id string, handler InboxEventHandler) error
func (ied *InboxEventDispatch) Deregister(id string) bool
func (ied *InboxEventDispatch) HandleEmail(ctx context.Context, email *inbox.Email) error
```

### Integration Points
- Main app registers `InboxEventDispatch` with inbox processor
- Feedback package's followup processor registers as handler with inbox dispatch
- Add followup questions processor to main application startup in `cmd/web/main.go` as part of feedback system initialization
- Wire SMTP configuration through existing email infrastructure

## Testing Strategy

### Unit Tests
- Question generation with various user contexts
- Question deduplication logic
- Response parsing and mapping
- Timing logic for different followup days

### Integration Tests
- End-to-end flow from question generation to email delivery
- Response processing workflow
- Database consistency checks
- Integration with feedback system

### Data Migration
- Add new fields to existing User documents
- Create indexes for efficient question querying
- Set up collection structure for user_followup_questions

## Future Enhancements

### Advanced Features
- AI-powered response analysis for sentiment and actionability
- Dynamic question adaptation based on response patterns
- Integration with calendar systems for interview scheduling
- Automated follow-up based on user response urgency

### Personalization Improvements
- Question difficulty adjustment based on user technical level
- Industry-specific question templates
- Geographic market considerations in questions
- Company culture fit questions based on user preferences

## Implementation Priority

### Phase 1 (Core Functionality)
1. Database schema updates and migrations
2. Basic question generation and tracking
3. Integration with existing followup processor
4. Email templates and delivery

### Phase 2 (Response Handling)
1. Email reply processing
2. Response mapping and storage
3. Basic analytics and reporting
4. Dashboard integration

### Phase 3 (Advanced Features)
1. AI-powered response analysis
2. Dynamic question adaptation
3. Advanced analytics and insights
4. Integration with external systems