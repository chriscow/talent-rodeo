# 7-Day Trial Email Strategy

**Date:** January 30, 2025  
**Purpose:** Maximize user engagement and subscription conversion during the free 7-day trial period

## Executive Summary

This strategy addresses the critical challenge of converting trial users to paid subscribers by creating a carefully orchestrated email sequence that demonstrates value, builds trust, and manages expectations. The approach recognizes that finding the "perfect job" takes time and positions waiting as a strategic advantage rather than a limitation.

**Key Update**: After analyzing the existing recap email system, we've determined that the current post-call recap email should be enhanced to serve as our trial welcome email, reducing the total sequence from 7 emails to 6 emails while leveraging existing personalization infrastructure.

## User Journey Context

### Trial Timeline
- **Day 0**: User completes AI call, subscription starts (7-day trial begins)
- **Day 1-7**: Full service with job searches and match analysis
- **Day 7**: Trial ends, subscription required to continue
- **Post-Day 7**: No new searches for expired users

### User Archetypes
1. **High-Volume Match Users**: Broad criteria, many good matches found quickly
2. **Selective/Niche Users**: Specific criteria, fewer but higher-quality matches
3. **Hard-to-Match Users**: C-suite, specialized roles, limited job postings
4. **Overwhelmed Users**: Too many criteria, analysis paralysis
5. **Passive Users**: Interested but not actively engaging with matches

## Email Strategy Framework

### Core Messaging Pillars
1. **Value Demonstration**: "Look what we found for you"
2. **Uniqueness**: "No one else does this kind of analysis"
3. **Patience Education**: "Great jobs are worth waiting for"
4. **Control & Tuning**: "You can refine your perfect match"
5. **Urgency**: "Don't miss your perfect opportunity"

### Email Sequence Design

## Current Recap Email Analysis & Integration

### Existing Recap Email System
The current recap email is sent immediately after call completion as part of the `ProcessCompletedCall()` workflow. Analysis shows this email is perfectly positioned to serve as our trial welcome email.

**Current Strengths:**
- ✅ **Perfect timing** - sent immediately post-call (Day 0 of trial)
- ✅ **Strong personalization** - uses transcript data and matches conversation tone
- ✅ **Expectation setting** - explains search timing and process
- ✅ **Feedback establishment** - introduces tuning concept

**Areas for Enhancement:**
- ⚠️ **Missing trial context** - no mention of 7-day trial period
- ⚠️ **Limited value positioning** - focuses on process vs. competitive advantages
- ⚠️ **No subscription preparation** - doesn't explain what happens after trial

## Email Type 0: Enhanced Recap + Trial Welcome
**Trigger**: Immediate post-call (replaces original "Welcome & Expectation Setting")  
**Purpose**: Combine personalized call recap with trial welcome and expectation setting  
**Tone**: Warm, personalized (matching call tone), excited about trial

### Content Strategy (Enhanced Approach)
#### Section 1: Personalized Recap (60% of email)
- Keep LLM-generated call summary using transcript data
- Preserve conversational tone matching from actual call
- Include specific points discussed in conversation
- Maintain technical expectations (search timing, dashboard refresh)

#### Section 2: Trial Welcome Context (30% of email)
- "Your 7-day premium trial starts now"
- "Full AI-powered job search for one week"  
- "See what makes us different from other job platforms"
- Clear trial timeline and benefits explanation

#### Section 3: Value & Uniqueness Messaging (10% of email)
- "Unlike other platforms that just match keywords..."
- "Your AI assistant is now working 24/7"
- "We analyze jobs other platforms miss"
- "Give feedback to train your personal AI"

### Enhanced Prompt Template
```txt
You are Shirely, an AI job search specialist at Talent Rodeo.
You just finished a phone call with {{.FullName}}, who is looking to you for assistance finding a job.

Now that your call is finished, you promised to send them a recap email.

IMPORTANT: This email also serves as their welcome to their 7-day premium trial.

Your email should:
1. Recap key points from your conversation (maintain personal tone)
2. Welcome them to their 7-day premium trial 
3. Explain what happens next (searches starting)
4. Set expectations about trial timeline and benefits
5. Position the uniqueness of AI-powered matching
6. Request feedback for search refinement
7. Include technical guidance about dashboard refresh

The trial ends in exactly 7 days ({{.TrialEndDate}}).
After the trial, they'll need to subscribe to continue receiving matches.

Tone: Warm, excited about trial, professional but casual, matching your speaking style from the call.
```

