# F005: Payment Integration

## Feature Overview
The Payment Integration feature establishes the payment infrastructure for Talent Rodeo, enabling monetization through one-time payments. This feature implements secure payment processing and payment status tracking using Stripe Checkout for a simplified payment experience. For the MVP, users will make one-time payments that grant access to premium features for a 30-day period.

## Implementation Status
- Stripe Integration: **Not Implemented**
- Payment Status Management: **Partially Implemented**
- Premium Features Access: **Partially Implemented**

## Detailed Specifications

### SF005.1: Stripe Integration

#### Description
Integration with Stripe's payment processing platform to handle one-time payments using Stripe Checkout, which provides a pre-built, secure payment form.

#### Implementation Details
- Stripe API configuration and secure key management using environment variables
- One-time payment processing using Stripe Checkout
- Basic customer management in Stripe
- Webhook handling for critical payment events only (successful payments, payment failures)
- Utilize Stripe's built-in receipt email delivery

#### Current Implementation
- Not implemented in current MVP
- Task definitions created in `/workspaces/talent-rodeo/.project/01_feature_specs/F005_Payment_Integration/subfeatures/SF005.1_Stripe_Integration/tasks/`
- Payment service structure planned but not implemented
- Pending implementation of API integration with Stripe

#### Development and Testing Setup
- Create a Stripe account for development
- Set up a test environment in Stripe dashboard
- Configure environment variables in the application:
  ```
  STRIPE_SECRET_KEY=sk_test_...
  STRIPE_PUBLISHABLE_KEY=pk_test_...
  STRIPE_WEBHOOK_SECRET=whsec_...
  STRIPE_MODE=test  // or "production" for production environment
  ```
- Use Stripe CLI for local webhook testing
- Store these configuration values in .env files for local development
- Configure secrets in Fly.io for production deployment

### SF005.2: Payment Status Management

#### Description
System for tracking payment status and access periods to ensure proper access control. For the MVP, this will focus on a binary status (paid/unpaid) tracked through the subscription_end_date field.

#### Implementation Details
- Utilize existing subscription_end_date field to determine access status
- Set subscription_end_date to 30 days from the current date when payment is successful
- Update subscription_end_date when one-time payment is successful via webhook
- Simple payment record keeping for administrative purposes

#### Current Implementation
- Database schema partially implemented in `/workspaces/talent-rodeo/assets/migrations/000012_add_subscription_end_date.up.sql`: Added subscription_end_date to candidates table
- Basic subscription expiration tracking implemented 
- Pending implementation of payment recording

### SF005.3: Premium Features Access

#### Description
Implementation of the access control layer that connects payment status to feature availability, ensuring premium features are only accessible to users with valid paid access.

#### Implementation Details
- Leverage existing tier-based access control (free vs. paid)
- Connect payment status to tier access using subscription_end_date
- Implement upgrade prompts for users on free tier
- Direct users to Stripe Checkout for payment processing
- Set users' tier to 'mid' for 30 days after successful payment

#### Premium Features
- Increased job search capacity (3 pages for free tier, 50 pages for paid tier)
- More frequent automated searches (every 3 days for free tier, daily for paid tier)
- Email notifications for new job matches (paid tier only)

#### Current Implementation
- Basic tier-based access implemented in various handlers (tier restrictions for job searches)
- Database support for subscription end date in candidates table
- Feature gating based on subscription status partially implemented
- Integration with payment system pending

## Dependencies
- User authentication system (implemented in F001_User_Onboarding)
- Database schema for payment tracking:
  - `candidates` table in `/workspaces/talent-rodeo/assets/migrations/000012_add_subscription_end_date.up.sql`: Added subscription_end_date
  - Need simple `payments` table for record-keeping (not yet implemented)
- Email notification system
- Secure HTTPS implementation

## Feature Dependencies
- Depends on F001_User_Onboarding for user authentication and profile data
- Depends on F006_Tier_Management for tier-based feature access controls
- Required for premium features across multiple features

## Security Considerations
- All payment processing must occur on Stripe's servers using Stripe Checkout
- API keys must be stored securely as environment variables, never in code
- All payment-related endpoints must be secured with HTTPS
- Validate webhook signatures to prevent fraudulent events

## Error Handling
- Clearly communicate payment failures to users
- Log all payment attempts for debugging purposes
- Provide users with clear next steps when payments fail
- Implement basic monitoring for payment failures

## User Experience Flow
1. User reaches feature limit or clicks upgrade prompt
2. User is directed to payment selection page
3. User selects paid tier and initiates payment
4. Stripe Checkout opens for secure payment processing
5. Upon successful payment, user is redirected back to application
6. User's account is updated with premium status and new subscription_end_date (30 days from now)
7. User receives confirmation email from Stripe

## Testing Requirements
- Test payment flow with Stripe test cards
- Verify webhook handling for successful and failed payments
- Validate feature access based on payment status
- Test subscription expiration and access removal

## Future Enhancements (Post-MVP)
- Support for recurring subscription payments (instead of one-time payments)
- Support for additional payment methods (PayPal, Apple Pay, etc.)
- Implementation of coupon/promotion system
- Advanced revenue analytics
- Automated invoicing
- Tax calculation and reporting
- Affiliate/referral payment tracking
- Enterprise billing options 