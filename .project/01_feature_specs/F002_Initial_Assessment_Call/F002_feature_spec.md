# F002: Initial Assessment Call

## Feature Overview
The Initial Assessment Call feature enables Talent Rodeo to conduct an automated phone conversation with candidates to gather information about their skills, experiences, and job preferences. This conversation is structured to extract meaningful data that can be used for subsequent job matching and document customization.

This feature serves as a key step in the candidate onboarding process, bridging the gap between initial registration and meaningful job matching. Once a candidate completes their basic profile, they are prompted to schedule an assessment call.

## Business Value
- **Scalability**: Enables processing of large numbers of candidates without proportional increase in human resources
- **Data Quality**: Collects richer, more structured candidate information than forms or resume parsing alone
- **Candidate Experience**: Provides a convenient, personalized experience that respects candidates' time
- **Efficiency**: Automates initial screening to allow recruiters to focus on qualified candidates
- **Consistency**: Ensures all candidates receive the same quality of initial assessment
- **Insights**: Generates valuable data on candidate qualifications and market trends

## User Experience
From the candidate's perspective, the Initial Assessment Call feature consists of these key steps:

1. After completing basic profile creation, candidates are prompted to schedule an assessment call
2. Candidates select a convenient time from available slots in an intuitive scheduling interface
3. At the scheduled time, they receive an automated call from the system
4. The call begins with a clear introduction explaining the purpose and process
5. The system engages in a natural conversation about their skills, experience, and preferences
6. After the call concludes, candidates receive a summary of the information collected
7. Candidates can review, confirm, or edit the information extracted from the call
8. They are presented with initial job matches based on their enhanced profile

## Implementation Status
- Automated Outbound Calling: **Mostly Implemented** - Twilio client interface, call scheduling, and WebSocket handling exist
- Structured Conversation: **Partially Implemented** - Advanced conversation middleware, LLM integration, and role management are available
- Information Extraction: **Partially Implemented** - Database schema and basic preference tracking exist
- Call Recording Management: **Not Implemented** - Database schema is in place with fields for recording data
- Candidate Profile Enhancement: **Partially Implemented** - Basic data structures and preliminary enhancement algorithms exist
- Feedback and Engagement: **Not Implemented** - Framework for email notifications exists

## Detailed Specifications

### SF002.1: Automated Outbound Calling

#### Description
The system schedules and initiates phone calls to candidates who have completed their profile creation and provided a phone number. These calls are made at a time convenient for the candidate and deliver an AI-driven conversation experience.

#### Implementation Details
- Call scheduling interface for users to select preferred time
- Integration with telephony services
- Call initiation with appropriate caller ID
- Call state tracking in database

#### Current Implementation
- User interface for scheduling in `/workspaces/talent-rodeo/cmd/web/handlers.schedule.go`: Implements scheduleFormData struct and scheduling functionality
- Appointment management functions in `/workspaces/talent-rodeo/cmd/web/handlers.schedule.go`: showSchedulePage, processScheduleForm, etc.
- API endpoints for scheduling in `/workspaces/talent-rodeo/cmd/web/routes.go`
- Tests in `/workspaces/talent-rodeo/cmd/web/handlers.schedule_test.go`
- Twilio integration implemented in `/internal/tasks/handlers/twilio_client.go`
- WebSocket handling for voice communication implemented in `/internal/handlers/voice.go`

### SF002.2: Structured Conversation

#### Description
The system conducts a natural conversation following a predefined framework designed to gather relevant information while providing a positive user experience. The conversation leverages LLMs to create a natural interaction flow.

The conversation is structured into distinct phases with a configurable number of turns per phase. The system transitions gracefully between topics when a phase has reached its allocated turns or when all required information has been gathered. Each phase focuses on gathering specific information:

1. **Introduction & Rapport Building**: Building initial connection and getting a broad overview of the candidate's career journey
2. **Work Experience Exploration**: Detailed discussion of each work experience, including responsibilities, challenges, and skills used
3. **Skills Assessment**: Identifying skills the candidate wants and doesn't want to use in their next role, with proficiency evaluation
4. **Preference Collection**: Gathering job preferences including salary expectations, non-negotiables, location preferences, and more

For each conversation phase, the system uses specially crafted prompts that guide the AI to elicit the required information while maintaining a natural, engaging dialogue.

#### Implementation Details
- Conversation flow defined in prompts with phase-specific guidance
- Turn tracking and phase transition management
- Dynamic adaptation based on resume content
- Turn-based dialogue management
- Fallback mechanisms for unclear responses
- Specific information extraction targets for each phase:
  - **Work Experiences**: Role details, responsibilities, challenges, and skills used
  - **Skills Evaluation**: Desired skills, unwanted skills, and proficiency levels
  - **Job Preferences**: Salary expectations, non-negotiables, work location preferences, industry preferences, company size preferences, culture preferences, and other preferences

