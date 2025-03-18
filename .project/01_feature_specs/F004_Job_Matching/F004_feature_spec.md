# F004: Job Matching

## Feature Overview
The Job Matching feature enables Talent Rodeo to evaluate how well job opportunities align with a candidate's skills, experience, and preferences. It applies scoring algorithms to quantify compatibility, filters results based on candidate preferences, and presents personalized job matches to users. This feature builds upon structured data extracted by the F003a Job Data Extraction feature.

## Implementation Status
- Preference Filtering: **Partially Implemented**
- Match Quality Assessment: **Partially Implemented**
- Match Result Display: **Minimally Implemented**

## Detailed Specifications

### SF004.1: Preference Filtering

#### Description
The system applies filters based on user preferences to ensure that job matches align with the candidate's personal requirements and career goals. For the MVP, filtering will be limited to location preferences.

#### Implementation Details
- Filter application post initial search
- Location-based filtering limited to supported metro areas and fully remote positions
- Additional filters deferred to post-MVP (compensation, company size, etc.)

#### Current Implementation
- Database schema in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql`: Stores user preferences
- Basic UI for preferences in `/workspaces/talent-rodeo/cmd/web/handlers.preferences.go`: Displays preferences form but does not process form data
- Basic integration with search handlers for initial filtering

#### MVP Requirements
- Implement form processing in `updatePreferences` function
- Create dropdown UI for location selection with:
  - "Fully Remote" option (which has its own GeoID)
  - Top 5 metro areas (with hard-coded GeoIDs)
- Store location preference in the `candidate_preferences` table
- Apply location filter to search results before scoring
- Add clear UI messaging that indicates other preferences will be considered in future updates

#### Location Handling Strategy
- For MVP, we will use a hard-coded lookup table of GeoIDs for the top 5 metropolitan areas
- The UI will clearly show "Supported Metro Areas" in a dropdown
- "Fully Remote" will be available as a separate option with its own GeoID
- No partial matches will be supported in the MVP; jobs must exactly match the selected location

### SF004.2: Match Quality Assessment

#### Description
The system calculates and displays a match percentage for each job based on how well it aligns with the candidate's skills and preferences. This helps candidates prioritize applications to roles where they have the highest likelihood of success.

#### Implementation Details
- Weighted scoring algorithm based on skill matches
- +10 points for each "Must Have" skill match
- +5 points for each "Preferred" skill match
- Normalization to percentage representation (see formula below)
- Single-stage LLM analysis for match assessment

#### Normalization Strategy
The following normalization formula will be implemented:

```go
// Calculate raw score
rawScore := 0
for _, skill := range candidateSkills {
    if jobHasSkill(job, skill) {
        if skill.IsMustHave {
            rawScore += 10
        } else {
            rawScore += 5
        }
    }
}

// Calculate max possible score
maxPossibleScore := len(mustHaveSkills)*10 + len(preferredSkills)*5

// Normalize to percentage (preventing division by zero)
matchPercentage := 0
if maxPossibleScore > 0 {
    matchPercentage = (rawScore * 100) / maxPossibleScore
}

