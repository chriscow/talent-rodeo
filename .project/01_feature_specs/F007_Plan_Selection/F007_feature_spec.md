# F007: Plan Selection

## Feature Overview
The Plan Selection feature implements a tiered pricing model for Talent Rodeo, offering Free and Pro plans with different job search capabilities. This feature manages the initial plan selection flow, upgrade path, and subscription management using Stripe for payment processing.

## Implementation Status
- Plan Selection Flow: **Not Implemented**
- Stripe Integration: **Not Implemented** (depends on F005)
- Profile Subscription Management: **Not Implemented**

## Detailed Specifications

### SF007.1: Plan Selection Flow

#### Description
Implementation of the mandatory plan selection process for new users and the upgrade flow for existing Free tier users.

#### Implementation Details
- Add new user state `UserStatusPlanSelection` to handle initial plan selection
- Modify profile handler to show plan selection page for this state
- Store selected plan in User.Tier field
- Implement plan comparison and selection UI using PicoCSS

#### Plan Details
##### Free Plan
- Up to 100 jobs analyzed
- 3 keywords maximum
- Job search every 3 days
- Basic job matching

##### Pro Plan
- Pricing: $20/week or $50/month
- Up to 1000 jobs analyzed
- More than 3 keywords allowed
- Daily job search
- Enhanced job matching

#### UI Implementation
- PicoCSS feature cards for plan comparison
- Plain styling for Free tier card
- Emphasis styling for Pro tier card
- Clear feature comparison list
- "Contrast" button class for upgrade CTA
- Responsive design for all screen sizes

### SF007.2: Profile Subscription Management

#### Description
Implementation of subscription management interface for Pro users and upgrade path for Free users.

#### Implementation Details
- Add subscription section to profile page for Pro users showing:
  - Current plan (Pro)
  - Next billing date
  - Amount
  - Cancel subscription button
- Implement upgrade button in navigation for Free users
- Connect cancellation flow to Stripe UI

#### Subscription Management
- Pro users retain access until current period ends after cancellation
- No automatic downgrade on cancellation
- Subscription end tracking using existing subscription_end_date field

### SF007.3: Integration with F005 Payment System

#### Description
Integration with the F005 Payment Integration feature for handling Pro plan payments and subscription management.

#### Implementation Details
- Utilize Stripe Checkout from F005
- Handle successful payment webhooks
- Update user tier and subscription_end_date
- Manage subscription cancellation through Stripe

## Dependencies
- F005_Payment_Integration for payment processing
- User authentication system
- Database schema updates:
  - New UserStatus for plan selection
  - Existing Tier and subscription_end_date fields

## User Experience Flow
1. New User Flow:
   - User completes initial signup
   - Redirected to plan selection page
   - Selects Free or Pro plan
   - If Pro, completes payment through Stripe
   - Continues to onboarding flow

2. Upgrade Flow:
   - Free user clicks upgrade in navigation
   - Views plan comparison page
   - Selects Pro plan
   - Completes payment through Stripe
   - Returns to previous page with Pro features

3. Pro User Management:
   - Views subscription details in profile
   - Can cancel through Stripe UI
   - Retains Pro access until period ends
   - Automatically reverts to Free tier after expiration

## Testing Requirements
- Verify plan selection flow for new users
- Test upgrade path from Free to Pro
- Validate subscription management UI
- Test cancellation flow
- Verify proper feature access based on plan
- Test subscription expiration handling

## Security Considerations
- Secure storage of plan selection
- Validate all plan changes server-side
- Ensure proper access control for features
- Protect subscription management endpoints

## Error Handling
- Clear error messages for failed payments
- Graceful handling of Stripe API issues
- Proper logging of plan changes
- Monitoring for subscription status changes

## Future Considerations (Post-MVP)
- Additional plan tiers
- Team/Enterprise pricing
- Annual billing options
- Promotional pricing
- Grandfathered pricing protection 