#### Current Implementation
- Conversation handling in `/workspaces/talent-rodeo/cmd/evals/conversations/conversations.go`: Contains middleware-based approach for managing conversations
- Role management in `/workspaces/talent-rodeo/cmd/evals/conversations/roles.go`: Implements role tracking and management
- Conversation summarization in `/workspaces/talent-rodeo/cmd/evals/conversations/summarizer.go`: Provides context maintenance for long conversations
- Logging and persistence in `/workspaces/talent-rodeo/cmd/evals/conversations/logger.go` and `/workspaces/talent-rodeo/cmd/evals/conversations/persistence.go`: Implements conversation tracking
- Prompt templates in `/workspaces/talent-rodeo/assets/prompts/`: Contains templates for various conversation scenarios (e.g., recruiter.system.005.txt)
- Voice response handling implemented in `/cmd/web/handlers.voice.go`
- Turn tracking and phase transition mechanisms in development with state machine approach

#### Planned Implementation
- Conversation middleware to track turns per phase and manage transitions (building on patterns from `/cmd/evals/conversations/`)
- Phase-specific prompt templates:
  - Introduction prompt (based on recruiter.system.005.txt)
  - Work experience exploration prompt
  - Skills assessment prompt
  - Job preferences collection prompt
- Graceful topic transition handling between phases
- Adaptive conversation flow based on remaining information needs
- Comprehensive information extraction and storage schema

### SF002.3: Information Extraction

#### Description
We have an LLM based agent call the candidate in order to gather additional
information that is not easily gathered from the resume. During the
conversation, the system identifies and extracts key information about the
candidate's skills, experiences, and job preferences. This extracted information
is structured and stored in the candidate's profile.

The system captures specific types of information from different conversation phases.
Each phase utilizes a different prompt to guide the LLM to extract the information.

1. **Work Experience Information**:
   - Role details and responsibilities
   - Challenges faced and how they were addressed
   - Technologies and methodologies used
   - Skills demonstrated in each role
   - Achievements and outcomes

2. **Skills Assessment**:
   - Skills the candidate wants to use in their next role
   - Skills the candidate does not want to use
   - Proficiency level for each desired skill (assessed through direct questions and indirect evaluation)
   - Examples of skill application from past experiences

3. **Job Preferences**:
   - Salary expectations (range or specific amounts)
   - Non-negotiable requirements
   - Work location preferences (remote, hybrid, on-site)
   - Geographical location preferences if applicable
   - Industry preferences or industries to avoid
   - Company size preferences
   - Culture and work environment preferences
   - Other job-specific preferences

The system not only records explicit statements but also analyzes the
conversation to derive implicit preferences and skill levels based on the depth
and quality of responses.

#### Implementation Details
- Real-time processing of conversation
- Asynchronous LLM analysis for recognition of skills and preferences via the minds library 
  (see `/cmd/evals/conversations/*.go` for example middleware implementation)
- Classification of information into categories
- Confidence scoring of extracted information
- Structured data schema for consistent storage of:
  - Skills with proficiency levels
  - Work experiences with detailed attributes
  - Job preferences with priority indicators

#### Current Implementation
- Preference tracking implemented in `/workspaces/talent-rodeo/cmd/web/handlers.preferences.go`: Functions for showing and updating user preferences
- Job search criteria storage in `/workspaces/talent-rodeo/assets/migrations/000006_add_scheduling_tables.up.sql`: Schema for storing extracted job search criteria
- Data storage interfaces in `/workspaces/talent-rodeo/internal/database/`: Contains handlers for accessing and storing candidate data
- Pattern recognition for skills and preferences partially implemented

### SF002.4: Call Recording Management

#### Description
With proper consent, the system records and stores conversation audio for quality assurance, training, and further analysis. These recordings are securely managed and accessible only to authorized personnel. The system also handles the transcription of audio to text for subsequent analysis.

#### Implementation Details
- Audio recording capability for both parties during the phone call
- Secure storage with encryption for audio files
- Appropriate consent mechanisms and disclosures
- Speech-to-text transcription of recorded conversations
- Access controls and audit trails for recording access
- Retention policies and automatic purging

#### Current Implementation
- Not implemented in current MVP
- Database schema in place in `/workspaces/talent-rodeo/assets/migrations/000006_add_scheduling_tables.up.sql`: appointments table has fields for call_sid, transcript, and transcript_summary
- Pending implementation of actual recording functionality
- Sample implementation available from product owner for reference

### SF002.5: Candidate Profile Enhancement

#### Description
This subfeature transforms extracted information from assessment calls into a comprehensive, structured candidate profile. It includes processes for data enrichment, validation, categorization, and prioritization to create an accurate profile for job matching.

#### Implementation Details
- Integration of data from multiple sources (resume, call transcript, user input)
- Profile data enrichment with standardized information
- Validation of profile information for consistency and accuracy
- Career insights generation based on collected data
- Profile summarization for quick reference
- Confidence scoring for profile elements

#### Current Implementation
- Partially implemented
- Basic data structures for candidate profiles exist in the database schema
- Enhancement algorithms in development for integrating extracted call information
- User interface for profile review needs implementation
- Confidence scoring system for extracted information planned

