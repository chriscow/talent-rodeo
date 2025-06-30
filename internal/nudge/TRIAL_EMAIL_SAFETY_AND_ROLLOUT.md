# Trial Email Safety & Rollout Plan

**Date:** January 30, 2025  
**Purpose:** Comprehensive safety mechanisms and phased rollout strategy for trial email implementation  
**Status:** Draft for Review

## Safety Architecture Overview

### Inherited Safety Mechanisms ✅

Trial emails will leverage the existing nudge processor's proven safety architecture:

#### **1. Multi-Layer Rate Limiting**
```go
// Per-cycle limits (prevent mass sending)
MaxEmailsPerRun: 50  // Default: max 50 emails per hour

// Per-user rate limiting (prevent spam)
WeeklyRateLimit: 7 * 24 * time.Hour  // Min 7 days between ANY nudges

// Per-type limits (prevent category spam)
MaxNudgesPerType: {
    TrialRecapWelcome:      1,  // Only one recap per user ever
    TrialFirstActivity:     1,  // Only one activity nudge
    TrialProgressUpdate:    1,  // Only one progress update
    TrialEducation:         1,  // Only one education email
    TrialFeatureSpotlight:  1,  // Only one feature spotlight
    TrialSocialProof:       1,  // Only one social proof
    TrialFinalCall:         1,  // Only one final call
}
```

#### **2. Testing Safety Controls**
```go
// Comprehensive test mode protection
TestModeEnabled:     true    // Restrict to safe recipients
DryRun:             true     // Log only, no actual sends
AllowedTestEmails:   []string{"madcow@gmail.com", "mindemann@gmail.com", ...}
RequireTestDomain:   true    // Only @test.talentrodeo.com emails
MaxTestEmailsTotal:  10      // Hard limit for test sessions
```

#### **3. Operating Controls**
```go
// Time-based controls
StartHour:  9    // 9 AM UTC (business hours)
EndHour:   17    // 5 PM UTC (no weekend/evening sends)
Interval:   1 * time.Hour  // Max once per hour processing

// Emergency controls
Enabled:  true   // Master kill switch
Paused:   false  // Emergency pause (keeps processor running)
```

### **Trial-Specific Safety Enhancements** 🚨

#### **1. Trial State Validation**
```go
func isUserEligibleForTrialEmail(user *internal.User, emailType NudgeType) (bool, string) {
    // Critical: Only send trial emails during active trial
    if user.SubscriptionEnd == nil {
        return false, "no trial period set"
    }
    
    if user.SubscriptionEnd.Before(time.Now()) {
        return false, "trial expired"
    }
    
    // Critical: Must have completed appointment
    appointment, err := internal.GetLatestCompletedAppointment(ctx, fs, user.ID)
    if err != nil || appointment == nil {
        return false, "no completed appointment found"
    }
    
    // Trial emails only for users who completed calls
    if user.Status < internal.UserStatusCallCompleted {
        return false, "user hasn't completed onboarding call"
    }
    
    return true, "eligible"
}
```

#### **2. Email Sequence Integrity**
```go
func validateTrialEmailSequence(user *internal.User, emailType NudgeType) (bool, string) {
    // Ensure emails are sent in proper order
    emailsSent := user.NudgeTypeCount
    
    switch emailType {
    case NudgeTypeTrialFirstActivity:
        // Can only send after recap
        if emailsSent[string(NudgeTypeTrialRecapWelcome)] == 0 {
            return false, "recap email not sent yet"
        }
        
    case NudgeTypeTrialProgressUpdate:
        // Can only send if trial is at least 48 hours old
        trialAge := time.Since(*user.SubscriptionEnd.Add(-7 * 24 * time.Hour))
        if trialAge < 48*time.Hour {
            return false, "trial too new for progress update"
        }
        
    // ... additional sequence validations
    }
    
    return true, "sequence valid"
}
```

