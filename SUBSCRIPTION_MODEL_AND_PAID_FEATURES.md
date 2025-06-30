# Subscription Model and Paid Features Implementation

**Date:** January 30, 2025

## Overview

This document describes the implementation of our "Freemium Trial" subscription model and how paid features are delivered to users. The system is designed to provide value through intelligent job searching and analysis while maintaining clear distinctions between free trial and paid tiers.

## Business Model: Freemium Trial

### User Journey
1. **Sign Up**: New users are automatically enrolled in a 7-day free trial
2. **Trial Period**: Users get full "pro" benefits for 7 days
3. **Trial Expiration**: After 7 days, users must upgrade to continue receiving benefits
4. **No Persistent Free Tier**: Users who don't upgrade receive no search benefits

### User Tiers
- `UserTierFree`: Users in their 7-day trial period (SubscriptionEnd is in the future)
- `UserTierPro`: Paid users with extended SubscriptionEnd dates
- `UserTierEnterprise`: Future tier for enterprise customers

## Subscription Management Tools

### CLI Management Tools

The CLI provides powerful subscription management tools for administrators:

**View Current Subscription:**
```bash
cli user subscription [user-id]
# Shows current subscription status, remaining days, and user info
```

**Extend Subscription:**
```bash
cli user subscription --extend [days] [user-id]
# Intelligently extends subscription:
# - If subscription is ACTIVE: adds days to current end date
# - If subscription is EXPIRED/not set: adds days to today
```

**Examples:**
```bash
# View current subscription status
cli user subscription user-123
# Output: 
# User: John Doe (user-123)
# Email: john@example.com
# Tier: Free
# Subscription End: 2025-02-06 15:04:05 (ACTIVE - 7 days remaining)

# Extend active subscription by 30 days (extends from current end date)
cli user subscription --extend 30 user-123
# Output: Extended subscription for John Doe (user-123) from 2025-02-06 15:04:05 to 2025-03-08 15:04:05 (+30 days)

# Extend expired subscription by 30 days (extends from today)
cli user subscription --extend 30 user-456
# Output: Extended subscription for Jane Smith (user-456) to 2025-03-01 15:04:05 (30 days from today)
```

### Admin Web Interface

The admin subscription management page provides a user-friendly interface for managing subscriptions:

**Features:**
- View user subscription status (Active/Expired/Not Set)
- Extend subscriptions with intelligent date calculation:
  - **Active subscriptions**: Days are added to the current end date
  - **Expired/unset subscriptions**: Days are added to today's date
- Subscribe/unsubscribe users to search criteria
- View current subscription counts
- Refresh job searches for specific criteria

**Subscription Extension Logic:**
The system intelligently handles subscription extensions to ensure users don't lose time:
- If a user has 5 days remaining and you add 30 days, they get 35 total days
- If a user's subscription expired 3 days ago and you add 30 days, they get 30 days from today

## Core Components

### Search Criteria (`SearchCriteria`)
Defines what we search for and how deep we search:
- Single, canonical criteria per unique search (query + location + work type + experience level)
- No duplication between "free" and "pro" versions
- Multiple users can subscribe to the same criteria

### Search Tasks (`SearchTask`)
Queued jobs that execute the actual searches:
- Created by the search scheduler when criteria need updating
- Contain snapshot of search parameters for the specific run
- Execute with provider-specific settings (pages, listing age, etc.)

### Subscriptions (`SearchCriteriaSubscription`)
Many-to-many relationship between users and search criteria:
- Users subscribe to criteria they're interested in
- Same criteria can have multiple subscribers
- Subscription doesn't change based on user tier

## Implementation Details

### Search Scheduling Logic

The `GetSearchCriteriaNeedsSearch()` function implements subscription-aware filtering:

1. **Get All Subscriptions**: Retrieve all user-criteria relationships
2. **Identify Active Users**: Filter users with `SubscriptionEnd > now`
3. **Find Active Criteria**: Only include criteria with at least one active subscriber
4. **Apply Standard Filters**: Check LastSearchedAt and incomplete tasks

**Key Insight**: A SearchCriteria is only searched if it has paying subscribers or users in trial.

### Publishing Logic

The `subscription.Processor.processCriteria()` function filters at publication time:

1. **Get All Subscribers**: Retrieve users subscribed to the criteria
2. **Filter Active Subscribers**: Only process users with active subscriptions
3. **Skip Expired Users**: No job analysis for users whose trial/subscription ended
4. **Update LastPublishedAt**: Even if no active users, mark as processed

**Key Insight**: Expensive LLM analysis only happens for users who can receive value.

### Race Condition Prevention

The system prevents duplicate search tasks through:
- `CreateSearchTask()` checks for existing active tasks
- Returns `ErrSearchTaskExists` if task already queued/processing
- Scheduler handles this gracefully by logging and continuing

## Data Flow

```
1. User subscribes to SearchCriteria
2. Search Scheduler runs periodically
3. GetSearchCriteriaNeedsSearch() finds criteria with active subscribers
4. SearchTask created and queued to provider
5. Provider executes search, finds jobs
6. Jobs linked to SearchCriteria
7. Subscription Processor runs periodically  
8. processCriteria() analyzes jobs for active subscribers only
9. Job analysis published to active users' UserJob collections
```

## Cost Optimization

### Search Costs
- Only search criteria with paying/trial users
- Avoid expensive API calls for abandoned criteria
- Efficient batching of user lookups

### Analysis Costs  
- LLM job analysis only for active subscribers
- No wasted processing for expired users
- Progress tracking for better UX

## Future Considerations

### Alternative Models Considered

1. **Tier-Aware Publishing**: Always search deep, filter at publication
   - Pro: Seamless upgrades, rich data ready
   - Con: Higher search costs for free users
   - Decision: Rejected in favor of trial model

2. **Separate Pro Criteria**: Duplicate criteria for different tiers
   - Pro: Clean separation
   - Con: Data duplication, complex migrations
   - Decision: Rejected for maintenance complexity

### Potential Enhancements

1. **Dynamic Search Depth**: Adjust search parameters based on subscriber tiers
2. **Granular Feature Gates**: Different features for different tiers
3. **Usage-Based Billing**: Track and limit based on consumption

## Technical Debt and Improvements

### Current Limitations
- All subscribers of a criteria get the same search depth
- No per-user customization of search parameters
- Binary active/inactive subscription model

### Monitoring Points
- Search task creation rates by tier
- Job analysis costs by user tier
- Subscription conversion rates
- System resource utilization

## Testing Considerations

Key test scenarios:
- Mixed subscriber groups (active + expired)
- Criteria with no active subscribers
- User subscription expiration edge cases
- Search task deduplication
- Publishing to filtered user sets

## Implementation Files

### Core Logic
- `internal/search_criteria.go`: Search scheduling logic
- `internal/subscription/subscription_processor.go`: Publishing logic
- `internal/user.go`: User tier and subscription management

### Data Models
- `internal/search_criteria_subscription.go`: Subscription relationships
- `internal/search_task.go`: Search task management
- `internal/user_job.go`: Individual user job storage

This implementation successfully balances providing value to users while maintaining cost discipline and clear upgrade incentives. 