### SF002.6: Post-Call Processing and Preferences Display

#### Description
After the assessment call concludes, the system processes the transcript to extract skills and preferences, updates the candidate's profile, and provides a UI for the candidate to review and adjust the extracted information.

The assessment call will end with the LLM telling the user that their information will be processed and their preferences page will be updated. When a user navigates to the preferences page while processing is still ongoing, they are redirected to a "processing" page that displays the current status of the transcript analysis. Once processing is complete, they are automatically redirected to the preferences page where they can review and adjust the extracted preferences.

The preferences page displays the extracted information in a user-friendly format, allowing candidates to make additional adjustments before proceeding with job matching. Some inferred preferences that influence job matching may not be directly editable as they are derived from complex analysis of the conversation.

#### Implementation Details
- Post-call transcript processing workflow
- Processing status tracking and display
- Redirect logic for handling in-progress processing
- Preferences page UI for displaying and editing extracted information
- Clear distinction between editable and non-editable (inferred) preferences
- Integration with job search initiation process

#### Current Implementation
- Basic preferences display page exists at `assets/templates/pages/profile-preferences.html`
- The page currently displays skills and salary preferences with HTMX-enabled editing
- Backend handlers for preferences in `/workspaces/talent-rodeo/cmd/web/handlers.preferences.go`
- Basic structure for job search initiation from the preferences page
- Additional preference types (work location, company size, etc.) exist in commented HTML
- Various transcript processing handlers exist in `/workspaces/talent-rodeo/internal/handlers/`

#### Planned Implementation
- Create a processing status page template
- Implement processing status tracking in the database
- Develop handler for redirecting to processing status page when needed
- Enhance preferences display to include all relevant extracted information
- Add clear explanations for non-editable inferred preferences
- Implement automatic redirection from processing page to preferences page when complete

## Security and Privacy Considerations
- All calls are recorded with explicit candidate consent
- Call data is encrypted in transit and at rest
- Candidates have full transparency into what information is collected
- Profile data is subject to the platform's privacy policy and data retention rules
- Candidates can request deletion of call recordings and transcripts

## Testing Strategy
Testing for this feature includes:
- Unit tests for individual components and services
- Integration tests for the complete call flow
- Load testing for the scheduling and call handling systems
- Quality assurance of conversation flows and information extraction
- User testing of the scheduling interface and post-call experience
- A/B testing of different conversation approaches to optimize engagement and data collection

## Metrics and Analytics
The feature tracks the following key metrics:
- Call completion rate
- Average call duration
- Information extraction accuracy
- Profile enhancement metrics (completeness, depth, accuracy)
- Candidate satisfaction scores
- Conversion rate to job applications
- Recruiter feedback on candidate data quality

## Dependencies
- Twilio client interface for outbound calling (implemented in `/internal/tasks/handlers/twilio_client.go`)
- LLM API for conversation (implemented via (https://github.com/chriscow/minds)[minds] library)
- Audio processing capabilities (not yet implemented)
- Database schema for call data:
  - `appointments` table in `/workspaces/talent-rodeo/assets/migrations/000006_add_scheduling_tables.up.sql` - Stores call scheduling data, status, and transcript information
  - `scheduling_preferences` table in `/workspaces/talent-rodeo/assets/migrations/000006_add_scheduling_tables.up.sql` - Stores candidate's preferred call times
  - `job_search_criteria` table in `/workspaces/talent-rodeo/assets/migrations/000006_add_scheduling_tables.up.sql` - Stores criteria extracted from conversations
  - `candidate_skills` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores skills extracted from conversations
  - `candidate_preferences` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores preferences extracted from conversations
- Secure storage for recordings (not yet implemented)

## Feature Dependencies
- Depends on F001_User_Onboarding for user account and phone information
- Required for F003_Job_Search and F004_Job_Matching to have quality candidate data

## Development Timeline
- Phase 1 (Complete): Basic call scheduling and simple conversations
- Phase 2 (In Progress): Enhanced conversations and information extraction
- Phase 3 (Planned): Advanced profile enhancement and job matching integration
- Phase 4 (Future): Adaptive conversations and multi-language support

## Documentation
- [Technical Architecture Document](.project/03_technical_docs/architecture/telephony_integration.md)
- [API Documentation](.project/03_technical_docs/api/call_management_api.md)
- [Conversation Design Guidelines](.project/03_technical_docs/design/conversation_guidelines.md)
- [User Journey Map](.project/02_ux_docs/journey_maps/assessment_call_journey.md)


## Future Enhancements
- Add multi-language support for international candidates
- Video call option for candidates who prefer visual interaction
- Implement sentiment analysis during conversations
- Enhanced personalization based on industry and role type
- Integration with additional skill assessment tools
- Enable follow-up scheduling for incomplete assessments
- Add human-in-the-loop fallback for complex situations
- Implement conversation summarization for recruiters
- Improved career path recommendations based on call data
- Adaptive conversation flows based on candidate responses
- Mobile app notifications and call handling 