#### **3. Anti-Duplication Safeguards**
```go
// Enhanced user tracking for trial emails
type TrialEmailTracking struct {
    TrialStartDate    time.Time               `firestore:"trial_start_date"`
    RecapSentAt       *time.Time              `firestore:"recap_sent_at"`
    LastTrialEmail    *NudgeType              `firestore:"last_trial_email"`
    TrialEmailsSent   map[string]time.Time    `firestore:"trial_emails_sent"`
    TrialEmailCount   int                     `firestore:"trial_email_count"`
}

func hasUserReceivedTrialEmail(user *internal.User, emailType NudgeType) bool {
    if user.TrialEmailTracking == nil {
        return false
    }
    
    sentAt, exists := user.TrialEmailTracking.TrialEmailsSent[string(emailType)]
    return exists && !sentAt.IsZero()
}
```

## **Critical Edge Cases & Protections** ⚠️

### **Edge Case 1: Multiple Appointment Scenario**
**Risk**: User has multiple appointments, causing duplicate trial sequences

**Solution**: Use dedicated fields on the User struct to track trial state and source appointment. This provides a single source of truth and prevents inconsistencies.

**Protection**:
```go
func getTrialSourceAppointment(ctx context.Context, fs *FirestoreClient, user *User) (*Appointment, error) {
    if user.TrialAppointmentID == nil {
        return nil, fmt.Errorf("user %s has no trial appointment ID set", user.ID)
    }
    
    return GetAppointment(ctx, fs, *user.TrialAppointmentID)
}

// Trial initialization when first appointment completes
func initializeUserTrial(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
    // Only initialize if no trial has been started
    if user.TrialAppointmentID != nil {
        return nil // Trial already initialized, skip
    }
    
    // Set trial tracking fields atomically
    userTimezone := parseTimezone(appointment.Timezone)
    trialStart := appointment.CompletedAt.In(userTimezone)
    trialEnd := calculateTrialEnd(trialStart, userTimezone)
    
    updates := map[string]any{
        "TrialAppointmentID": appointment.ID,
        "TrialStartedAt":     trialStart,
        "TrialEndAt":         trialEnd,
        "SubscriptionEnd":    trialEnd, // Maintain existing field for compatibility
    }
    
    return UpdateUser(ctx, fs, user.ID, updates)
}
```

**Safety Benefits**:
- Clear audit trail of which appointment started the trial
- Prevents duplicate trial sequences from subsequent appointments  
- Enables direct appointment lookup without scanning
- Maintains trial consistency across all systems

### **Edge Case 2: Recap Email Failure Recovery**
**Risk**: Recap email fails, but trial period starts anyway

**Protection**:
```go
func ensureRecapEmailSuccess(ctx context.Context, user *User, appointment *Appointment) error {
    // Trial setup is atomic with recap email success
    return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        // 1. Send recap email FIRST
        err := SendRecapEmail(ctx, fs, user, appointment, transcript, cfg, logger)
        if err != nil {
            return fmt.Errorf("recap email failed, aborting trial setup: %w", err)
        }
        
        // 2. Only set trial dates after successful email
        trialEnd := CalculateTrialEndDate(time.Now(), appointment.Timezone)
        if err := UpdateUserSubscriptionEndAtTx(ctx, fs, tx, user, trialEnd); err != nil {
            return err
        }
        
        // 3. Mark recap as sent
        user.TrialEmailTracking = &TrialEmailTracking{
            TrialStartDate: time.Now(),
            RecapSentAt:    &time.Now(),
        }
        
        return tx.Set(fs.Collection("users").Doc(user.ID), user)
    })
}
```

### **Edge Case 3: System Clock Issues**
**Risk**: Server time drift causes emails to be sent at wrong times

**Protection**:
```go
func validateEmailTiming(user *User, emailType NudgeType) (bool, string) {
    if user.SubscriptionEnd == nil {
        return false, "no trial end date"
    }
    
    trialStart := user.SubscriptionEnd.Add(-7 * 24 * time.Hour)
    now := time.Now()
    
    // Add 10-minute buffer for clock skew
    tolerance := 10 * time.Minute
    
    if now.Before(trialStart.Add(-tolerance)) {
        return false, "trial hasn't started yet"
    }
    
    if now.After(user.SubscriptionEnd.Add(tolerance)) {
        return false, "trial has ended"
    }
    
    return true, "timing valid"
}
```

## **Testing Strategy** 🧪

