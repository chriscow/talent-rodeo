# Talent Rodeo MVP Specification

## Executive Summary

Talent Rodeo is an AI-powered job matching platform that helps job seekers find opportunities aligned with their skills, experience, and preferences. The MVP will focus on core functionality that delivers immediate value to job seekers while establishing the foundation for future feature expansion.

## Product Vision

Talent Rodeo helps job seekers navigate the employment market through AI-driven conversations, intelligent job matching, and personalized document optimization. The platform makes the job search process more efficient, effective, and tailored to individual needs.

## MVP Scope

The initial release will include essential features necessary to validate the core value proposition. This includes user onboarding, conversational assessment, job search and matching, document customization, and a tiered subscription model with payment integration for monetization.

## Core Feature Set

### 1. User Onboarding & Profile Creation

- **LinkedIn OAuth Integration**: Uses login via LinkedIn OAuth button
- **Resume Upload**: Process and analyze uploaded resumes in standard formats (PDF, DOCX)
- **Phone Verification**: Validate user phone number for future interactions
- **Basic Profile Creation**: Extract key information from resume and LinkedIn profile

### 2. Initial Assessment Call

- **Automated Outbound Calling**: Schedule and conduct initial assessment call
- **Structured Conversation**: Follow a predefined conversation framework to gather information
- **Information Extraction**: Identify key skills, experiences, and preferences
- **Call Recording Management**: Store and process conversation recordings with proper consent
- **Error Handling**: Store and track errors during transcript processing for admin review
- **Transcript Analysis**: Process and analyze call transcripts to refine search criteria
- **Search Criteria Generation**: Convert conversation insights into structured search parameters

### 3. Job Search

- **LinkedIn Job Data Integration**: Utilize ScrapingDog to access LinkedIn job listings
- **Skill-Based Search**: Leverage extracted skills to perform targeted job searches
- **Search Result Storage**: Save job search results in database for efficient retrieval and matching
- **Background Processing**: Automated background service to perform periodic job searches
- **Error Recovery**: Track and handle search errors with admin notifications
- **Search Frequency Management**: Control search timing based on user tier level

### 4. Job Matching

- **Preference Filtering**: Apply filters for compensation, location, and work arrangement
- **Match Quality Assessment**: Calculate and display match percentage for each job
- **Match Result Display**: Present job matches in an intuitive, actionable format

### 5. Document Customization

- **Resume Keyword Optimization**: Align resume terminology with job description
- **Cover Letter Generation**: Create basic cover letters tailored to specific job opportunities
- **Format Preservation**: Maintain professional formatting and structure
- **Document Download**: Provide downloadable versions of customized documents

### 6. Payment Integration

- **Stripe Integration**: Integration with Stripe's payment processing platform
- **Payment Status Management**: Track and manage payment status and subscription lifecycle
- **Premium Features Access**: Control access to features based on payment status

### 7. Tier Management

- **Free Tier**: Limited searches, basic customization
- **Mid Tier**: Daily searches, enhanced customization, more frequent check-ins
- **Premium Tier**: Priority searches, unlimited customization, premium support
- **Tier Access Control**: Enforce feature access based on subscription tier

## User Experience Flow

1. **Initial Registration**: User visits site and authenticates via "Continue with LinkedIn"
2. **Profile Completion**: User uploads resume and provides phone number
3. **Assessment Call**: AI conducts phone conversation to understand user needs
4. **Profile Review**: User reviews and confirms extracted information
5. **Job Search**: System conducts initial search based on profile
6. **Match Review**: User explores and interacts with job matches
7. **Document Customization**: User requests customized documents for selected opportunities
8. **Tier Selection**: User selects a subscription tier based on their needs
9. **Payment Processing**: User completes payment for paid tiers
10. **Ongoing Engagement**: System schedules follow-up calls based on tier level

## Technical Architecture

### Frontend

