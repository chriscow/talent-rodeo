# Trial Email Strategy Implementation Plan

**Date:** January 30, 2025  
**Lead Engineer:** Implementation Plan  
**Project:** 7-Day Trial Email Sequence  

## Project Overview

Implement a 6-email trial sequence to maximize subscription conversion, starting with enhancing the existing recap email to serve as trial welcome, then building 5 additional emails with sophisticated targeting and personalization.

## Technical Architecture Analysis

### Current System Dependencies
- **Email System**: `internal/smtp/mailer.go` - existing infrastructure ✅
- **Nudge System**: `internal/nudge/` - needs extension for trial types
- **Transcript Processing**: `internal/transcript/transcript_processor.go` - modify for enhanced recap
- **User Data**: `internal/user.go` - add trial engagement tracking
- **Templates**: `assets/emails/` and `assets/prompts/` - create new templates

### Data Flow Analysis
```
Call Completion → Enhanced Recap Email (Day 0)
      ↓
Trial Tracking Started → User Activity Monitoring
      ↓  
Scheduled Trial Emails (Days 1-6) → Conversion Tracking
```

## Implementation Plan

### Phase 1: Enhanced Recap Email (Week 1) - **PRIORITY 1**

#### 1.1 Update Existing Recap Email Prompt
**File**: `assets/prompts/recap-email.001.txt`
**Action**: Enhance with trial context while preserving personalization

#### 1.2 Modify GenerateRecapEmail Function  
**File**: `internal/transcript/transcript_processor.go`
```go
func GenerateRecapEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, appointment *internal.Appointment, transcript *internal.Transcript, cfg smtp.Config, logger *slog.Logger) error {
    // Add trial timeline data
    promptData := map[string]any{
        "FullName":       user.Name,
        "FirstName":      user.FirstName,
        "Transcript":     transcript,
        
        // NEW: Trial context
        "TrialStartDate": time.Now(),  // NOTE: This is in GMT and to display in the user's timezone, we need to convert it to the user's timezone
        "TrialEndDate":   time.Now().Add(7 * 24 * time.Hour),  // NOTE: This is in GMT and to display in the user's timezone, we need to convert it to the user's timezone
        "DaysRemaining":  7,
        "SubscriptionEnd": user.SubscriptionEnd, // From appointment completion  // NOTE: This is in GMT and to display in the user's timezone, we need to convert it to the user's timezone
    }
    
    // Rest of existing implementation...
}
```

#### 1.3 A/B Testing Setup
- **Control**: Current recap email
- **Treatment**: Enhanced recap with trial context
- **Metrics**: Email open rate, dashboard visits, trial conversion
- **Duration**: 2 weeks minimum

### Phase 2: Core Trial Infrastructure (Week 2-3)

#### 2.1 Extend Nudge Type System
**File**: `internal/nudge/types.go`
```go
const (
    // Existing types...
    NudgeTypeTrialRecapWelcome      NudgeType = "trial_recap_welcome"
    NudgeTypeTrialFirstActivity     NudgeType = "trial_first_activity"  
    NudgeTypeTrialProgressUpdate    NudgeType = "trial_progress_update"
    NudgeTypeTrialEducation         NudgeType = "trial_education"
    NudgeTypeTrialFeatureSpotlight  NudgeType = "trial_feature_spotlight"
    NudgeTypeTrialSocialProof       NudgeType = "trial_social_proof"
    NudgeTypeTrialFinalCall         NudgeType = "trial_final_call"
)
```

#### 2.2 Enhanced Trial Email Data Structure
**File**: `internal/nudge/types.go`
```go
type TrialEmailData struct {
    *NudgeEmailData // Inherit base fields
    
    // Trial progress
    TrialDay              int
    DaysRemaining         int
    TrialStartDate        time.Time
    TrialEndDate          time.Time
    SubscriptionEndsAt    time.Time
    
    // Personalization data
    Transcript            *internal.Transcript
    CallDate              time.Time
    
    // Match statistics
    TotalJobsAnalyzed     int
    TotalMatches          int
    HighQualityMatches    int
    FiveStarMatches       int
    
    // Engagement metrics
    DashboardVisits       int
    FeedbackGiven         int
    LastActivityDate      *time.Time
    HasUsedFeedback       bool
    
    // Dynamic content
    MatchVolumeCategory   string // "high", "moderate", "low"
    UserArchetype         string // Behavioral analysis
    IndustryInsights      string // AI-generated content
    RoleInsights          string // AI-generated content
}
```