### **Phase 0: Unit Testing**
```go
func TestTrialEmailSafety(t *testing.T) {
    tests := []struct {
        name string
        user *internal.User
        emailType NudgeType
        expectSafe bool
        expectReason string
    }{
        {
            name: "user without trial",
            user: &internal.User{SubscriptionEnd: nil},
            emailType: NudgeTypeTrialFirstActivity,
            expectSafe: false,
            expectReason: "no trial period set",
        },
        {
            name: "expired trial",
            user: &internal.User{
                SubscriptionEnd: timePtr(time.Now().Add(-1 * time.Hour)),
            },
            emailType: NudgeTypeTrialFirstActivity,
            expectSafe: false,
            expectReason: "trial expired", 
        },
        {
            name: "valid trial user",
            user: &internal.User{
                SubscriptionEnd: timePtr(time.Now().Add(5 * 24 * time.Hour)),
                Status: internal.UserStatusCallCompleted,
            },
            emailType: NudgeTypeTrialFirstActivity,
            expectSafe: true,
            expectReason: "eligible",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            safe, reason := isUserEligibleForTrialEmail(tt.user, tt.emailType)
            assert.Equal(t, tt.expectSafe, safe)
            assert.Equal(t, tt.expectReason, reason)
        })
    }
}
```

### **Phase 1: Integration Testing**
```go
// Ultra-safe test configuration
var TrialEmailTestSettings = NudgeProcessorSettings{
    Enabled:             true,
    Interval:            30 * time.Second,
    MaxEmailsPerRun:     1,  // One at a time
    
    // Safety first
    DryRun:              true,
    TestModeEnabled:     true,
    AllowedTestEmails:   []string{"test@test.talentrodeo.com"},
    RequireTestDomain:   true,
    TestDomains:        []string{"test.talentrodeo.com"},
    MaxTestEmailsTotal: 3,
}
```

## **Phased Rollout Strategy** 🚀

### **Phase 1: Silent Mode (2 days)**
**Goal**: Verify logic in production without sending emails

```json
{
    "enabled": true,
    "dry_run": true,
    "test_mode_enabled": false,
    "max_emails_per_run": 5,
    "interval": "2h"
}
```

**Validation**: 
- Monitor "would send trial email" logs
- Verify user eligibility counts
- Check timezone calculations

### **Phase 2: Team Testing (3 days)**
**Goal**: Send real emails to team only

```json
{
    "enabled": true,
    "dry_run": false,
    "test_mode_enabled": true,
    "allowed_test_emails": [
        "madcow@gmail.com",
        "mindemann@gmail.com"
    ],
    "max_emails_per_run": 2,
    "max_test_emails_total": 20
}
```

### **Phase 3: Limited Beta (1 week)**
**Goal**: Test with 50 real users maximum

```json
{
    "enabled": true,
    "dry_run": false,
    "test_mode_enabled": false,
    "max_emails_per_run": 10,
    "start_hour": 14,
    "end_hour": 21,
    "interval": "2h"
}
```

### **Phase 4: Full Rollout**
**Goal**: Gradual scale-up

Week 1: `max_emails_per_run: 25`  
Week 2: `max_emails_per_run: 50`  
Week 3+: `max_emails_per_run: 100`

## **Monitoring & Alerting** 📊

### **Critical Alerts**
- Trial emails to non-trial users
- Emails after trial expiration  
- Volume spike (>2x normal)
- High error rate (>10%)
- Duplicate emails detected

### **Key Metrics**
- Emails sent by type per day
- Safety check failure rate
- Delivery success rate
- Timezone accuracy
- Trial conversion rate

### **Emergency Procedures**
```bash
# Emergency stop
curl -X PUT "cloudflare-kv/trial-settings" -d '{"paused": true}'

# Rollback affected users
UPDATE users SET trial_email_tracking = NULL 
WHERE last_nudge_email_sent > NOW() - INTERVAL 24 HOUR
```

## **Success Criteria**

**Phase 1 → 2**: Zero safety violations, accurate eligibility logic  
**Phase 2 → 3**: Team receives expected emails, no duplicates  
**Phase 3 → 4**: <5% delivery failures, no user complaints  
**Phase 4**: Stable for 2+ weeks, >10% conversion rate

This plan ensures trial emails are deployed safely while maintaining the ability to monitor, control, and optimize the system at every step. 