// Apply minimum threshold (e.g., 30%)
if matchPercentage < 30 {
    // Don't display this job
}
```

#### Current Implementation
- Basic scoring implementation in `scoreJobs` function in `/workspaces/talent-rodeo/internal/handlers/search.go`: Implements the skill scoring logic
- Database schema in `/workspaces/talent-rodeo/assets/migrations/000011_add_match_assessments_table.up.sql`: Structure exists but not actively used
- Preliminary implementation in `/internal/handlers/assessment.go`: Contains initial LLM integration
- No implementation of normalization, experience matching, or preference alignment

#### MVP Requirements
- Enhance `scoreJobs` function to implement the normalization formula above
- Implement single-stage LLM analysis that receives:
  - Candidate skills and experience
  - Job details from extraction
  - Raw match score and percentage
- LLM should output:
  - Overall match assessment (1-5 rating)
  - Brief explanation of the match quality (2-3 sentences)
  - Key matching points and potential gaps
- Store match results in the simplified database schema (see below)
- Implement a NATS message handler for processing match assessments
- Use the existing NATS infrastructure from F003a:
  - Reference the `rodeoStream` constant defined in `/handlers/types.go`
  - Follow pattern in `/internal/handlers/generic_handler.go` for LLM integration

#### Handling Incomplete Data
- If skill information is incomplete or ambiguous, the LLM will be relied upon to make a judgment
- Jobs with insufficient information for reliable matching will be excluded from results
- Minimum threshold for displaying jobs: 30% match score

### SF004.3: Match Result Display

#### Description
The system presents job matches to users in an intuitive, actionable format. Results are presented in a card-based interface that encourages user interaction and feedback.

#### Implementation Details
- Display jobs in a card-based interface showing 3 jobs at a time
- Each card includes essential job information and match details
- User interaction required to view additional jobs
- Feedback collection mechanism to improve future matches

#### Current Implementation
- Basic implementation in `/workspaces/talent-rodeo/cmd/web/handlers.search.go`: Contains mock search progress and results
- No implementation of detailed match breakdown or visual indicators
- No implementation of search results dashboard

#### MVP Requirements
- Implement basic search results dashboard template with the following components:
  - Card-based interface showing 3 jobs at a time
  - Each card containing:
    - Job Title
    - Company Name
    - Company Logo (if available)
    - Match percentage as a simple visual indicator (progress bar or stars)
    - LLM-generated match explanation (brief)
    - "Interested" and "Not Interested" buttons
    - "Tell us why" dropdown with common reasons for feedback
  - Non-scrolling header with status bar showing:
    - Number of jobs searched
    - Number of matches found
    - Next scheduled search time (based on subscription tier)
- When user votes on a job, it fades out and a new job card appears
- Include clear messaging that all jobs are saved and can be revisited
- Create handlers for retrieving and displaying match results
- Implement a "History" tab for accessing all previously viewed jobs
- Support super user access to view match scores and LLM explanations

## Database Schema Simplification

For the MVP, we will consolidate the six match-related tables into two tables:

1. `job_matches` table (consolidation of `candidate_search_results` and `match_assessments`):
   ```sql
   CREATE TABLE job_matches (
       id SERIAL PRIMARY KEY,
       user_id INTEGER REFERENCES users(id),
       job_id INTEGER REFERENCES jobs(id),
       raw_score INTEGER,
       match_percentage INTEGER,
       llm_rating INTEGER,
       llm_explanation TEXT,
       user_feedback TEXT,
       user_feedback_category VARCHAR(50),
       created_at TIMESTAMP DEFAULT NOW(),
       updated_at TIMESTAMP DEFAULT NOW()
   );
   ```

2. `skill_matches` table (simplified from `skills_matches`):
   ```sql
   CREATE TABLE skill_matches (
       id SERIAL PRIMARY KEY,
       job_match_id INTEGER REFERENCES job_matches(id),
       skill_id INTEGER REFERENCES candidate_skills(id),
       is_match BOOLEAN,
       match_confidence FLOAT,
       created_at TIMESTAMP DEFAULT NOW()
   );
   ```

**Note:** These changes should be made by editing the existing migrations in place, not by creating new migrations.

## Error Handling Strategy

- **LLM Processing Failures**: Implement retry logic with exponential backoff for transient failures
- **Rate Limiting**: If being throttled, back off and try again later
- **Missing User Skills**: If no skills or preferences are set, user will be prompted to have another phone call or manually add skills/job titles
- **Minimum Requirements**: Users must provide at least 3 skills or 1 job title to enable matching

## Dependencies
- Job extraction results from F003a (Job Data Extraction)
- Database schema for job matches (simplified as described above)
- User profile with preference data:
  - `candidate_skills` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores candidate skills with preference ratings
  - `candidate_preferences` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores additional candidate preferences
- LLM integration using `github.com/chriscow/minds` library:
  - Reference implementation in `/internal/handlers/assessment.go` and `/internal/handlers/generic_handler.go`
  - NATS messaging infrastructure using the same patterns as F003a

## Integration Points Clarification
- User onboarding (F001): Skills and experience data flows from user profile
- Job search (F003): Job listings flow from search results
- Job data extraction (F003a): Structured job data feeds into match algorithm

## Feature Dependencies
- Depends on F001_User_Onboarding for user profile data
- Depends on F002_Initial_Assessment_Call for skills and preferences information
- Depends on F003_Job_Search for job listing data to match against
- Depends on F003a_Job_Data_Extraction for structured job data
- Required for F007_Document_Customization to target documents to appropriate jobs

## Future Enhancements
- Implement advanced filtering (company size, industry, etc.)
- Add semantic matching for skills beyond keyword matching
- Enable company reputation scoring in match calculation
- Implement personalized job recommendations using ML
- Add competitor job comparison features
- Develop career path projection based on current matches
- Implement "stretch role" identification for growth opportunities
- Add historical match tracking to show progress over time
- Add comprehensive LLM performance evaluation system for continuous improvement
- Create detailed error reporting for LLM match assessment failures
- Implement job matching performance metrics and optimization
- Add multi-stage LLM analysis for more sophisticated matching

## User Experience Considerations
- Match results should load quickly and be visually intuitive
- Filtering options should be easily accessible and adjustable
- Match strength should be clearly communicated through visual cues
- Users should be able to easily compare multiple job matches
- Search refinement options should be suggested based on match results
- UI should always maintain shippable, working state throughout implementation
- Super users should have access to additional debugging and quality control views
- Feedback collection should be simple and intuitive with minimal friction

## Testing Requirements
- Unit tests for all matching functions
- Integration tests with job search functionality
- User acceptance testing for match relevance
- Performance testing with large volumes of job listings
- UI component tests for match display interfaces
- A/B testing for match presentation variations
- LLM assessment evaluation and quality metrics
- Performance expectation: Handle at least 20 job assessments per user within 2 minutes

## Implementation Plan for MVP
1. Modify database migrations to create simplified schema (edit in place)
2. Implement location preference UI with dropdown for supported areas
3. Create the job card display UI showing 3 jobs at a time
4. Implement the skill matching and normalization algorithm
5. Add single-stage LLM integration for match assessment
6. Implement user feedback collection mechanism
7. Create "History" tab for accessing previously viewed jobs
8. Connect all components through the existing handler structure

Each subfeature must be implemented with complete UI components, comprehensive tests, and a clear path forward for future enhancements. Code should always remain in a shippable state, following our agile practices. 