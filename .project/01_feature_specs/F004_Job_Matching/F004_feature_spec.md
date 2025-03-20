# F004: Job Matching

## Feature Overview
The Job Matching feature enables Talent Rodeo to evaluate how well job opportunities align with a candidate's skills, experience, and preferences. It applies scoring algorithms to quantify compatibility, filters results based on candidate preferences, and presents personalized job matches to users. This feature builds upon structured data extracted by the F003a Job Data Extraction feature.

## Implementation Status
- Location Preferences (SF004.1): **Not Implemented**
- Database Schema (SF004.2): **Partially Implemented** 
- Match Scoring Algorithm (SF004.3): **Not Implemented**
- LLM Match Assessment (SF004.4): **Not Implemented**
- Job Card UI (SF004.5): **Not Implemented**
- Results Dashboard (SF004.6): **Not Implemented**

## Detailed Specifications

### SF004.1: Location Preferences

#### Description
The system applies filters based on user preferences to ensure that job matches align with the candidate's personal requirements and career goals. For the MVP, filtering will be limited to location preferences.

#### Implementation Details
- Searches are performed by keywords. Filters are applied post initial search
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
- The LinkedIn search via ScrapingDog will exactly match the selected location
- Note: A placeholder table will be implemented initially with developer-created GeoIDs, and the actual GeoID values will be filled in later by the product team

#### Developer Implementation Notes
- The LinkedIn search via ScrapingDog requires a location to perform a search
- Search results returned are already filtered for the location selected by the user
- The top 5 metro areas will be defined by the product owner and provided to the developer
- In the meantime, the developer should create a hard-coded map of the top 5 metro areas from their own knowledge with placeholder GeoIDs, including a "Remote" entry
- For the "Fully Remote" option, ScrapingDog accepts a specific "Remote" geoID

### SF004.2: Database Schema

#### Description
The system calculates an initial match percentage for each job based on how well it aligns with the candidate's 
skills and preferences. This helps filter out obviously irrelevant jobs. Then, an LLM performs qualitative analysis 
of the remaining jobs to provide detailed reasoning about the match quality and make a final determination on 
whether the job is a good match. This approach leverages the LLM's strength in natural language reasoning while 
still providing a binary classification needed for the system.

#### Match-Related Tables
The system consolidates six match-related tables (currently defined in `/workspaces/talent-rodeo/assets/migrations/000011_add_match_assessments_table.up.sql`) into two tables:
- Originally defined tables: `match_assessments`, `skills_matches`, `matched_skills`, `missing_skills`, `experience_matches`, `location_matches`, and `compensation_fits`

The consolidated tables are:

