# Trial Email Strategy Implementation Plan

**Date:** July 3, 2025  
**Updated:** Based on TRIAL_EMAIL_STRATEGY.md  
**Project:** 5-Email Trial Sequence Implementation  

## Project Overview

Implement a refined 5-email trial sequence to maximize subscription conversion during the 7-day trial period. The approach focuses on template-based content with minimal LLM usage, emphasizing the established Shirely voice and strategic value positioning.

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

## Refined Implementation Plan

### Email Sequence Overview

**5-Email Sequence** (aligned with TRIAL_EMAIL_STRATEGY.md):

| Day | Email Type | Purpose | Content Approach | Testing Focus |
|-----|------------|---------|------------------|---------------|
| 0 | **Recap Email** | Personalized call recap + trial intro | Enhanced LLM-generated | Conversion impact |
| 1 | **Welcome to Talent Rodeo** | Set expectations, reduce confusion | Template with Shirely voice | Engagement metrics |
| 2 | **Why You Might Not See Many Matches Yet** | Prevent disappointment, manage expectations | Template with match volume switching | Volume category accuracy |
| 3 | **3 Tips to Improve Your Matches** | Drive engagement, showcase features | Template with Shirely voice | Feature adoption |
| 6 | **Trial Ending Soon – Stay in the Rodeo** | Convert while value is fresh | Template with Shirely voice | Conversion rate |

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

**Testing Goal**: Verify recap email enhancement improves trial engagement without breaking existing functionality.

**Success Metrics**:
- Recap email delivery rate > 98%
- Trial context appears correctly in all timezones
- User engagement increase > 10% vs baseline

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

**Testing Goal**: Establish reliable trial email scheduling and data collection.

**Success Metrics**:
- All 4 new nudge types defined and functional
- Match volume categorization accuracy > 95%
- Email scheduling works correctly across timezones
- No performance degradation on existing systems

#### 2.1 Extend Nudge Type System
**File**: `internal/nudge/types.go`
```go
const (
    // Existing types...
    // Note: Recap email uses existing system enhancement
    NudgeTypeTrialWelcome           NudgeType = "trial_welcome"          // Day 1
    NudgeTypeTrialExpectations      NudgeType = "trial_expectations"     // Day 2  
    NudgeTypeTrialEngagement        NudgeType = "trial_engagement"       // Day 3
    NudgeTypeTrialConversion        NudgeType = "trial_conversion"       // Day 6
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
    
    // Match statistics (aligned with strategy)
    TotalJobsAnalyzed     int
    TotalMatches          int
    HighQualityMatches    int       // Score >= 4.0 (used for volume determination)
    FiveStarMatches       int       // Score >= 4.5
    MatchVolumeCategory   string    // "high", "typical", "low" (strategy alignment)
    
    // Engagement metrics
    DashboardVisits       int
    FeedbackGiven         int
    LastActivityDate      *time.Time
    HasUsedFeedback       bool
}
```

#### 2.3 User Struct Enhancement for Trial Tracking
**File**: `internal/user.go`
```go
type User struct {
    // Existing fields...
    
    // Trial tracking fields
    TrialAppointmentID *string    `firestore:"trial_appointment_id,omitempty"` // ID of appointment that started trial
    TrialStartedAt     *time.Time `firestore:"trial_started_at,omitempty"`     // When trial period started
    TrialEndAt         *time.Time `firestore:"trial_end_at,omitempty"`         // When trial period ends (calculated)
    
    // Optional: Enhanced trial engagement tracking
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

// NOTE: With new User struct fields, we may simplify this to use:
// - user.TrialAppointmentID (replaces AppointmentID here)
// - user.TrialStartedAt (replaces TrialStartDate here)  
// - user.LastAccessedAt (replaces LastActivityDate here)
// This would reduce data duplication and maintain single source of truth

**Benefits of User struct approach:**
- Single source of truth for trial state
- No need to scan through appointments to find trial source
- Consistent trial timing across all systems
- Direct appointment reference for timezone data
- Prevents trial inconsistencies from multiple appointments
- Clear audit trail of which appointment started the trial

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

**Testing Goal**: Validate email content quality and delivery across all scenarios.

**Success Metrics**:
- All 4 email templates render correctly
- Match volume switching works for high/typical/low categories
- Shirely voice consistency maintained across templates
- Email deliverability > 95% (no spam issues)

#### 3.1 Static Email Template Creation (MVP Approach)
Create `text/template` based HTML/text templates for standard content:

**Static Templates to create:**
- `assets/emails/trial-welcome.html` - Welcome to Talent Rodeo (Day 1)
- `assets/emails/trial-expectations.html` - Why You Might Not See Many Matches Yet (Day 2)
- `assets/emails/trial-engagement.html` - 3 Tips to Improve Your Matches (Day 3)
- `assets/emails/trial-conversion.html` - Trial Ending Soon (Day 6)

**Template data structure:**
```html
<p>Your trial period started on {{.TrialStartDateUser}} and will end on {{.TrialEndDateUser}}.</p>
<p>You have <strong>{{.DaysRemaining}} days remaining</strong> to explore all features!</p>