#### 2.3 User Activity Tracking Enhancement
**File**: `internal/user.go`
```go
type User struct {
    // Existing fields...
    
    // Trial engagement tracking
    TrialEngagement *TrialEngagement `firestore:"trial_engagement,omitempty"`
}

type TrialEngagement struct {
    DashboardVisits     int                `firestore:"dashboard_visits"`
    FeedbackGiven       int                `firestore:"feedback_given"`
    LastActivityDate    *time.Time         `firestore:"last_activity_date,omitempty"`
    EmailsReceived      []TrialEmailSent   `firestore:"emails_received"`
    TrialStartDate      time.Time          `firestore:"trial_start_date"`
}

type TrialEmailSent struct {
    Type     NudgeType  `firestore:"type"`
    SentAt   time.Time  `firestore:"sent_at"`
    Opened   bool       `firestore:"opened"`
    Clicked  bool       `firestore:"clicked"`
}
```

#### 2.4 Trial Email Scheduler
**File**: `internal/nudge/trial_scheduler.go`
```go
type TrialEmailScheduler struct {
    fs     *internal.FirestoreClient
    mailer smtp.Mailer
    logger *slog.Logger
}

func (s *TrialEmailScheduler) ScheduleTrialEmails(ctx context.Context, user *internal.User) error {
    // Schedule emails 1-6 based on trial start date
    // Check eligibility for each email type
    // Queue emails with proper timing
}

func (s *TrialEmailScheduler) ShouldSendEmail(ctx context.Context, user *internal.User, emailType NudgeType) (bool, error) {
    // Activity-based targeting logic
    // Match volume detection
    // User archetype analysis
}
```

### Phase 3: Email Templates & Content (Week 4-5)

#### 3.1 Email Template Creation
Create HTML/text templates for each email type:

**Templates to create:**
- `assets/emails/trial-first-activity.html`
- `assets/emails/trial-progress-update.html`
- `assets/emails/trial-education.html`
- `assets/emails/trial-feature-spotlight.html`
- `assets/emails/trial-social-proof.html`
- `assets/emails/trial-final-call.html`

#### 3.2 LLM Prompt Development
**Priority Prompts:**
- `assets/prompts/trial-progress-summary.txt` - Dynamic progress content
- `assets/prompts/trial-market-insights.txt` - Industry-specific insights
- `assets/prompts/trial-education-content.txt` - Role-specific education

#### 3.3 Dynamic Content Generation
**File**: `internal/nudge/content_generator.go`
```go
type ContentGenerator struct {
    llmClient tools.LLMClient
}

func (g *ContentGenerator) GenerateProgressSummary(ctx context.Context, user *internal.User, matches []UserJob) (string, error) {
    // Generate personalized progress content based on matches found
}

func (g *ContentGenerator) GenerateMarketInsights(ctx context.Context, user *internal.User) (string, error) {
    // Generate industry/role specific market insights
}
```

### Phase 4: Advanced Features (Week 6-7)

#### 4.1 Match Volume Detection
**File**: `internal/nudge/targeting.go`
```go
func DetermineMatchVolumeCategory(totalMatches int) string {
    if totalMatches >= 5 { return "high" }
    if totalMatches >= 2 { return "moderate" }
    return "low"
}

func AnalyzeUserArchetype(ctx context.Context, user *internal.User) string {
    // Behavioral analysis based on activity patterns
    // Return: "high_volume", "selective", "hard_to_match", "overwhelmed", "passive"
}
```

#### 4.2 Email Delivery Engine
**File**: `internal/nudge/trial_engine.go`
```go
type TrialEmailEngine struct {
    scheduler     *TrialEmailScheduler
    generator     *ContentGenerator
    targetingEngine *TargetingEngine
}

func (e *TrialEmailEngine) ProcessTrialEmails(ctx context.Context) error {
    // Daily job to process and send trial emails
    // Check all users in trial period
    // Determine which emails to send
    // Generate personalized content
    // Send emails and track metrics
}
```

#### 4.3 Analytics & Metrics
**File**: `internal/nudge/trial_analytics.go`
```go
type TrialAnalytics struct {
    ConversionRate      float64
    EmailEngagementRate map[NudgeType]float64
    UserArchetypeMetrics map[string]TrialMetrics
}

func CalculateTrialMetrics(ctx context.Context, fs *internal.FirestoreClient) (*TrialAnalytics, error) {
    // Calculate conversion rates by email type
    // Analyze engagement patterns
    // Generate insights for optimization
}
```

## Critical Follow-Up Questions

### 1. **Email Delivery Timing & Scheduling**
- **Q**: Should we use a dedicated email job queue or integrate with existing nudge scheduler?
- **Q**: What time zone should we use for email delivery timing? User's timezone or company timezone?
- **Q**: How do we handle users who complete calls outside business hours?
- **Q**: Should emails be sent on weekends, or should we delay until Monday?