1. `job_matches` table (consolidation of `candidate_search_results` and `match_assessments`):
   ```sql
   CREATE TABLE job_matches (
       id SERIAL PRIMARY KEY,
       user_id INTEGER REFERENCES users(id),
       job_id INTEGER REFERENCES jobs(id),
       initial_score INTEGER,
       match_percentage INTEGER,
       is_match BOOLEAN,
       job_match_analysis TEXT, -- extensive chain-of-thought reasoning
       job_match_summary TEXT, -- This is what is presented to the user
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

**Note:** These changes should be made by editing the existing migration file at `/workspaces/talent-rodeo/assets/migrations/000011_add_match_assessments_table.up.sql` directly. Since the database has not been deployed to production and we are still developing the initial schema, it is safe to edit existing migrations to keep the number of migrations files to a reasonable level.

### SF004.3: Match Scoring Algorithm

#### Description
The system calculates an initial match percentage for each job based on how well it aligns with the candidate's skills and preferences. This helps filter out obviously irrelevant jobs and provides a quantitative foundation for further analysis.

#### Implementation Details
- Weighted scoring algorithm based on skill matches
- +10 points for each "Must Have" skill match
- +5 points for each "Preferred" skill match
- Normalization to percentage representation using the formula below
- Initial filtering of jobs below match threshold

#### Normalization Formula
The scoring normalization formula calculates a percentage match score as follows:
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

#### Developer Implementation Notes
- The `jobHasSkill` function will be implemented using an LLM prompt that takes in a skill and the job description
- The LLM will return a structured JSON response that returns true if the skill (or a synonym, e.g., "Go" or "Golang") is explicitly stated in the job description
- The 30% threshold for job display is an arbitrary starting point and should be implemented as a constant that is easy to find and change
- The maximum number of jobs to process for each user is defined by how many "pages" of job listings are defined for the user's tier:
  - Free users: 3 pages of job listings
  - Paid users: 50 pages of job listings

#### Current Implementation
- Basic scoring implementation in `scoreJobs` function in `/workspaces/talent-rodeo/internal/handlers/search.go`: Implements the skill scoring logic
- Database schema in `/workspaces/talent-rodeo/assets/migrations/000011_add_match_assessments_table.up.sql`: Structure exists but not actively used
- Preliminary implementation in `/internal/handlers/assessment.go`: Contains initial LLM integration framework including:
  - Basic NATS message handler setup
  - Data structures for match assessments 
  - Stub methods for LLM processing
  - Database storage functions
- No implementation of normalization, experience matching, or preference alignment

#### MVP Requirements
- Implement the normalization formula for skill matching
- Create weighted scoring system for different match criteria
- Apply minimum threshold filtering (30%) as an easily modifiable constant
- Store raw and normalized scores in the database
- Location preference is part of the core search and not a separate filter

### SF004.4: LLM Match Assessment

#### Description
The system uses an LLM to perform qualitative analysis of jobs that pass the initial scoring threshold. This provides detailed reasoning about match quality and makes final determinations on job suitability.

#### Implementation Details
- Single-stage LLM analysis for match assessment
- Comprehensive analysis of job-candidate alignment
- Detailed explanation of match quality
- Binary match determination (Match/No Match)
- Storage of analysis and determinations

#### Developer Implementation Notes
- LLMs are rate limited by the total number of tokens processed within a time window
- The system will rely on the LLM API to handle rate limiting
- If rate limits are hit, the system should implement exponential backoff
- The LLM prompt has not been tested for consistency and quality of results yet

#### Current Implementation
- Basic scoring implementation in `scoreJobs` function in `/workspaces/talent-rodeo/internal/handlers/search.go`: Implements the skill scoring logic
- Database schema in `/workspaces/talent-rodeo/assets/migrations/000011_add_match_assessments_table.up.sql`: Structure exists but not actively used
- Preliminary implementation in `/internal/handlers/assessment.go`: Contains initial LLM integration
- No implementation of normalization, experience matching, or preference alignment

#### MVP Requirements
- Enhance `scoreJobs` function to implement the normalization formula above as an initial filter
  - Summary of transcript in first person from that candidate's perspective
  - Candidate skills and experience
  - Job details from extraction
  - Raw match score and percentage (for reference only)
- LLM should output:
  - A comprehensive analysis of how the job relates to the candidate's profile (chain-of-thought reasoning)
  - Detailed explanation of the match quality highlighting specific alignments and misalignments
  - Key matching points and potential gaps
  - A final binary determination (Match/No Match) based on its qualitative analysis
  - For jobs determined to be non-matches: Detailed explanation of why the job doesn't match, including specific criteria used for this determination
- Store match results in the simplified database schema (see below)
- Store all LLM analysis, assessments, and explanations for jobs with the candidate's search records
- Implement a NATS message handler for processing match assessments (see `/internal/handlers/assessment.go` and modify as needed)
- Use the existing NATS infrastructure from F003a:
  - Reference the `rodeoStream` constant defined in `/handlers/types.go`
  - Follow pattern in `/internal/handlers/generic_handler.go` for LLM integration
- Implement exponential backoff when hitting LLM rate limits

#### Handling Incomplete Data
- If skill information is incomplete or ambiguous, the LLM will be relied upon to make a judgment
- Jobs with insufficient information for reliable matching will be excluded from results
- Jobs with initial match percentage below 30% will be automatically filtered out

#### Non-Matching Job Processing
- Jobs can be determined as non-matches in two ways:
  1. Automatically filtered if initial match percentage is below 30%
  2. Determined to be a non-match by the LLM after qualitative analysis
- For each job determined to be a non-match by the LLM, the analysis should include:
  - Primary reasons for the mismatch (skill gaps, experience level, location, etc.)
  - Specific criteria that led to the non-match determination
  - Key differences between candidate profile and job requirements
  - Potential paths for the candidate to become qualified (if applicable)
- The LLM's comprehensive analysis should be stored in the `job_match_analysis` field
- A concise summary of the non-match reasoning should be stored in the `job_match_summary` field
- The `is_match` boolean field should be set to `FALSE` for these records
- Non-matching jobs should still be accessible to users through the "Non-Matches" section in the History tab
- The system should use non-match data to improve future search algorithm refinements
- For automatically filtered jobs (below 30% threshold), a simple explanation should be stored indicating they didn't meet the minimum match criteria

### SF004.5: Job Card UI

#### Description
The system presents individual job matches in an intuitive card format that displays essential information and enables user interaction.

#### Implementation Details
- Card-based interface for individual jobs
- Essential job information display
- Match quality indicators
- User interaction buttons
- Feedback collection mechanism

#### Developer Implementation Notes
- If no new jobs are available after a user votes on a job, the system will show a message to the user
- No mockups or wireframes are available for the job card UI; developer should provide a preliminary design
- The effectiveness of the job matching feature will be monitored by analyzing user feedback on job cards

#### Current Implementation
- No current implementation

#### MVP Requirements
- Create job card component with:
  - Job Title
  - Company Name
  - Company Logo (if available)
  - LLM-generated match summary
  - "Interested" and "Not Interested" buttons
  - If not interested, UI slides out with "Tell us why" feedback input box
- Implement card interaction handlers
- Add smooth transitions for card updates
- When user votes on a job, it should fade out and a new job card should appear (if available)
- If no new jobs are available, display a clear message to the user
- Provide preliminary job card UI design at development time
- Feedback and votes should be stored in the database

### SF004.6: Results Dashboard

#### Description
The system provides a comprehensive dashboard for viewing and managing job matches, including both current and historical results.

#### Implementation Details
- Dashboard showing 3 jobs at a time in large cards in a grid
- Non-scrolling status header
- History button to view previous matches
- Non-matches section with explanations
- Super user access for detailed analysis

#### Developer Implementation Notes
- The super user view will contain any and all information about the user's job search history
- Next scheduled search time calculation:
  - Free users: 3 days from the last search
  - Paid users: 1 day from the last search

#### Current Implementation
- Basic implementation in `/workspaces/talent-rodeo/cmd/web/handlers.search.go`: Contains mock search progress and results
- No implementation of detailed views or history

#### MVP Requirements
- Implement dashboard layout with status header showing:
  - Number of jobs searched
  - Number of matches found
  - Next scheduled search time (3 days from last search for free users, 1 day for paid users)
- Create job card container with 3-card view
- Add History tab functionality
- Implement Non-matches section with explanations for why jobs didn't match
- Create super user view for detailed analysis of the user's job search history
- Include clear messaging that all jobs are saved and can be revisited
- Create handlers for retrieving and displaying match results

## Error Handling Strategy

- **LLM Processing Failures**: Implement retry logic with exponential backoff for transient failures
- **Rate Limiting**: If being throttled, back off and try again later
- **Missing User Skills**: If no skills or preferences are set, user will be prompted to have another phone call or manually add skills/job titles
- **Minimum Requirements**: Users must provide at least 3 skills or 1 job title to enable matching

## Dependencies
- Job extraction results from F003a (Job Data Extraction)
  - The F003a feature provides structured job data through an asynchronous extraction pipeline
  - Job details are extracted from raw listings and stored in the `extracted_job_details` table
  - Skills are extracted and normalized in the `job_skills` table
  - This structured data is made available through the NATS messaging system and can be queried from the database
- Database schema for job matches (simplified as described in SF004.2)
- User profile with preference data:
  - `candidate_skills` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores candidate skills with preference ratings
  - `candidate_preferences` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores additional candidate preferences
- LLM integration using `github.com/chriscow/minds` library:
  - Reference implementation in `/internal/handlers/assessment.go` and `/internal/handlers/generic_handler.go`
  - NATS messaging infrastructure using the same patterns as F003a

## Integration Points Clarification
- User onboarding (F001): Skills and experience data flows from user profile
- Job search (F003): Job listings flow from search results
- Job data extraction (F003a): 
  - Structured job data feeds into match algorithm through NATS messaging
  - Job details extracted by F003a are stored in `extracted_job_details` table
  - Job skills extracted by F003a are stored in `job_skills` table
  - When new jobs are extracted, notifications are published to NATS topics
  - The job matching system subscribes to these topics to process new jobs as they become available

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

## LLM Prompt Guidelines

### Match Assessment Prompt Structure
The following structure should be used for prompting the LLM to generate match assessments:

```
You are analyzing a job posting for {Company} for the role of {JobTitle} to determine how well it matches a candidate's profile.

