# 7-Day Trial Email Strategy

**Date:** July 3, 2025  
**Purpose:** Maximize user engagement and subscription conversion during the free 7-day trial period

## Executive Summary

This strategy addresses the critical challenge of converting trial users to paid subscribers by creating a carefully orchestrated 5-email sequence that demonstrates value, builds trust, and manages expectations. The approach recognizes that finding the "perfect job" takes time and positions waiting as a strategic advantage rather than a limitation.

**Key Update**: After analyzing the existing recap email system and user experience considerations, we've determined that a focused 5-email sequence will be more effective than the original 7-email approach, reducing email fatigue while hitting key psychological moments in the trial journey.

## User Journey Context

### Trial Timeline
- **Day 0**: User completes AI call, subscription starts (7-day trial begins)
- **Day 1-7**: Full service with job searches and match analysis
- **End of Day 7**: Trial ends, subscription required to continue
- **Post-Day 7**: No new searches performed or jobs analyzed for expired users

### User Archetypes
1. **High-Volume Match Users**: Broad criteria, many good matches found quickly but not that specific.
2. **Typical Match Users**: Moderate criteria, reasonable number of quality matches.
3. **Low-Volume Match Users**: Very specific criteria, few but high-quality matches OR specialized roles with limited postings.
4. **Passive Users**: Interested but not actively engaging with matches

## Email Strategy Framework

### Core Messaging Pillars
1. **Value Demonstration**: "Look what we found for you"
2. **Uniqueness**: "No one else does this kind of analysis"
3. **Patience Education**: "Great jobs are worth waiting for"
4. **Control & Tuning**: "You can refine your perfect match"
5. **Quality Over Quantity**: "This service is for finding the RIGHT job, not ANY job"

### Shirely Voice & Tone Integration

All email content must follow the established Shirely voice guidelines (see `SHIRELY_VOICE_AND_TONE_GUIDELINES.md`):
- **Self-aware AI humor**: Confident about being an AI, references advantages
- **Name play**: Frequent references to "HIRE is in my name"
- **Conversational enthusiasm**: Genuine excitement about helping careers
- **Professional but approachable**: Uses casual asides and contractions
- **Career-focused empathy**: Understands job search frustrations and aspirations

**Email Template Prompt**: "Write [email type] from Shirely, the AI behind Talent Rodeo. She's enthusiastic, self-aware about being an AI (in a confident way), and has HIRE literally in her name which she references playfully. She focuses on understanding career aspirations beyond resumes, works 24/7 as a personal recruiter, and genuinely cares about helping people find the right career moves. Tone should be conversational, encouraging, and professional but approachable. Include her personality quirks like referencing AI advantages, using casual asides, and showing excitement about helping people's careers."

## Refined 5-Email Sequence

### Email Type 0: Recap Email (Day 0)
**Trigger**: Immediate post-call completion  
**Purpose**: Personalized call recap with brief trial context  
**Content**: LLM-generated using existing recap system with trial timeline added  
**Tone**: Warm, personalized (matching call tone), sets stage for trial

### Email Type 1: Welcome to Talent Rodeo (Day 1)
**Trigger**: Day 1 after call  
**Purpose**: Set expectations, reduce confusion, introduce platform value  
**Content**: Template-based with Shirely voice  
**Tone**: Enthusiastic welcome, educational about unique approach

**Key Messages:**
- "Welcome to your personal AI recruiter experience"
- "Here's what makes us different from job boards"
- "Your 7-day trial timeline and what to expect"
- "I'm working 24/7 to find your perfect matches"

### Email Type 2: Why You Might Not See Many Matches Yet (Day 2)
**Trigger**: Day 2 after call  
**Purpose**: Prevent drop-off from early disappointment, manage expectations  
**Content**: Template-based with match volume switching (`high`/`typical`/`low`)  
**Tone**: Understanding, educational, positioning scarcity as quality

