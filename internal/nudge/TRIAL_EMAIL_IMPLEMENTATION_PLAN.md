# Trial Email Strategy Implementation Plan

**Date:** January 30, 2025  
**Lead Engineer:** Implementation Plan  
**Project:** 7-Day Trial Email Sequence  

## Project Overview

Implement a 6-email trial sequence to maximize subscription conversion, starting with enhancing the existing recap email to serve as trial welcome, then building 5 additional emails with sophisticated targeting and personalization.

## ⚠️ **CRITICAL TIMEZONE ANALYSIS**

### Current System Timezone Handling

#### User Timezone Collection ✅
- **Scheduling**: Users provide timezone during appointment scheduling via JavaScript: `Intl.DateTimeFormat().resolvedOptions().timeZone`
- **Storage**: Timezone stored in `appointment.Timezone` field
- **Validation**: Handled by `internal/request/time.go` with proper timezone validation

#### Trial Period Calculation - Current Issue ⚡
```go
// In ProcessAppointmentComplete() - internal/appointment.go:327
subscriptionEnd := time.Now().Add(24 * 7 * time.Hour) // 7 days from now
```

**Problem**: This uses GMT/UTC for trial calculation, not user's timezone.

### Trial Period Analysis & Solution

#### Worst Case Scenario for Users 📉
- **User timezone**: Hawaii (UTC-10)
- **Call completion**: January 30, 2025 at 2:00 PM HST (midnight GMT Jan 31)
- **Current calculation**: Trial ends January 31 + 7 days = February 7 midnight GMT
- **User experience**: Trial ends February 6 at 2:00 PM HST (10 hours early!)

#### Best Case Scenario for Users 📈  
- **User timezone**: Auckland (UTC+13)
- **Call completion**: January 30, 2025 at 2:00 PM NZDT (1:00 AM GMT Jan 30)
- **Current calculation**: Trial ends January 30 + 7 days = February 6 midnight GMT  
- **User experience**: Trial ends February 6 at 1:00 PM NZDT (13 hours extra!)

#### **Recommended Solution: End-of-Day in User's Timezone**

```go
func CalculateTrialEndDate(callCompletionTime time.Time, userTimezone string) (time.Time, error) {
    // Load user's timezone
    loc, err := time.LoadLocation(userTimezone)
    if err != nil {
        return time.Time{}, fmt.Errorf("invalid timezone: %w", err)
    }
    
    // Convert call completion to user's timezone
    userTime := callCompletionTime.In(loc)
    
    // Add 7 days and set to end of day (23:59:59)
    trialEndDate := userTime.AddDate(0, 0, 7)
    trialEndDate = time.Date(
        trialEndDate.Year(), 
        trialEndDate.Month(), 
        trialEndDate.Day(),
        23, 59, 59, 0, // End of day
        loc,
    )
    
    // Convert back to UTC for storage
    return trialEndDate.UTC(), nil
}
```

#### **Benefits of This Approach**
✅ **User-friendly**: Always gives users the full 7 days in their local time  
✅ **Consistent**: Trial always ends at 11:59 PM in user's timezone  
✅ **Fair**: No timezone-based advantage/disadvantage  
✅ **Clear messaging**: "Your trial ends on [Date] at 11:59 PM [Timezone]"

### Email Display Timezone Handling

#### Current Email Timezone Context Missing ⚠️
**Problem**: Email templates use GMT timestamps without user timezone conversion.

#### **Solution: Dynamic Timezone Conversion in Templates**

```go
// Enhanced recap email data structure
type TrialEmailData struct {
    *NudgeEmailData
    
    // Trial dates in multiple formats
    TrialStartDate     time.Time    // UTC for calculations
    TrialEndDate       time.Time    // UTC for calculations  
    UserTimezone       string       // e.g., "America/New_York"
    
    // Pre-formatted strings for email display
    TrialStartDateUser string       // "January 30, 2025 at 2:00 PM EST"
    TrialEndDateUser   string       // "February 6, 2025 at 11:59 PM EST"
    DaysRemaining      int          // Calculated from user's current time
}
```