### Implementation Requirements
1. **Update recap email prompt** (`assets/prompts/recap-email.001.txt`)
2. **Enhance email data structure** with trial timeline information
3. **Add subscription context** to existing template
4. **Mark as "Trial Email Type 0"** in nudge system

---

## Email Type 1: First Activity Nudge
**Trigger**: 24-36 hours after call, only if no dashboard activity  
**Purpose**: Drive initial engagement with the platform  
**Tone**: Helpful, encouraging

### Content Strategy
- Gentle reminder about platform features
- Highlight the feedback/tuning system
- Explain how their input improves matching
- Create urgency around optimization

### Key Messages
- "Your job search is active - here's how to make it even better"
- "The more feedback you give, the smarter our AI becomes"
- "Users who tune their preferences find better matches faster"

---

## Email Type 2: Progress Update & Value Demo
**Trigger**: 48 hours after call (Day 2)  
**Purpose**: Show progress and demonstrate value  
**Tone**: Informative, proud of progress

### Dynamic Content Based on Match Volume

#### High-Volume Scenario (5+ matches)
- Celebrate the variety of opportunities found
- Highlight top 2-3 matches with analysis snippets
- Emphasize quality filtering working
- Encourage feedback to refine further

#### Moderate-Volume Scenario (2-4 matches)
- Position selectivity as a strength
- Showcase detailed analysis of best matches
- Explain how being selective leads to better outcomes
- Encourage patience for more opportunities

#### Low/No-Match Scenario (0-1 matches)
- Frame scarcity as exclusivity
- Explain market dynamics for their role level
- Position waiting as strategic advantage
- Highlight the thoroughness of search criteria
- Introduce expectation that perfect roles take time

### Key Messages
- "We've analyzed [X] jobs and found [Y] potential matches"
- "Our AI found these insights about your market..."
- "Quality over quantity - we're being selective for you"

---

## Email Type 3: Education & Patience Building
**Trigger**: Day 3-4, especially for users with few matches  
**Purpose**: Build confidence in the waiting strategy  
**Tone**: Educational, strategic advisor

### Content Strategy
- Share market insights for their role/industry
- Explain job posting cycles and timing
- Position strategic patience as competitive advantage
- Share success stories (anonymized)
- Reinforce uniqueness of approach

### Key Messages
- "The best [role type] positions often take 2-4 weeks to appear"
- "While others apply to hundreds of jobs, you're waiting for the right one"
- "Our data shows [industry] hiring peaks on [days/times]"
- "Previous users in similar roles found their perfect match after [timeframe]"

---

## Email Type 4: Feature Spotlight - Tuning System
**Trigger**: Day 4-5, prioritize users who haven't used feedback  
**Purpose**: Drive engagement with unique tuning feature  
**Tone**: Empowering, educational

### Content Strategy
- Deep dive into feedback system capabilities
- Show before/after examples of tuning impact
- Emphasize competitive advantage of personalization
- Create urgency around optimization time remaining

### Key Messages
- "No other job platform lets you train AI like this"
- "Your feedback teaches our AI your preferences better than any resume"
- "Users who tune their matches are 3x more likely to find their ideal role"
- "You still have [X] days left in your trial to perfect your search"

---

## Email Type 5: Social Proof & Urgency
**Trigger**: Day 5-6  
**Purpose**: Build confidence and create subscription urgency  
**Tone**: Confident, with gentle urgency

### Content Strategy
- Share success metrics and testimonials
- Explain what happens after trial ends
- Create urgency without being pushy
- Reinforce value proposition
- Soft call-to-action for subscription

### Key Messages
- "Users report finding roles [X]% faster than traditional job search"
- "Your trial ends in [X] days - here's what you'll miss without subscribing"
- "Keep your AI working for you 24/7"
- "The perfect role could appear tomorrow - stay ready"

---

## Email Type 6: Final Day - Strong CTA
**Trigger**: Day 6-7 (before expiration)  
**Purpose**: Drive subscription conversion  
**Tone**: Urgent but respectful, value-focused

### Content Strategy
- Summarize value delivered during trial
- List specific achievements and insights
- Clear explanation of subscription benefits
- Strong but respectful call-to-action
- Create urgency around losing momentum

### Key Messages
- "Your 7-day trial delivered: [specific achievements]"
- "Don't lose momentum on your job search"
- "Your AI has learned your preferences - keep the advantage"
- "Subscribe now to continue your personalized job search"

---

