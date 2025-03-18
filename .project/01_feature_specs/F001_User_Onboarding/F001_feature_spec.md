# F001: User Onboarding & Profile Creation

## Feature Overview
The User Onboarding feature establishes the initial connection with users and
collects essential information to create their profile in the Talent Rodeo
system. This feature serves as the entry point for all users and sets the
foundation for subsequent interactions with the platform.

## Implementation Status
- LinkedIn OAuth integration: **Implemented**
- Resume upload functionality: **Implemented**
- Phone verification: **Implemented**
- Basic profile creation: **Implemented**

## Detailed Specifications

### SF001.1: LinkedIn OAuth Integration

#### Description
Users can authenticate into the system using their LinkedIn credentials. This
provides a secure, streamlined login experience while also allowing Talent Rodeo
to access basic profile information from LinkedIn.

#### Implementation Details
- Implemented using the LinkedIn OAuth 2.0 protocol
- Scopes requested: "openid", "profile", "email"
- User data extracted: first name, last name, email, profile picture
- Authentication state preserved in session cookies

#### Current Implementation
- Implementation located in `/workspaces/talent-rodeo/internal/auth/linkedin.go`: Handles OAuth2 protocol for LinkedIn authentication
- Authentication flow managed in `/workspaces/talent-rodeo/cmd/web/handlers.auth.go`: Contains startSocialAuth and completeSocialAuth endpoints for OAuth flow
- Session management: `/workspaces/talent-rodeo/assets/migrations/000003_sessions.up.sql` for database schema
- Authentication tests in `/workspaces/talent-rodeo/cmd/web/handlers.auth_test.go`

### SF001.2: Resume Upload

#### Description
Users can upload their resume in PDF or DOCX format. The system parses and
extracts text from the resume to build the user's initial profile.

#### Implementation Details
- Supported file formats: PDF, DOCX
- File size validation to prevent oversized uploads
- Text extraction from both formats using dedicated parsers
- Resume text stored in the database
- UI provides feedback on successful upload

#### Current Implementation
- Implementation in `/workspaces/talent-rodeo/cmd/web/handlers.resume.go`: Handles file upload, validation, and storage
- File type validation in isValidFileType() function
- Text extraction in `/workspaces/talent-rodeo/experimental/rodeo/resume.go`: Contains ExtractDOCX and ExtractPDF functions
- Resume data stored in users table (resume_filename, full_resume, summarized_resume fields)
- Test implementation in `/workspaces/talent-rodeo/cmd/web/handlers.resume_test.go`

### SF001.3: Phone Verification

#### Description
Users provide their phone number during onboarding for later communication with
the AI assistant.

#### Implementation Details
- Phone number collected via form input
- Basic validation for phone number format
- Phone number stored in user profile
- No current SMS verification flow implemented in MVP nor necessary
- Basic validate that phone number in resume matches the one provided

#### Current Implementation
- Phone collection in `/workspaces/talent-rodeo/cmd/web/handlers.resume.go`: Part of resumeUploadForm struct
- Phone validation in form processing
- Phone number stored in users table (phone field) as defined in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql`

### SF001.4: Basic Profile Creation

#### Description
The system creates a user profile by combining data from LinkedIn authentication and uploaded resume. This profile serves as the foundation for skill extraction and job matching.

#### Implementation Details
- Profile created with LinkedIn details (name, email, etc.)
- Resume text and extracted skills stored in profile
- Profile linked to user's candidate record
- Onboarding state tracking to guide users through the full process

#### Current Implementation
- Profile creation in `/workspaces/talent-rodeo/cmd/web/handlers.auth.go`: Handled in completeSocialAuth function
- Onboarding state management in:
  - `/workspaces/talent-rodeo/internal/onboarding/types.go`: Defines onboarding steps and transitions
  - `/workspaces/talent-rodeo/internal/database/onboarding.go`: Database operations for onboarding state
- Profile data stored in database tables defined in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql`:
  - users table: Stores basic user info including resume and phone
  - candidates table: Stores candidate-specific information
  - onboarding_progress table: Tracks detailed onboarding progress
  - onboarding_feedback table: Stores user feedback on onboarding process

## Dependencies
- Database schema for user profiles:
  - `users` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores core user data including resume, phone, and onboarding state
  - `candidates` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores candidate-specific information
  - `onboarding_progress` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Tracks detailed onboarding steps
  - `sessions` table in `/workspaces/talent-rodeo/assets/migrations/000003_sessions.up.sql` - Manages authentication state
- LinkedIn Developer App credentials (configured in environment)
- File storage system for resumes
- Text extraction libraries for different file formats

## Feature Dependencies
- None - This is a foundational feature that other features depend on

## Future Enhancements
- Add multi-factor authentication options beyond LinkedIn
- Implement phone verification via SMS
- Allow direct import from other job platforms (Indeed, Monster)
- Add resume parsing for additional file formats
- Enable profile editing after initial creation 