CANDIDATE PROFILE:
Skills: {CandidateSkills}
Experience: {CandidateExperience}
Preferences: {CandidatePreferences}

JOB DETAILS:
Title: {JobTitle}
Company: {Company}
Description: {JobDescription}
Requirements: {JobRequirements}
Location: {JobLocation}
Additional Info: {JobAdditionalInfo}

INITIAL MATCH METRICS:
Initial Score: {RawScore}
Match Percentage: {MatchPercentage}%
Match Threshold: 30%

TASK:
First, analyze the job posting against the candidate's profile by considering the following factors:
1. Skill alignment - how well do the candidate's skills match the job requirements?
2. Experience match - does the candidate have relevant experience for this role?
3. Location compatibility - does the job location match the candidate's preferences?
4. Career trajectory - does this job make sense for the candidate's career path?
5. Industry alignment - is this in an industry aligned with the candidate's background?

Write a comprehensive analysis examining these factors and explaining how well the job matches the candidate's profile. Include specific examples of alignments and misalignments.

Based on your thorough analysis above, provide:
1. A concise summary of the match quality (2-3 sentences)
2. A list of 3-5 specific matching points between the candidate and the job
3. A list of 1-3 potential gaps or misalignments
4. Your final determination: Is this job a MATCH or NO MATCH for this candidate?
5. A brief explanation of your final determination

Your analysis should be objective, thorough, and educational, focusing on qualitative reasoning rather than numerical scoring.
```

The LLM's response should be parsed to extract:
- The comprehensive match analysis (chain-of-thought reasoning)
- The concise match summary
- Key matching points and gaps identified
- The final match determination (Match/No Match)
- The explanation for the determination

This approach ensures the LLM leverages its strength in natural language reasoning while still providing the binary classification needed for the system.

## Testing Requirements
- Unit tests for the initial matching algorithm
- Tests for the LLM prompt generation and response parsing
- Integration tests with job search functionality
- User acceptance testing for match quality and determination accuracy
- Performance testing with large volumes of job listings
- UI component tests for match display interfaces
- A/B testing for different match presentation formats
- LLM assessment evaluation focusing on the quality and consistency of the match/no-match determinations and supporting analyses
- Evaluation of LLM determinations compared to user feedback to continuously improve the system
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