- **Web Application**: Plain HTML/JS as foundation with progressive enhancement approach
- **Dynamic Interactions**: HTMX for specific interactive elements requiring server communication
- **Client Interactivity**: AlpineJS only where necessary for complex client-side interactions
- **Styling**: PicoCSS for lightweight, responsive interface
- **Template Rendering**: Go's html/template package for server-side rendering

### Backend

- **Go Services**: Core application logic implemented in Go
- **Flow Router**: Request routing with nested middleware groups
- **PostgreSQL Database**: Structured data storage for user profiles and job information
- **NATS Messaging**: Asynchronous task processing, distributed processing, load balancing, and key-value store
- **Error Management**: Simple MVP error tracking and admin notification system
- **Background Services**: Automated job search processing based on user tiers

### AI Integration

- **LLM Provider**: Integration with selected LLM API for conversational and document tasks
- **Voice Processing**: Integration with voice-to-text and text-to-voice services
- **Minds Library**: Utilization of existing Minds framework for LLM handling
- **Transcript Analysis**: Processing of conversation transcripts for search criteria generation
- **Error Recovery**: Graceful handling of AI processing failures with error tracking

### Integration Points

- **LinkedIn OAuth API**: For authentication and basic profile information
- **ScrapingDog + LinkedIn**: For job search data acquisition
- **Telephony Services**: For automated calling features
- **Stripe API**: For payment processing and subscription management

## Security & Privacy Measures

- **Data Encryption**: Encrypt sensitive user data at rest and in transit
- **Authentication**: LinkedIn OAuth for secure authentication, delegating MFA and account security to LinkedIn
- **Consent Management**: Clear opt-in processes for data usage
- **Access Controls**: Role-based access control for system functions
- **Payment Security**: PCI-compliant payment processing through Stripe

## MVP Success Metrics

- **User Acquisition**: 500+ active users within first 3 months
- **Conversion Rate**: 8%+ free-to-paid conversion rate
- **Job Match Quality**: 70%+ user satisfaction with job matches
- **Document Effectiveness**: 20%+ improvement in interview rates with customized documents
- **Retention**: 60%+ monthly active user retention

## Development Approach

- **Agile Development**: Two-week sprints with defined deliverables
- **Continuous Integration**: Automated testing and deployment pipeline
- **Incremental Delivery**: Phased implementation prioritizing core functionality
- **User Testing**: Early access program for feedback collection
- **Working Code**: Maintain shippable, working code at all times
- **Error Handling**: Simple but effective error tracking and recovery mechanisms
- **Admin Tools**: Basic admin notification system for error monitoring

## Deployment Timeline

- **Phase 1 (Weeks 1-4)**: Core backend services, database schema, authentication
- **Phase 2 (Weeks 5-8)**: Conversation framework, resume processing
- **Phase 3 (Weeks 9-12)**: Job search integration, matching algorithm
- **Phase 4 (Weeks 13-16)**: Document customization, payment integration, tier management
- **Phase 5 (Weeks 17-20)**: Testing, refinement, and launch preparation

## Future Considerations (Post-MVP)

- **Recruiter Portal**: Allow recruiters to search for candidates
- **Advanced Interview Preparation**: Mock interviews and coaching
- **Application Tracking**: Comprehensive application lifecycle management
- **Career Development Tools**: Skill gap analysis and learning recommendations
- **Additional Payment Methods**: Support for PayPal, Apple Pay, etc.
- **Multi-language Support**: Support for international candidates
- **Advanced Analytics**: Enhanced reporting and insights

## Resource Requirements

- **Development Team**: 2 backend engineers, 1 frontend developer, 1 AI specialist
- **Infrastructure**: Cloud hosting with autoscaling capabilities
- **External Services**: LLM API access, voice services, job board API subscriptions, payment processing
- **Support**: Customer support infrastructure for tier-appropriate assistance

** We want to follow generally agile practices. We want to always have
shippable, working code.  Each feature needs to be complete with UI, tests with
a path forward when we implement future features. Every specification and task 
should note this.