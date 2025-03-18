# User Journey Map

This document outlines the complete user journey through the Talent Rodeo platform, from initial authentication to job matching and document customization.

## Overview

Talent Rodeo guides users through a structured process to optimize their job search experience. The journey is designed to gather necessary information progressively while providing value at each step.

## Onboarding States

The platform uses a state-based approach to track user progress through the onboarding flow:

1. **StepInitial**: User has authenticated but has not uploaded a resume
2. **StepResumeUploaded**: Resume uploaded, waiting for phone call scheduling
3. **StepPhoneCallScheduled**: Phone call scheduled, waiting for call
4. **StepPhoneCallCompleted**: Call completed, waiting for preferences setup
5. **StepPreferencesExtracted**: Preferences set, ready for job search
6. **StepJobSearchStarted**: Job search initiated, processing
7. **StepJobSearchCompleted**: Job search completed, viewing results

## Detailed User Flow

### 1. Authentication

**Entry Point: /** (Home page)
- User arrives at the home page
- Clicks "Continue with LinkedIn" button
- Flow:
  - `/auth/linkedin` - Initiates OAuth flow
  - `/auth/linkedin/callback` - Completes authentication
  - Redirects to `/profile`

**Templates:**
- `partials/login.html`

**User Experience:**
- Simple, recognizable social login
- No manual form filling required
- Leverages existing LinkedIn profile

### 2. Resume Upload

**Entry Point: `/profile`** (Onboarding state: StepInitial)
- System detects new user status
- Displays resume upload interface
- User uploads resume and provides phone number
- System extracts keywords from resume

**Templates:**
- `pages/profile-resume-upload.html`
- `partials/resume-form.html`

**User Experience:**
- Clear instruction on next steps
- Simple drag-and-drop or file selection
- Real-time status updates during processing
- Progress indicators for multi-step journey

### 3. Phone Call Scheduling

**Entry Point: `/profile`** (Onboarding state: StepResumeUploaded)
- System presents scheduling calendar
- User selects preferred date and time
- System confirms appointment

**Templates:**
- `pages/profile-schedule-phone-call.html`
- `partials/call-status.html`

**User Experience:**
- Visual calendar interface
- Clear time slot availability
- Timezone detection and adjustment
- Confirmation messaging
- Option to reschedule if needed

### 4. Waiting for Phone Call

**Entry Point: `/profile`** (Onboarding state: StepPhoneCallScheduled)
- System displays countdown to scheduled call
- User receives call reminders
- System updates status after call completion

**Templates:**
- `pages/profile-waiting-call.html`
- `partials/waiting-for-call.html`

**User Experience:**
- Clear countdown timer
- Call preparation information
- Option to reschedule
- Real-time status updates

### 5. Preferences Setup

**Entry Point: `/profile`** (Onboarding state: StepPhoneCallCompleted)
- System presents preferences form
- User confirms or adjusts extracted preferences
- User adds additional job search criteria

**Templates:**
- `pages/profile-preferences.html`

**User Experience:**
- Pre-filled form from call and resume data
- Intuitive controls for adjustments
- Clear sections for different preference types
- Guidance on optimizing search parameters

### 6. Job Search Initiation

**Entry Point: `/profile`** (Onboarding state: StepPreferencesExtracted)
- System presents job search options
- User initiates search
- System displays progress indicators

**Templates:**
- `pages/search-progress.html`
- Job search is processed asynchronously
  
**User Experience:**
- Clear search button
- Progress indicators during search
- Estimated completion time
- Real-time updates on search status

### 7. Job Match Review

**Entry Point: `/profile`** (Onboarding state: StepJobSearchCompleted)
- System presents job match results
- User browses and filters matches
- User selects jobs for further action

**Templates:**
- `pages/profile-jobs.html`
- `partials/job-search-results.html`
- `pages/job-detail.html`

**User Experience:**
- Match quality indicators
- Sorting and filtering options
- Detailed job information
- Action buttons for next steps
- Option to refine search parameters

### 8. Interview Preparation

**Entry Point: `/interview`**
- System provides interview preparation tools
- User engages with mock interviews
- System provides feedback and coaching

**Templates:**
- `pages/interview.html`

**User Experience:**
- Practice question sets
- Audio recording capabilities
- Real-time feedback
- Performance metrics
- Improvement suggestions

### 9. Document Customization

**Entry Point: Job detail pages**
- User requests document customization
- System generates tailored resume and cover letter
- User downloads or directly applies with documents

**Templates:**
- Document customization UI (not yet implemented)

**User Experience:**
- Simple customization request flow
- Preview of tailored documents
- Download options
- Direct application capabilities

## User Interactions

### Skills Management

**Entry Point: Accessible throughout the journey**
- User can view extracted skills
- User can add, edit, or remove skills
- System incorporates updated skills into job matching

**Templates:**
- `partials/skills.html`
- `partials/skill-item.html`

**User Experience:**
- Inline editing capabilities
- Drag-and-drop prioritization
- Skill category grouping
- Suggestions for missing skills

### Tier Management

**Accessible throughout the journey**
- User can view current tier and limitations
- User can upgrade to access premium features
- System adjusts capabilities based on tier

**User Experience:**
- Clear tier comparison
- Seamless upgrade flow
- Feature unlocking animations
- Usage tracking

## Administrative Functions

### Task Management

**Entry Point: `/admin/tasks`**
- Administrators can view and manage background tasks
- System provides diagnostics and controls

**Templates:**
- `partials/tasks-table.html`

**User Experience (Admin):**
- Task listing with status
- Filtering and sorting options
- Retry controls for failed tasks
- Performance metrics

### User Management

**Entry Point: `/user/:id/keywords`**
- Administrators can edit user keywords
- System provides user profile management

**User Experience (Admin):**
- User search capabilities
- Profile editing interface
- Activity tracking
- Support action tools

## Responsive Design

The interface is designed to function across various devices:
- Desktop: Full-featured experience with advanced controls
- Tablet: Optimized layouts for medium screens
- Mobile: Essential functionality with simplified controls

## Accessibility Considerations

- High contrast mode support
- Keyboard navigation
- Screen reader compatibility
- Focus management for form elements
- Appropriate ARIA attributes throughout

## Key UI Components

1. **Navigation Bar** (`partials/nav.html`)
   - Site navigation
   - Authentication status
   - User profile access

2. **Status Updates**
   - Real-time feedback system
   - Toast notifications
   - Progress indicators

3. **Forms**
   - Consistent validation patterns
   - Inline help text
   - Error handling
   - Autosave capabilities

4. **Job Listings**
   - Consistent card layout
   - Quick action buttons
   - Match quality indicators
   - Detail expansion

## Implementation Files

| User Journey Stage | Handler Files | Template Files |
|-------------------|---------------|----------------|
| Authentication | `handlers.auth.go` | `partials/login.html` |
| Resume Upload | `handlers.resume.go` | `pages/profile-resume-upload.html`, `partials/resume-form.html` |
| Phone Call Scheduling | `handlers.schedule.go` | `pages/profile-schedule-phone-call.html` |
| Waiting for Call | `handlers.voice.go` | `pages/profile-waiting-call.html`, `partials/waiting-for-call.html` |
| Preferences Setup | `handlers.preferences.go` | `pages/profile-preferences.html` |
| Job Search | `handlers.search.go` | `pages/search-progress.html` |
| Job Matches | `handlers.search.go` | `pages/profile-jobs.html`, `partials/job-search-results.html` |
| Interview Preparation | `handlers.interview.go` | `pages/interview.html` |
| Skills Management | `handlers.skills.go` | `partials/skills.html`, `partials/skill-item.html` | 