### 2. **User Activity Tracking**
- **Q**: How do we define "dashboard activity" for the first activity nudge? Page views, time spent, actions taken?
- **Q**: Should we track email opens/clicks through third-party service or build internally?
- **Q**: How frequently should we update user engagement metrics?
- **Q**: What constitutes "using feedback" - any rating, or specific feedback actions?

### 3. **Content Personalization**
- **Q**: How many job matches should we include in progress update emails? Top 3, top 5, or variable based on quality?
- **Q**: Should market insights be generated in real-time or pre-computed daily/weekly?
- **Q**: How do we handle users with very niche roles where we have limited market data?
- **Q**: What's the fallback content if LLM generation fails?

### 4. **A/B Testing & Optimization**
- **Q**: What's the minimum sample size needed for statistical significance in A/B tests?
- **Q**: Should we test email frequency (6 vs 4 vs 3 emails) or content variations first?
- **Q**: How do we ensure test groups are balanced across user archetypes and industries?
- **Q**: What's the success metric priority: open rate, click rate, or conversion rate?

### 5. **Integration Points**
- **Q**: How does this integrate with existing nudge email types (no resume, no call, five star matches)?
- **Q**: Should trial emails override existing nudge emails or work alongside them?
- **Q**: How do we handle users who unsubscribe from trial emails but want to keep other notifications?
- **Q**: What happens if a user subscribes before trial ends - do we stop the sequence?

### 6. **Error Handling & Edge Cases**
- **Q**: What happens if the recap email fails to send - do we retry or skip to trial email 1?
- **Q**: How do we handle users whose trial period changes (extensions, early conversions)?
- **Q**: What's the behavior for users who have multiple appointments during trial?
- **Q**: How do we handle users who re-register after a failed trial?

### 7. **Performance & Scalability**
- **Q**: With potentially thousands of trial users, how do we ensure email generation doesn't overwhelm LLM API limits?
- **Q**: Should we batch email generation and sending, or process individually?
- **Q**: How do we monitor and alert on email delivery failures?
- **Q**: What's the expected load on Firestore for tracking engagement metrics?

### 8. **Business Logic Clarifications**
- **Q**: Should trial emails continue if a user's subscription expires but they haven't converted?
- **Q**: Do we send trial emails to users who signed up but haven't completed their first call yet?
- **Q**: How do we handle users who are in trial but haven't uploaded a resume?
- **Q**: Should email content differ for users who have zero matches vs some low-quality matches?

## Risk Assessment & Mitigation

### High Risk Areas
1. **Email Deliverability**: Risk of emails landing in spam
   - **Mitigation**: Implement proper SPF/DKIM, monitor reputation, A/B test subject lines

2. **LLM Content Quality**: Risk of inappropriate or low-quality generated content
   - **Mitigation**: Implement content validation, fallback templates, manual review process

3. **User Overwhelm**: Risk of too many emails causing unsubscribes
   - **Mitigation**: Frequency controls, unsubscribe options, engagement monitoring

### Medium Risk Areas
1. **Performance**: Email generation at scale
   - **Mitigation**: Async processing, rate limiting, caching strategies

2. **Data Consistency**: Trial tracking across multiple systems
   - **Mitigation**: Transactional updates, data validation, monitoring

## Success Metrics & Monitoring

### Primary KPIs
- **Trial to Subscription Conversion Rate**: Target > 15%
- **Email Engagement Rate**: Target > 25% open rate, > 5% click rate
- **Time to First Dashboard Visit**: Target < 24 hours post-call

### Monitoring Setup
- **Email Delivery**: Track send success, bounce rates, spam reports
- **User Engagement**: Dashboard visit frequency, feedback usage
- **Conversion Funnel**: Track users through each email and conversion point
- **System Performance**: Email generation time, API usage, error rates

## Dependencies & Prerequisites
- [ ] Existing nudge system must be stable
- [ ] User subscription tracking must be accurate
- [ ] Email infrastructure must support higher volume
- [ ] LLM API credits/limits need review for increased usage
- [ ] Analytics/monitoring setup for email tracking

## Estimated Timeline
- **Phase 1**: 1 week (Enhanced recap email)
- **Phase 2**: 2 weeks (Core infrastructure)
- **Phase 3**: 2 weeks (Templates and content)
- **Phase 4**: 2 weeks (Advanced features)
- **Testing & Optimization**: 1 week
- **Total**: 8 weeks for full implementation

**Critical Path**: Enhanced recap email → Trial tracking → Email templates → Scheduling engine 