## Implementation Specifications

### New Nudge Types Required

```go
const (
    // Existing types...
    NudgeTypeTrialRecapWelcome      NudgeType = "trial_recap_welcome"    // Enhanced recap email
    NudgeTypeTrialFirstActivity     NudgeType = "trial_first_activity"  
    NudgeTypeTrialProgressUpdate    NudgeType = "trial_progress_update"
    NudgeTypeTrialEducation         NudgeType = "trial_education"
    NudgeTypeTrialFeatureSpotlight  NudgeType = "trial_feature_spotlight"
    NudgeTypeTrialSocialProof       NudgeType = "trial_social_proof"
    NudgeTypeTrialFinalCall         NudgeType = "trial_final_call"
)
```

### Timing Configuration

```go
type TrialEmailSettings struct {
    // Enhanced recap is sent immediately with call processing (no delay)
    FirstActivityDelayHours     int // 24-36 hours, only if no activity
    ProgressUpdateDelayHours    int // 48 hours
    EducationStartDay           int // Day 3
    FeatureSpotlightDay         int // Day 4-5
    SocialProofDay              int // Day 5-6
    FinalCallDay                int // Day 6-7
    
    // Activity thresholds
    MinMatchesForHighVolume     int // 5+ matches
    MinMatchesForModerate       int // 2-4 matches
    
    // Targeting controls
    SendEducationToLowMatches   bool // Target users with few matches
    SendFeatureToInactiveOnly   bool // Only users who haven't used feedback
}
```

### Data Requirements

Enhance `NudgeEmailData` for trial-specific content:

```go
type TrialEmailData struct {
    *NudgeEmailData // Inherit base fields
    
    // Trial progress
    TrialDay            int
    DaysRemaining       int
    SubscriptionEndsAt  time.Time
    
    // Enhanced recap-specific data
    Transcript          *internal.Transcript  // For personalized recap content
    TrialStartDate      time.Time            // When trial began
    TrialEndDate        time.Time            // When trial ends
    
    // Match statistics
    TotalJobsAnalyzed   int
    TotalMatches        int
    HighQualityMatches  int    // Score >= 4.0
    FiveStarMatches     int    // Score >= 4.5
    
    // Engagement metrics
    DashboardVisits     int
    FeedbackGiven       int
    LastActivityDate    *time.Time
    HasUsedFeedback     bool
    
    // Market insights
    IndustryInsights    string // AI-generated market analysis
    RoleInsights        string // AI-generated role-specific insights
    
    // Personalization
    MatchVolumeCategory string // "high", "moderate", "low"
    UserArchetype       string // Based on behavior analysis
}
```

### Conditional Logic Requirements

#### Match Volume Detection
```go
func determineMatchVolumeCategory(totalMatches int) string {
    if totalMatches >= 5 { return "high" }
    if totalMatches >= 2 { return "moderate" }
    return "low"
}
```

#### Activity-Based Targeting
```go
func shouldSendFirstActivityNudge(user *User, callTime time.Time) bool {
    // Only send if no dashboard activity within 24-36 hours
    if user.LastAccessedAt.After(callTime) {
        return false // User has been active
    }
    return time.Since(callTime) >= 24*time.Hour
}
```

#### Market Insights Generation
Generate dynamic content using LLM:
- Industry hiring trends
- Role-specific market analysis  
- Timing insights for their field
- Competitive landscape observations

### Email Templates Strategy

#### Enhanced Recap Email (Type 0)
- **Template**: Enhance existing `assets/emails/recap-email.html` 
- **Prompt**: Update existing `assets/prompts/recap-email.001.txt`
- **Implementation**: Modify existing `GenerateRecapEmail()` function

#### New Trial Email Templates
Create template files for remaining email types:
- `assets/emails/trial-first-activity.html`
- `assets/emails/trial-progress-update.html`
- `assets/emails/trial-education.html`
- `assets/emails/trial-feature-spotlight.html`
- `assets/emails/trial-social-proof.html`
- `assets/emails/trial-final-call.html`

### LLM Prompts Required

#### Enhanced Recap Prompt (Priority 1)
- **File**: `assets/prompts/recap-email.001.txt` (update existing)
- **Purpose**: Add trial context while preserving personalization

#### New Trial Prompts
Create prompt files for dynamic content:
- `assets/prompts/trial-market-insights.txt`
- `assets/prompts/trial-progress-summary.txt`
- `assets/prompts/trial-education-content.txt`

## Success Metrics