{{if eq .MatchVolumeCategory "high"}}
<p>Great news! We've found {{.HighQualityMatches}} quality matches for you!</p>
{{else if eq .MatchVolumeCategory "typical"}}
<p>We've found {{.HighQualityMatches}} quality matches - the best roles take time to appear.</p>
{{else}}
<p>We've found {{.HighQualityMatches}} matches for your specialized criteria. Perfect roles are worth waiting for!</p>
{{end}}
```

#### 3.2 Match Volume Detection Function
**File**: `internal/nudge/match_volume.go`
```go
const (
    HighVolumeMatchThreshold    = 10 // 10+ quality matches = high volume
    TypicalVolumeMatchThreshold = 4  // 4-9 quality matches = typical volume
                                     // 3 or fewer quality matches = low volume
)

func DetermineMatchVolumeCategory(qualityMatches int) string {
    if qualityMatches >= HighVolumeMatchThreshold { 
        return "high" 
    }
    if qualityMatches >= TypicalVolumeMatchThreshold { 
        return "typical" 
    }
    return "low"
}
```

#### 3.3 Simplified Content Generation
**File**: `internal/nudge/trial_content.go`
```go
type TrialContentBuilder struct {
    fs *internal.FirestoreClient
}

func (b *TrialContentBuilder) BuildTrialEmailData(ctx context.Context, user *internal.User, emailType NudgeType) (*TrialEmailData, error) {
    // Get user's trial source appointment directly from user record
    if user.TrialAppointmentID == nil {
        return nil, fmt.Errorf("user %s has no trial appointment ID set", user.ID)
    }
    
    appointment, err := internal.GetAppointment(ctx, b.fs, *user.TrialAppointmentID)
    if err != nil {
        return nil, fmt.Errorf("failed to get trial appointment %s: %w", *user.TrialAppointmentID, err)
    }
    
    // Get match statistics
    highQualityMatches := b.getHighQualityMatchCount(ctx, user.ID)
    matchVolumeCategory := DetermineMatchVolumeCategory(highQualityMatches)
    
    return &TrialEmailData{
        UserTimezone:          appointment.Timezone,
        TrialStartDateUser:    formatUserTime(appointment.CompletedAt, appointment.Timezone),
        TrialEndDateUser:      formatUserTime(user.SubscriptionEnd, appointment.Timezone),
        DaysRemaining:         calculateDaysRemaining(user.SubscriptionEnd, appointment.Timezone),
        HighQualityMatches:    highQualityMatches,
        MatchVolumeCategory:   matchVolumeCategory,
    }, nil
}
```

### Phase 4: Integration & End-to-End Testing (Week 6-7)

**Testing Goal**: Complete email sequence functions perfectly from trial start to conversion.

**Success Metrics**:
- Complete 5-email sequence delivers without errors
- No duplicate emails sent to users
- Trial conversion rate improvement > 15%
- System performance remains stable under load

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

**A**: Use the existing nudge scheduler. ✅ **ANALYSIS CONFIRMS**: The current nudge processor is perfect for trial emails:
- **Runs every 1 hour** - sufficient frequency for trial email timing
- **Comprehensive safety controls**: Rate limiting, batch processing, error handling
- **Timezone-aware operating hours**: Can be configured for optimal send times (8-10 AM local)
- **Proven infrastructure**: Already handles user eligibility, email generation, and tracking
- **Feature flag integration**: Cloudflare KV for dynamic configuration
- **Template system**: Already supports both static templates and LLM generation
- **User state tracking**: Existing fields for email tracking and rate limiting

  - **Q**: What's the optimal send time for each timezone? (8 AM local, business hours only?)
**A**: The nudge scheduler runs every 1 hour, which is sufficient frequency. Recommended optimal send times:
- **8-10 AM local time** for progress updates and educational content
- **Business hours only** (9 AM - 5 PM local) to avoid weekend/evening interruptions  
- **Use nudge processor's operating hours feature** to configure timezone-appropriate windows

- **Q**: Should emails be sent on weekends, or should we delay until Monday?
**A**: Weekends are fine.  We should not delay until Monday.

- **Q**: How do we handle Daylight Saving Time transitions during trial periods?
**A**: Trials should end at 11:59 PM on the day of the trial end whatever the timezone
or state of daylight saving time.  

### 2. **User Activity Tracking**
- **Q**: How do we define "dashboard activity" for the first activity nudge? Page views, time spent, actions taken?
**A**: Every time the user visits an authenticated page on the site we update their
user record with "LastAccessedAt" set to the current time.  We should use this to track
user activity.

- **Q**: Should we track email opens/clicks through third-party service or build internally?
**A**: I do not want to add tracking at this time. This is an MVP.

- **Q**: How frequently should we update user engagement metrics?
**A**: We should update user engagement metrics every time the user visits an authenticated page on the site.

- **Q**: What constitutes "using feedback" - any rating, or specific feedback actions?
**A**: Any rating is considered "using feedback". The user would have already had 
their "LastAccessedAt" set to the current time even if they did not rate the job.

### 3. **Content Personalization & Timezone**
- **Q**: How many job matches should we include in progress update emails? Top 3, top 5, or variable based on quality?
**A**: Top 3 if they are rated 4 stars or higher.

  - **Q**: Should market insights be generated in real-time or pre-computed daily/weekly?
**A**: Remove market insights from all specs - not needed for MVP. Focus on job match progress and feedback system education.

- **Q**: How do we handle users with very niche roles where we have limited market data?
**A**:  If we returned fewer than 3 matches that are rated 4 stars or higher, we should
we should consider the user a "niche" user and explain how it may take time to 
find the exact role they are looking for.

  - **Q**: What's the fallback content if LLM generation fails?
**A**: Use `text/template` for standard email content with static templates. Reserve LLM generation only for "niche" users (< 3 matches rated 4+ stars) who need personalized explanations about specialized roles taking time to appear.

- **Q**: Should email content mention the user's timezone explicitly? (e.g., "in your Pacific timezone")
**A**: No.  We should not mention the user's timezone explicitly.

### 4. **A/B Testing & Optimization**
**A**: No A/B testing for MVP - focus on single, well-designed email sequence. Architecture should support future A/B testing without major refactoring.

### 5. **Integration Points**
- **Q**: How does this integrate with existing nudge email types (no resume, no call, five star matches)?
**A**: The current nudge emails are sent before the user is fully onboarded.  Once
they are onboarded, the trial starts and these trial emails are sent.

  - **Q**: Should trial emails override existing nudge emails or work alongside them?
**A**: Trial emails work alongside existing nudge emails. However, existing nudges (no resume, no call) only apply to pre-onboarded users. Once trial starts, only trial emails are sent.

  - **Q**: How do we handle users who unsubscribe from trial emails but want to keep other notifications?
**A**: No unsubscribe option for trial emails - they're essential onboarding communications. Users receive exactly 6 emails over 7 days.

  - **Q**: What happens if a user subscribes before trial ends - do we stop the sequence?
**A**: Hide subscription UI during trial - users can't self-subscribe until trial ends. Admin-initiated subscriptions are allowed but don't affect email sequence. UI shows "You'll receive 6 emails during your trial."

### 6. **Error Handling & Edge Cases**
  - **Q**: What happens if the recap email fails to send - do we retry or skip to trial email 1?
**A**: Trial requires successful recap email - if it fails, trial doesn't start. Retry recap email send and only proceed with trial setup after successful delivery.

  - **Q**: How do we handle users whose trial period changes (extensions, early conversions)?
**A**: No trial extensions allowed. Admin extensions don't affect email flow - sequence continues normally based on original trial dates.

  - **Q**: What's the behavior for users who have multiple appointments during trial?
**A**: Use the trial source appointment referenced by `user.TrialAppointmentID`. This ensures consistency and prevents trial timing changes from subsequent appointments. Only the first completed appointment starts the trial.

  - **Q**: How do we handle users who re-register after a failed trial?
**A**: Not applicable - users can only register once per email address.

  - **Q**: What if appointment timezone is invalid/corrupted?
**A**: Validate timezone on appointment creation. If corruption detected later, log to Sentry and fall back to UTC with error tracking.

### 7. **Performance & Scalability**
- **Q**: With potentially thousands of trial users across 24 timezones, how do we ensure email generation doesn't overwhelm LLM API limits?
**A**: If the LLM returns some form of rate limiting, we should back off and retry.

- **Q**: Should we batch email generation and sending, or process individually?
**A**: Send them individually.

- **Q**: How do we monitor and alert on email delivery failures?
**A**: Log the error to Sentry via `internal/sentry.go`.


### 8. **Business Logic Clarifications**
- **Q**: Do we send trial emails to users who signed up but haven't completed their first call yet?
**A**: No their trial doesn't start until completing their first call. "SubscriptionEnd" timestamp will be `nil`.
- **Q**: How do we handle users who are in trial but haven't uploaded a resume?
**A**: No their trial doesn't start until completing their first call. "SubscriptionEnd" timestamp will be `nil`.
- **Q**: Should email content differ for users who have zero matches vs some low-quality matches?
**A**: Yes we have a "niche" user.  We should explain how it may take time to find the exact role they are looking for.
- **Q**: Should we adjust trial length based on timezone to ensure fairness? (Always 7 full days local time vs 7 * 24-hour periods)
**A**: We have adjusted the spec above to reflect this.  The trial ends at 11:59 PM on the day of the trial end whatever the timezone
or state of daylight saving time.

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

## Phased Implementation with Testing

### Phase 0: Foundation (3 days) ⚡ **CRITICAL**
**Goal**: Fix timezone handling and establish reliable trial period calculation

**Tasks**:
1. Fix trial period calculation in `internal/appointment.go`
2. Add timezone helper functions
3. Test timezone handling across multiple regions

**Testing**: Manual testing with users in different timezones

### Phase 1: Enhanced Recap Email (1 week)
**Goal**: Enhance existing recap email with trial context

**Tasks**:
1. Update recap email prompt with trial information
2. Add timezone-aware trial timeline data
3. A/B test against current recap email

**Testing**: A/B test with 50% of users, monitor engagement metrics

### Phase 2: Core Infrastructure (1.5 weeks)  
**Goal**: Build reliable trial email scheduling system

**Tasks**:
1. Add 4 new nudge types
2. Implement TrialEmailData structure
3. Create match volume detection logic
4. Test email scheduling system

**Testing**: Unit tests + integration tests with test users

### Phase 3: Email Templates (1 week)
**Goal**: Create and test all email templates

**Tasks**:
1. Create 4 email templates with Shirely voice
2. Implement match volume switching logic
3. Test template rendering across email clients

**Testing**: Email deliverability tests, template rendering validation

### Phase 4: End-to-End Testing (1 week)
**Goal**: Complete sequence testing and optimization

**Tasks**:
1. Test complete 5-email sequence
2. Performance testing under load
3. Error handling and edge cases
4. Monitoring and analytics setup

**Testing**: Full user journey testing, load testing, error scenarios

**Total Timeline**: 4.5 weeks for MVP implementation 

## Key Implementation Decisions ✅

### **MVP-First Approach**
- **Static templates** for standard content (faster development, reliable)
- **Template-based content** with match volume switching (no LLM needed)
- **Existing nudge scheduler** (proven infrastructure, no new systems)
- **No A/B testing** (single optimized sequence)
- **Text/template engine** for email generation
- **5 emails total** (including enhanced recap)

### **User Experience Priorities** 
- **Timezone fairness** - trial ends 11:59 PM in user's local time
- **No unsubscribe** - 5 emails over 7 days is reasonable
- **Activity-based targeting** - only send if no recent dashboard visits
- **Quality over quantity** - show top 3 matches rated 4+ stars
- **Essential onboarding** - emails are part of trial value proposition

## **SAFETY & ROLLOUT** 🛡️

**See `TRIAL_EMAIL_SAFETY_AND_ROLLOUT.md` for comprehensive safety plan**

### **Key Safety Mechanisms**
- **Multi-layer rate limiting**: Per-cycle, per-user, per-type limits
- **Testing controls**: DryRun mode, test domains, allowed email lists  
- **State validation**: Trial period checks, appointment verification
- **Sequence integrity**: Proper email ordering, anti-duplication
- **Emergency controls**: Instant pause, rollback procedures

### **Phased Rollout**
1. **Silent Mode** (2 days): Production monitoring, no emails sent
2. **Team Testing** (3 days): Real emails to team members only
3. **Limited Beta** (1 week): 50 real users maximum
4. **Full Rollout**: Gradual scale-up over 3 weeks

## **IMMEDIATE ACTION REQUIRED** ⚡

1. **Fix trial period calculation** (Phase 0) - Users are currently getting inconsistent trial lengths
2. **Test timezone handling** across multiple user timezones  
3. **Verify appointment timezone storage** is reliable and consistent
4. **Review safety document** - Ensure all team members understand rollout procedures

This timezone fix should be deployed **immediately** as it affects all current trial users. 