**Dynamic Content Based on Match Volume:**

#### High Volume Scenario (10+ quality matches)
- Celebrate the variety of opportunities found
- Emphasize quality filtering is working
- "I'm being selective for you - quality over quantity"

#### Typical Scenario (4-9 quality matches)
- Position selectivity as a strength
- Explain market timing and job posting cycles
- "The best roles take time to appear"

#### Low Volume Scenario (3 or fewer quality matches)
- Frame scarcity as exclusivity for specialized roles
- Explain market dynamics and posting cycles
- "Perfect roles are worth waiting for"

### Email Type 3: 3 Tips to Improve Your Matches (Day 3)
**Trigger**: Day 3 after call  
**Purpose**: Drive engagement, encourage feedback/tuning, showcase unique features  
**Content**: Template-based with Shirely voice  
**Tone**: Helpful, empowering, action-oriented  
**Send to**: All users (additional touchpoint for engagement)

**Key Messages:**
- "Here's how to make me even smarter about your preferences"
- "No other platform lets you train AI like this"
- "3 specific ways to improve your match quality"
- "I learn fast - it's one of my better qualities!"

### Email Type 4: Trial Ending Soon – Stay in the Rodeo (Day 6)
**Trigger**: Day 6 after call (24 hours before trial expiration)  
**Purpose**: Drive subscription conversion while value is fresh  
**Content**: Template-based with Shirely voice  
**Tone**: Urgent but respectful, value-focused, quality-positioning

**Key Messages:**
- "Jobs are posted constantly - you might miss your perfect opportunity"
- "This service isn't for someone taking ANY job - it's for finding the RIGHT job"
- "Keep your personal AI recruiter working for you"
- "Don't lose momentum on finding your perfect career move"

## Implementation Specifications

### New Nudge Types Required

```go
const (
    // Existing types...
    NudgeTypeTrialWelcome           NudgeType = "trial_welcome"          // Day 1
    NudgeTypeTrialExpectations      NudgeType = "trial_expectations"     // Day 2
    NudgeTypeTrialEngagement        NudgeType = "trial_engagement"       // Day 3
    NudgeTypeTrialConversion        NudgeType = "trial_conversion"       // Day 6
)
```

### Match Volume Constants

```go
const (
    HighVolumeMatchThreshold    = 10 // 10+ quality matches (4+ stars) = high volume
    TypicalVolumeMatchThreshold = 4  // 4-9 quality matches (4+ stars) = typical volume
                                     // 3 or fewer quality matches (4+ stars) = low volume
)

func determineMatchVolumeCategory(qualityMatches int) string {
    if qualityMatches >= HighVolumeMatchThreshold { 
        return "high" 
    }
    if qualityMatches >= TypicalVolumeMatchThreshold { 
        return "typical" 
    }
    return "low"
}
```

### Timing Configuration

```go
type TrialEmailSettings struct {
    WelcomeDelayDays        int // Day 1
    ExpectationsDelayDays   int // Day 2
    EngagementDelayDays     int // Day 3
    ConversionDelayDays     int // Day 6 (24 hours before expiration)
}
```

### Data Requirements

```go
type TrialEmailData struct {
    *NudgeEmailData // Inherit base fields
    
    // Trial progress
    TrialDay            int
    DaysRemaining       int
    TrialEndDate        time.Time
    
    // Match statistics for volume switching
    TotalJobsAnalyzed   int
    TotalMatches        int
    HighQualityMatches  int    // Score >= 4.0 (used for volume determination)
    FiveStarMatches     int    // Score >= 4.5
    MatchVolumeCategory string // "high", "typical", "low" based on HighQualityMatches
    
    // Engagement metrics
    DashboardVisits     int
    FeedbackGiven       int
    HasUsedFeedback     bool
    LastActivityDate    *time.Time
}
```

### Email Templates Strategy

**All emails use template-based approach** with `text/template`:

