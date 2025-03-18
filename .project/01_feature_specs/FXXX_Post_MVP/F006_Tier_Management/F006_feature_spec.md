# F006: Tier Management

## Feature Overview
The Tier Management feature establishes and enforces tiered subscription levels that determine user access to platform capabilities. This feature enables value differentiation across subscription levels, with enhanced capabilities for paid subscribers, building on the payment infrastructure provided by F005: Payment Integration.

## Implementation Status
- Free Tier: **Implemented**
- Mid Tier: **Partially Implemented**
- Premium Tier: **Partially Implemented**
- Tier Access Control: **Partially Implemented**

## Detailed Specifications

### SF006.1: Free Tier

#### Description
The Free Tier provides basic platform functionality, allowing users to experience core value while encouraging upgrade to paid tiers. This tier includes limited searches, basic document customization, and essential features.

#### Implementation Details
- Limited number of job searches per time period
- Basic resume keyword optimization
- Limited document customization options
- No scheduled follow-up calls
- Limited search result storage duration
- UI indication of tier limitations

#### Current Implementation
- Free tier implementation in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler.go`: Enforces 3-page maximum for job searches
- Limitation enforcement in search algorithm as shown in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler_tier_test.go`
- Database schema for tier storage in candidates table
- Default tier assignment during user creation

### SF006.2: Mid Tier

#### Description
The Mid Tier provides enhanced capabilities for serious job seekers who need more comprehensive support. This includes increased search volume, enhanced document customization, and more frequent check-ins.

#### Implementation Details
- Daily job searches with higher result limits
- Enhanced resume optimization
- Cover letter generation
- Weekly AI check-in calls
- Extended search result storage
- Priority in search queue
- UI indication of tier benefits

#### Current Implementation
- Mid tier search limits implemented in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler.go`: Enforces 50-page maximum
- Tier upgrade tracking in database schema
- Partial implementation of tier-based access controls
- Missing full implementation of all mid-tier benefits

### SF006.3: Premium Tier

#### Description
The Premium Tier provides the highest level of service for candidates actively seeking new opportunities. This includes unlimited access to platform features, priority support, and advanced customization options.

#### Implementation Details
- Unlimited job searches
- Priority search execution
- Advanced document customization
- Bi-weekly AI check-in calls
- Unlimited result storage
- Multiple resume versions
- Interview preparation assistance
- UI indication of tier benefits

#### Current Implementation
- Premium tier search limits implemented in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler.go`: Enforces 200-page maximum
- Tier upgrade tracking in database schema
- Partial implementation of tier-based access controls
- Missing full implementation of all premium-tier benefits

### SF006.4: Tier Access Control

#### Description
The system enforces feature access based on user subscription tier. This component ensures users only access features appropriate for their subscription level, while encouraging upgrades for additional capabilities.

#### Implementation Details
- Feature access control middleware
- Tier-based permission checking
- Upgrade prompts for restricted features
- Tier limitation enforcement
- UI elements for tier-specific features
- Graceful degradation for downgraded users

#### Current Implementation
- Tier-based access control in search functionality implemented in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler.go`
- Tier history tracking in `/workspaces/talent-rodeo/assets/migrations/000013_add_candidates_tier_history.up.sql`
- Implementation of tier downgrade handling as shown in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler_tier_test.go`
- Missing complete middleware for tier-based access across all features

## Dependencies
- F005: Payment Integration - Provides subscription status tracking and payment processing
- User authentication system from F001_User_Onboarding
- Feature access control framework
- Database schema for tier tracking:
  - `candidates` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Contains tier field for current subscription level
  - `candidates_tier_history` table in `/workspaces/talent-rodeo/assets/migrations/000013_add_candidates_tier_history.up.sql` - Tracks tier change history
  - `tier_type` enum in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Defines available tier levels (free, mid, top)

## Feature Dependencies
- Depends on F001_User_Onboarding for user profile data
- Depends on F005_Payment_Integration for subscription management
- Required by all features with tier-based access controls (F003, F004, F007)

## Future Enhancements
- Add team/enterprise tier for recruiting firms
- Implement referral program with tier benefits
- Add specialized tiers for different industries
- Enable gifting of premium access
- Implement promotional trials for higher tiers
- Add tier-specific analytics and insights
- Enable custom tier creation for special cases 