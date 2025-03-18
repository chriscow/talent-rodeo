# Wireframe Overview

This document provides a description of key screens in the Talent Rodeo application and their purpose in the user journey.

## 1. Home/Landing Page

**Purpose:** Introduce the platform and provide authentication entry point.

**Key Elements:**
- Hero section with platform value proposition
- "Continue with LinkedIn" button for authentication
- Benefits overview section
- Tier comparison information
- Footer with legal information

**User Actions:**
- Authenticate with LinkedIn
- Learn about platform benefits
- Compare service tiers

## 2. Resume Upload Screen

**Purpose:** Collect user resume and phone number to begin onboarding.

**Key Elements:**
- Welcome header
- Clear instructions
- File upload area with drag-and-drop support
- Phone number input field with validation
- Progress indicator showing onboarding steps
- Submit button

**User Actions:**
- Upload resume (PDF/DOCX)
- Enter phone number
- Submit information
- View processing status

## 3. Schedule Phone Call Screen

**Purpose:** Allow user to schedule initial assessment call.

**Key Elements:**
- Scheduling instructions
- Calendar interface showing available slots
- Timezone selector/indicator
- Time slot list with selection capability
- Confirmation button
- Contact preferences section (optional)

**User Actions:**
- Select date on calendar
- Choose available time slot
- Set timezone preferences
- Confirm appointment
- Set contact preferences

## 4. Waiting for Call Screen

**Purpose:** Provide information about upcoming call and status.

**Key Elements:**
- Countdown timer to scheduled call
- Call preparation tips
- Calendar add buttons (Google/Apple/Outlook)
- Reschedule option
- What to expect section

**User Actions:**
- Add call to calendar
- Reschedule if needed
- Prepare for call using provided information

## 5. Preferences Screen

**Purpose:** Confirm and refine job search preferences.

**Key Elements:**
- Form sections for different preference categories:
  - Job titles
  - Industries
  - Location preferences
  - Salary range
  - Remote/hybrid/on-site options
  - Experience level
- Pre-filled information from resume and call
- Edit controls for each section
- Save button
- Next steps information

**User Actions:**
- Review extracted preferences
- Edit and refine preferences
- Add additional preferences
- Save and continue

## 6. Job Search Progress Screen

**Purpose:** Display status during asynchronous job search.

**Key Elements:**
- Progress bar
- Status messages
- Estimated completion time
- Animation indicating activity
- Information about what's happening

**User Actions:**
- Monitor search progress
- Wait for completion
- Cancel search (optional)

## 7. Job Matches Screen

**Purpose:** Display and interact with job search results.

**Key Elements:**
- Results summary header
- Filter and sort controls
- Job cards with:
  - Job title
  - Company
  - Location
  - Match percentage
  - Key requirements
  - Posted date
- Pagination controls
- Search refinement options

**User Actions:**
- Browse job matches
- Filter and sort results
- Click on jobs for details
- Refine search parameters
- Select jobs for application

## 8. Job Detail Screen

**Purpose:** Provide comprehensive information about a specific job.

**Key Elements:**
- Complete job description
- Company information
- Salary details (if available)
- Required skills with match indicators
- Apply button
- Customize resume/cover letter options
- Back to results link
- Similar jobs suggestions

**User Actions:**
- Read full job details
- Request document customization
- Apply to job
- Return to results
- Explore similar jobs

## 9. Interview Practice Screen

**Purpose:** Provide mock interview capabilities.

**Key Elements:**
- Practice modes (general/job-specific)
- Question display area
- Audio recording controls
- Feedback section
- Response timer
- Progress through question set
- Performance metrics

**User Actions:**
- Select practice mode
- Record responses to questions
- Review feedback
- Track performance
- Complete practice sessions

## 10. Skills Management Screen

**Purpose:** View and edit skills profile.

**Key Elements:**
- Skills list with categorization
- Add/edit/remove controls
- Drag-and-drop for prioritization
- Skill match indicators
- Suggestions for additional skills
- Save button

**User Actions:**
- Review extracted skills
- Add missing skills
- Remove irrelevant skills
- Prioritize key skills
- Save changes

## 11. Tier Management Screen

**Purpose:** View and upgrade service tier.

**Key Elements:**
- Current tier indicator
- Feature comparison table
- Upgrade buttons
- Usage statistics
- Benefits of upgrading section
- Payment information form (when upgrading)

**User Actions:**
- View current limitations
- Compare tier benefits
- Upgrade tier
- Enter payment information
- Confirm upgrade

## 12. Admin Task Management Screen

**Purpose:** Allow administrators to monitor and manage background tasks.

**Key Elements:**
- Task listing with:
  - Task ID
  - Type
  - Status
  - Created/Updated timestamps
  - Error messages (if applicable)
- Filter and search controls
- Retry buttons for failed tasks
- Task details expansion
- Pagination controls

**User Actions:**
- Review task status
- Filter tasks by type/status
- Retry failed tasks
- View detailed task information

## 13. Admin User Management Screen

**Purpose:** Allow administrators to manage user data.

**Key Elements:**
- User search controls
- User listing
- Profile information
- Keywords editor
- Activity timeline
- Support action buttons

**User Actions:**
- Search for users
- View user profiles
- Edit user keywords
- View user activity
- Perform support actions

## Implementation Notes

1. **Responsive Considerations:**
   - All screens should adapt to desktop, tablet, and mobile viewports
   - Mobile views may combine or simplify certain elements
   - Touch-friendly controls for mobile users

2. **Accessibility Requirements:**
   - All interactive elements must be keyboard accessible
   - Color contrast must meet WCAG standards
   - Screen reader compatibility required
   - Form labels and ARIA attributes properly implemented

3. **State Management:**
   - All forms should preserve state during navigation
   - Error states should be clearly indicated
   - Loading states should be visually distinct
   - Success confirmations should be prominent

4. **Implementation Priority:**
   - Core onboarding flow (screens 1-7) is highest priority
   - Skills management is medium priority
   - Admin screens are lower priority for MVP

5. **Technical Constraints:**
   - Templates must be compatible with Go's html/template package
   - Client-side interactivity through minimal JavaScript
   - HTMX for dynamic content updates
   - AlpineJS for complex client-side interactions 