1. **Recap Email**: Enhance existing LLM-generated recap with trial context
2. **Welcome Email**: `assets/emails/trial-welcome.html` 
3. **Expectations Email**: `assets/emails/trial-expectations.html` (with match volume switching)
4. **Engagement Email**: `assets/emails/trial-engagement.html`
5. **Conversion Email**: `assets/emails/trial-conversion.html`

**No additional LLM prompts required** beyond enhancing existing recap email prompt.

**Note**: Match volume switching is based on the number of 4+ star rated matches (HighQualityMatches field).

## Success Metrics

### Primary KPIs
- **Subscription Conversion Rate**: % of trial users who subscribe
- **Email Engagement Rate**: Open rates, click rates by email type
- **Platform Engagement**: Dashboard visits, feedback given during trial
- **Time to First Quality Match**: How quickly users get 4+ star matches

### Secondary Metrics
- **Match Quality Scores**: Average scores of matches found
- **User Satisfaction**: Survey responses about trial experience
- **Feature Adoption**: Usage of feedback/tuning system during trial
- **Email Sequence Completion**: How many users receive all 5 emails

## Benefits of 5-Email Approach

### ✅ **User Experience Benefits**
- **Reduced email fatigue**: 5 focused emails vs. 7 touchpoints
- **Clear purpose per email**: Each email has distinct goal
- **Predictable timing**: Simple schedule, no complex conditional logic
- **Consistent voice**: All emails use established Shirely personality

### ✅ **Operational Benefits**
- **Simpler implementation**: Template-based content, no complex LLM integration
- **Easier testing**: Clear A/B test boundaries per email type
- **Maintainable content**: Templates can be updated without code changes
- **Scalable approach**: Foundation for future personalization layers

### ✅ **Business Benefits**
- **Higher engagement**: Focused messages with clear calls-to-action
- **Better conversion**: Builds value systematically toward subscription
- **Quality positioning**: Reinforces "RIGHT job" vs "ANY job" messaging
- **Retention foundation**: Sets proper expectations for long-term success

## Implementation Priority

### Phase 1: Core Infrastructure (Week 1)
1. **Define nudge types** and timing constants
2. **Create email data structures** with match volume detection
3. **Build template switching logic** for match volume categories
4. **Enhance recap email** with trial context

### Phase 2: Template Development (Week 2)
1. **Create 4 new email templates** using Shirely voice guidelines
2. **Implement match volume switching** in expectations email based on 4+ star matches
3. **Test email rendering** across different clients

### Phase 3: Integration & Testing (Week 3)
1. **Integrate with existing nudge system**
2. **Test complete email sequence** with real trial users
3. **Monitor email delivery and engagement**
4. **Refine timing and content** based on initial feedback

### Phase 4: Optimization (Week 4)
1. **A/B test subject lines** and content variations
2. **Analyze conversion metrics** by email type
3. **Implement any needed adjustments**
4. **Document learnings** for future iterations

## Email Sequence Summary

**Final 5-Email Sequence:**

| Day | Email Type | Purpose | Content Approach |
|-----|------------|---------|------------------|
| 0 | **Recap Email** | Personalized call recap + trial intro | LLM-generated (enhanced) |
| 1 | **Welcome to Talent Rodeo** | Set expectations, reduce confusion | Template with Shirely voice |
| 2 | **Why You Might Not See Many Matches Yet** | Prevent disappointment, manage expectations | Template with match volume switching |
| 3 | **3 Tips to Improve Your Matches** | Drive engagement, showcase features | Template with Shirely voice |
| 6 | **Trial Ending Soon – Stay in the Rodeo** | Convert while value is fresh | Template with Shirely voice |

This streamlined approach maintains the core psychological journey while simplifying implementation and reducing email fatigue. The focus on quality positioning and Shirely's unique voice will help differentiate the service while driving meaningful engagement and conversion. 