### Primary KPIs
- **Subscription Conversion Rate**: % of trial users who subscribe
- **Email Engagement Rate**: Open rates, click rates by email type
- **Platform Engagement**: Dashboard visits, feedback given during trial
- **Time to First Match**: How quickly users get quality matches

### Secondary Metrics
- **Match Quality Scores**: Average scores of matches found
- **User Satisfaction**: Survey responses about trial experience
- **Retention After Subscription**: 30-day retention of converted users
- **Feature Adoption**: Usage of feedback/tuning system

## A/B Testing Framework

### Test Variations
1. **Email Frequency**: 7 emails vs. 5 emails vs. 4 emails
2. **Tone Variations**: Professional vs. Casual vs. Friendly
3. **Urgency Levels**: High urgency vs. Medium urgency vs. Educational
4. **Content Types**: Match-focused vs. Feature-focused vs. Market-focused

### Test Segments
- Segment by user archetype (high-volume vs. selective vs. hard-to-match)
- Segment by industry/role type
- Segment by engagement level

## Risk Mitigation

### Email Fatigue Prevention
- Monitor unsubscribe rates by email type
- Implement send-time optimization
- Respect user timezone preferences
- Allow customization of email frequency

### Content Quality Assurance
- A/B test subject lines for open rates
- Monitor spam folder placement
- Ensure mobile-friendly templates
- Test email rendering across clients

### Conversion Pressure Balance
- Avoid aggressive sales tactics
- Focus on value demonstration
- Respect user decision-making process
- Provide clear value in every email

## Benefits of Enhanced Recap Approach

### ✅ **Operational Benefits**
- **No new infrastructure needed** - uses existing email system
- **Maintains conversation continuity** - seamless from call to trial
- **Leverages existing personalization** - transcript-based content
- **Preserves technical functionality** - dashboard guidance remains

### ✅ **User Experience Benefits**
- **Single, comprehensive welcome** - no confusing multiple "welcome" emails
- **Immediate value positioning** - sets trial expectations from start
- **Personal connection maintained** - builds on call relationship
- **Clear trial understanding** - user knows what they signed up for

### ✅ **Business Benefits**
- **Earlier conversion preparation** - subscription context from day 0
- **Better trial utilization** - users understand full trial value
- **Reduced email fatigue** - 6 emails vs 7 emails total
- **Higher engagement** - personalized content performs better

## Updated Email Sequence Summary

**New 6-Email Sequence:**

0. **Enhanced Recap + Trial Welcome** (Immediate post-call) ← **Replaces standalone welcome**
1. **First Activity Nudge** (24-36 hours after call, only if no dashboard activity)
2. **Progress Update & Value Demo** (48 hours after call - Day 2)
3. **Education & Patience Building** (Day 3-4, especially for users with few matches)
4. **Feature Spotlight - Tuning System** (Day 4-5, prioritize users who haven't used feedback)
5. **Social Proof & Urgency** (Day 5-6, building conversion motivation)
6. **Final Day Strong CTA** (Day 6-7, respectful but urgent conversion push)

## Implementation Priority

### Phase 1: Enhanced Recap Email (Week 1)
1. **Update recap email prompt** with trial context
2. **Enhance GenerateRecapEmail() function** with trial data
3. **A/B test enhanced vs. current version** to validate approach
4. **Mark as trial email type 0** in nudge system

### Phase 2: Core Trial Infrastructure (Week 2-3)
1. Implement remaining nudge types and data structures
2. Create basic email templates for types 1-6
3. Build conditional logic for match volume detection
4. Implement timing controls for trial sequence

### Phase 3: Content Development (Week 4-5)
1. Create high-quality email templates
2. Develop LLM prompts for dynamic content
3. Build market insights generation
4. Create A/B testing framework

### Phase 4: Optimization (Week 6-7)
1. Implement advanced personalization
2. Build comprehensive analytics
3. Create user preference controls
4. Launch full A/B testing program

## Immediate Next Steps

1. **Update `assets/prompts/recap-email.001.txt`** with trial context
2. **Modify `GenerateRecapEmail()` function** to include trial timeline data
3. **Test enhanced recap email** with existing users
4. **Plan remaining trial email templates** based on validated approach

This strategy transforms the 7-day trial from a simple time-limited demo into a comprehensive journey that builds value, manages expectations, and drives conversion through education and engagement rather than pressure tactics. The enhanced recap email serves as the perfect foundation, combining proven personalization with strategic trial positioning. 