#### **Template Display Strategy**
```html
<!-- In email templates -->
<p>Your 7-day trial started on {{.TrialStartDateUser}} and will end on {{.TrialEndDateUser}}.</p>
<p>You have {{.DaysRemaining}} days remaining to explore all features.</p>
```

### **Can We Be Clever About Timezone Without Collecting It? 🤔**

#### Option 1: Use Appointment Timezone ✅ **RECOMMENDED**
- **Pro**: Already collected during scheduling
- **Pro**: High accuracy (user explicitly selected)
- **Pro**: No additional UX friction
- **Con**: Requires appointment to exist (but that's our trigger anyway)

#### Option 2: Browser-Based Detection 
- **Pro**: No user input required
- **Con**: Requires active session (not available in async email processing)
- **Con**: Can change based on travel/VPN

#### Option 3: IP-Based Geolocation
- **Pro**: No user input required  
- **Con**: Unreliable (VPNs, proxies, corporate networks)
- **Con**: Privacy concerns
- **Con**: Can be wrong

**Recommendation**: Use `appointment.Timezone` - it's reliable, already collected, and accurate.

## Technical Architecture Analysis

### Current System Dependencies
- **Email System**: `internal/smtp/mailer.go` - existing infrastructure ✅
- **Nudge System**: `internal/nudge/` - needs extension for trial types
- **Transcript Processing**: `internal/transcript/transcript_processor.go` - modify for enhanced recap
- **User Data**: `internal/user.go` - add trial engagement tracking
- **Templates**: `assets/emails/` and `assets/prompts/` - create new templates
- **Timezone Handling**: `internal/request/time.go` - extend for trial calculations ⚡

### Data Flow Analysis
```
Call Completion → Enhanced Recap Email (Day 0) → Timezone-Aware Trial Tracking
      ↓
Trial Tracking Started → User Activity Monitoring (timezone-aware)
      ↓  
Scheduled Trial Emails (Days 1-6) → Conversion Tracking → End at User's EOD
```

## Implementation Plan

### Phase 0: Timezone Foundation (Priority 0 - Must Do First) ⚡

#### 0.1 Fix Trial Period Calculation
**File**: `internal/appointment.go`
```go
// Update ProcessAppointmentComplete function
func ProcessAppointmentComplete(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
    return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        // ... existing appointment updates ...
        
        // Calculate trial end in user's timezone
        trialEnd, err := CalculateTrialEndDate(time.Now(), appointment.Timezone)
        if err != nil {
            // Fallback to 7 days UTC if timezone parsing fails
            trialEnd = time.Now().Add(7 * 24 * time.Hour)
        }
        
        if err := UpdateUserSubscriptionEndAtTx(ctx, fs, tx, user, trialEnd); err != nil {
            return fmt.Errorf("failed to update user subscription end at: %w", err)
        }
        
        // ... rest of existing code ...
    })
}

// Add new timezone-aware trial calculation function
func CalculateTrialEndDate(callCompletionTime time.Time, userTimezone string) (time.Time, error) {
    // Implementation as shown above
}
```

#### 0.2 Add Timezone Helper Functions
**File**: `internal/utils/timezone.go` (new file)
```go
package utils

import (
    "fmt"
    "time"
)

// FormatTimeInUserTimezone formats a UTC time for display in user's timezone
func FormatTimeInUserTimezone(utcTime time.Time, timezone string, format string) (string, error) {
    loc, err := time.LoadLocation(timezone)
    if err != nil {
        return "", fmt.Errorf("invalid timezone %s: %w", timezone, err)
    }
    
    userTime := utcTime.In(loc)
    return userTime.Format(format), nil
}

// CalculateDaysRemaining calculates days remaining from user's current time perspective
func CalculateDaysRemaining(trialEndUTC time.Time, userTimezone string) int {
    loc, err := time.LoadLocation(userTimezone)
    if err != nil {
        // Fallback to UTC calculation
        return int(trialEndUTC.Sub(time.Now()).Hours() / 24)
    }
    
    now := time.Now().In(loc)
    trialEnd := trialEndUTC.In(loc)
    
    duration := trialEnd.Sub(now)
    days := int(duration.Hours() / 24)
    
    if days < 0 {
        return 0
    }
    return days
}
```

### Phase 1: Enhanced Recap Email (Week 1) - **PRIORITY 1**

#### 1.1 Update Existing Recap Email Prompt
**File**: `assets/prompts/recap-email.001.txt`
**Action**: Enhance with trial context while preserving personalization

#### 1.2 Modify GenerateRecapEmail Function  
**File**: `internal/transcript/transcript_processor.go`
```go
func GenerateRecapEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, appointment *internal.Appointment, transcript *internal.Transcript, cfg smtp.Config, logger *slog.Logger) error {
    // Add timezone-aware trial timeline data
    trialStartFormatted, _ := utils.FormatTimeInUserTimezone(
        time.Now(), 
        appointment.Timezone, 
        "January 2, 2006 at 3:04 PM MST",
    )
    
    trialEndFormatted, _ := utils.FormatTimeInUserTimezone(
        user.SubscriptionEnd, 
        appointment.Timezone, 
        "January 2, 2006 at 11:59 PM MST",
    )
    
    promptData := map[string]any{
        "FullName":       user.Name,
        "FirstName":      user.FirstName,
        "Transcript":     transcript,
        
        // NEW: Timezone-aware trial context
        "TrialStartDate":     time.Now(),
        "TrialEndDate":       user.SubscriptionEnd,
        "TrialStartDateUser": trialStartFormatted,
        "TrialEndDateUser":   trialEndFormatted,
        "DaysRemaining":      utils.CalculateDaysRemaining(user.SubscriptionEnd, appointment.Timezone),
        "UserTimezone":       appointment.Timezone,
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
    
    // Trial progress (UTC for calculations)
    TrialDay              int
    DaysRemaining         int
    TrialStartDate        time.Time
    TrialEndDate          time.Time
    
    // Timezone handling
    UserTimezone          string
    TrialStartDateUser    string    // Formatted for user's timezone
    TrialEndDateUser      string    // Formatted for user's timezone
    
    // Personalization data
    AppointmentTimezone   string    // From appointment
    CallCompletionDate    time.Time
    
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
    UserTimezone        string             `firestore:"user_timezone"`       // NEW
    AppointmentID       string             `firestore:"appointment_id"`      // NEW - link to timezone source
}

type TrialEmailSent struct {
    Type     NudgeType  `firestore:"type"`
    SentAt   time.Time  `firestore:"sent_at"`
    Opened   bool       `firestore:"opened"`
    Clicked  bool       `firestore:"clicked"`
}
```

#### 2.4 Trial Email Scheduler with Timezone Awareness
**File**: `internal/nudge/trial_scheduler.go`
```go
type TrialEmailScheduler struct {
    fs     *internal.FirestoreClient
    mailer smtp.Mailer
    logger *slog.Logger
}

func (s *TrialEmailScheduler) ScheduleTrialEmails(ctx context.Context, user *internal.User, appointmentTimezone string) error {
    // Schedule emails 1-6 based on trial start date
    // Use user's timezone for scheduling decisions
    
    // Example: Schedule first activity email for 8 AM user's time tomorrow
    userLoc, err := time.LoadLocation(appointmentTimezone)
    if err != nil {
        userLoc = time.UTC // Fallback
    }
    
    tomorrowUserTime := time.Now().In(userLoc).Add(24 * time.Hour)
    emailTime := time.Date(
        tomorrowUserTime.Year(),
        tomorrowUserTime.Month(), 
        tomorrowUserTime.Day(),
        8, 0, 0, 0, // 8 AM user time
        userLoc,
    ).UTC()
    
    // Schedule the email
    // ... implementation
}

func (s *TrialEmailScheduler) ShouldSendEmail(ctx context.Context, user *internal.User, emailType NudgeType) (bool, error) {
    // Activity-based targeting logic
    // Match volume detection  
    // User archetype analysis
    // Timezone-aware timing checks
}
```

### Phase 3: Email Templates & Content (Week 4-5)

#### 3.1 Email Template Creation with Timezone Display
Create HTML/text templates for each email type with proper timezone formatting:

**Templates to create:**
- `assets/emails/trial-first-activity.html`
- `assets/emails/trial-progress-update.html`
- `assets/emails/trial-education.html`
- `assets/emails/trial-feature-spotlight.html`
- `assets/emails/trial-social-proof.html`
- `assets/emails/trial-final-call.html`

**Example timezone-aware template snippet:**
```html
<p>Your trial period started on {{.TrialStartDateUser}} and will end on {{.TrialEndDateUser}}.</p>
<p>You have <strong>{{.DaysRemaining}} days remaining</strong> to explore all features!</p>
```

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

func (g *ContentGenerator) GenerateTrialEmailData(ctx context.Context, user *internal.User, emailType NudgeType) (*TrialEmailData, error) {
    // Get user's appointment for timezone
    appointment, err := internal.GetLatestCompletedAppointment(ctx, g.fs, user.ID)
    if err != nil {
        return nil, err
    }
    
    // Calculate timezone-aware dates
    trialStartFormatted, _ := utils.FormatTimeInUserTimezone(
        appointment.CompletedAt, 
        appointment.Timezone, 
        "January 2, 2006 at 3:04 PM MST",
    )
    
    trialEndFormatted, _ := utils.FormatTimeInUserTimezone(
        user.SubscriptionEnd, 
        appointment.Timezone, 
        "January 2, 2006 at 11:59 PM MST",
    )
    
    return &TrialEmailData{
        // ... other fields ...
        UserTimezone:          appointment.Timezone,
        TrialStartDateUser:    trialStartFormatted,
        TrialEndDateUser:      trialEndFormatted,
        DaysRemaining:         utils.CalculateDaysRemaining(user.SubscriptionEnd, appointment.Timezone),
    }, nil
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

#### 4.2 Email Delivery Engine with Timezone Scheduling
**File**: `internal/nudge/trial_engine.go`
```go
type TrialEmailEngine struct {
    scheduler       *TrialEmailScheduler
    generator       *ContentGenerator
    targetingEngine *TargetingEngine
}

func (e *TrialEmailEngine) ProcessTrialEmails(ctx context.Context) error {
    // Daily job to process and send trial emails
    // Check all users in trial period
    // Use timezone-aware scheduling for optimal send times
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
    TimezoneAnalysis    map[string]float64  // Conversion by timezone
}
```

## Critical Follow-Up Questions

### 1. **Email Delivery Timing & Scheduling**
- **Q**: Should we use a dedicated email job queue or integrate with existing nudge scheduler?
- **Q**: What's the optimal send time for each timezone? (8 AM local, business hours only?)
- **Q**: Should emails be sent on weekends, or should we delay until Monday?
- **Q**: How do we handle Daylight Saving Time transitions during trial periods?

### 2. **User Activity Tracking**
- **Q**: How do we define "dashboard activity" for the first activity nudge? Page views, time spent, actions taken?
- **Q**: Should we track email opens/clicks through third-party service or build internally?
- **Q**: How frequently should we update user engagement metrics?
- **Q**: What constitutes "using feedback" - any rating, or specific feedback actions?

### 3. **Content Personalization & Timezone**
- **Q**: How many job matches should we include in progress update emails? Top 3, top 5, or variable based on quality?
- **Q**: Should market insights be generated in real-time or pre-computed daily/weekly?
- **Q**: How do we handle users with very niche roles where we have limited market data?
- **Q**: What's the fallback content if LLM generation fails?
- **Q**: Should email content mention the user's timezone explicitly? (e.g., "in your Pacific timezone")

### 4. **A/B Testing & Optimization**
- **Q**: What's the minimum sample size needed for statistical significance in A/B tests?
- **Q**: Should we test email frequency (6 vs 4 vs 3 emails) or content variations first?
- **Q**: How do we ensure test groups are balanced across user archetypes, industries, and timezones?
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
- **Q**: What if appointment timezone is invalid/corrupted?

### 7. **Performance & Scalability**
- **Q**: With potentially thousands of trial users across 24 timezones, how do we ensure email generation doesn't overwhelm LLM API limits?
- **Q**: Should we batch email generation and sending, or process individually?
- **Q**: How do we monitor and alert on email delivery failures?
- **Q**: What's the expected load on Firestore for tracking engagement metrics?

### 8. **Business Logic Clarifications**
- **Q**: Should trial emails continue if a user's subscription expires but they haven't converted?
- **Q**: Do we send trial emails to users who signed up but haven't completed their first call yet?
- **Q**: How do we handle users who are in trial but haven't uploaded a resume?
- **Q**: Should email content differ for users who have zero matches vs some low-quality matches?
- **Q**: Should we adjust trial length based on timezone to ensure fairness? (Always 7 full days local time vs 7 * 24-hour periods)

## Risk Assessment & Mitigation

### High Risk Areas
1. **Email Deliverability**: Risk of emails landing in spam
   - **Mitigation**: Implement proper SPF/DKIM, monitor reputation, A/B test subject lines

2. **LLM Content Quality**: Risk of inappropriate or low-quality generated content
   - **Mitigation**: Implement content validation, fallback templates, manual review process

3. **User Overwhelm**: Risk of too many emails causing unsubscribes
   - **Mitigation**: Frequency controls, unsubscribe options, engagement monitoring

4. **⚠️ Timezone Complexity**: Risk of incorrect trial calculations and poor UX
   - **Mitigation**: Comprehensive testing across timezones, fallback to UTC, clear documentation

### Medium Risk Areas
1. **Performance**: Email generation at scale across timezones
   - **Mitigation**: Async processing, rate limiting, caching strategies, timezone-aware batching

2. **Data Consistency**: Trial tracking across multiple systems with timezone handling
   - **Mitigation**: Transactional updates, data validation, monitoring, timezone validation

## Success Metrics & Monitoring

### Primary KPIs
- **Trial to Subscription Conversion Rate**: Target > 15%
- **Email Engagement Rate**: Target > 25% open rate, > 5% click rate
- **Time to First Dashboard Visit**: Target < 24 hours post-call
- **⚠️ Timezone Fairness**: Conversion rate variance across timezones < 5%

### Monitoring Setup
- **Email Delivery**: Track send success, bounce rates, spam reports
- **User Engagement**: Dashboard visit frequency, feedback usage
- **Conversion Funnel**: Track users through each email and conversion point
- **System Performance**: Email generation time, API usage, error rates
- **⚠️ Timezone Monitoring**: Track trial end accuracy, user timezone distribution

## Dependencies & Prerequisites
- [ ] Existing nudge system must be stable
- [ ] User subscription tracking must be accurate
- [ ] Email infrastructure must support higher volume
- [ ] LLM API credits/limits need review for increased usage
- [ ] Analytics/monitoring setup for email tracking
- [ ] ⚠️ **Timezone calculation fix must be deployed first** (Phase 0)

## Estimated Timeline
- **Phase 0**: 3 days (Timezone foundation) ⚡ **CRITICAL**
- **Phase 1**: 1 week (Enhanced recap email)
- **Phase 2**: 2 weeks (Core infrastructure)
- **Phase 3**: 2 weeks (Templates and content)
- **Phase 4**: 2 weeks (Advanced features)
- **Testing & Optimization**: 1 week
- **Total**: 8.5 weeks for full implementation

**Critical Path**: Timezone fix → Enhanced recap email → Trial tracking → Email templates → Scheduling engine 

## **IMMEDIATE ACTION REQUIRED** ⚡

1. **Fix trial period calculation** (Phase 0) - Users are currently getting inconsistent trial lengths
2. **Test timezone handling** across multiple user timezones
3. **Verify appointment timezone storage** is reliable and consistent

This timezone fix should be deployed **immediately** as it affects all current trial users. 