# Job Matching System Implementation Plan

**IMPORTANT NOTICE**: This document has been split into two new documents for better organization:
1. `JOB_MATCHING_OVERVIEW.md` - Contains the high-level implementation plan
2. `JOB_MATCHING_PHASES.md` - Contains detailed implementation information for each phase

Please refer to these new documents for the most up-to-date information.

**IMPORTANT**: DO NOT DELETE ANYTHING IN THIS FILE. Mark items as completed (e.g. via ~~strike through~~) or move them to a "Deprecated/Completed" section to maintain a full record.

I want to maintain a record of the implementation plan for future reference.


# Feature Description
Be sure you read the JOB_MATCHING_CORE.md file to understand the overall goal of the project.

## **Implementation Guidelines**
+   - Keep it simple - this is an MVP
+   - Avoid premature optimization
+   - Minimize dependencies
+   - Focus on user value
+   - Add complexity only when needed

## Overview

The Job Matching System will power job searches, intelligent matching, and candidate pairing based on skills, requirements, culture, and preferences. We will start with an MVP and iterate as needed. Key principles include simplicity, minimal dependencies, and high user value.

## Naming Conventions & Table Structure Suggestions

- **Jobs Data:**  
  - [ ] Rename the generic `jobs` table to `linkedin_jobs` to clearly indicate the source of data.  
- **Users & Candidates:**  
  - [ ] Separate user authentication/authorization (table: `users`) from candidate-specific profiles (table: `candidates`).  
  - [ ] Add supporting tables for RBAC such as `user_roles` if needed.  
- **Matching & Analysis:**  
  - [ ] Use `job_matches` for match results.  
  - [ ] Create dedicated tables like `job_keywords` and `job_requirements` to store extracted and analyzed data.
- **Privacy:**  
  - [ ] Keep private candidate information in a separate table (`candidate_private_info`) to facilitate future encryption without disrupting public data. We don't want the complexity of encryption to be a barrier to getting the MVP out the door.

## Project Structure
```
internal/
└── jobmatch/
    ├── handlers/          # HTTP/API handlers for job matching operations
    ├── models/            # Domain models (e.g. LinkedinJob, Candidate, Match)
    ├── processor/         # Core business logic (analysis, matching, vector ops)
    └── store/             # Database operations and migrations
```

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1–2)

- **Database Schema Design**  
  - [ ] Review existing migrations in `/assets/migrations` and `/internal/database`.  
  - **Tables:**
    - [ ] `linkedin_jobs`: Stores raw job data from LinkedIn.
    - [ ] `users`: Basic account information and roles.
    - [ ] `candidates`: Candidate profiles (public info).
    - [ ] `candidate_private_info`: Sensitive data (email, phone, etc.).
    - [ ] `job_matches`: Stores computed match results.
    - [ ] `job_keywords` & `job_requirements`: For keyword extraction and requirement details.
- **Base Models Implementation**  
  - [ ] Define models for LinkedinJob (copied from /internal/scrapingdog/linkedin/jobs.go), Candidate, Match, Preference, Keyword, etc.
- **Task Handler Setup**  
  - [ ] Create handlers for:
    - [ ] Job search and data ingestion.
    - [ ] Job analysis (keyword extraction, requirement parsing).
    - [ ] Match processing and update notifications.

### Phase 2: Search and Analysis (Week 2–3)

- **LinkedIn Jobs Integration**  
  - [ ] Refactor ScrapingDog code (found in `/internal/scrapingdog` and `/cmd/try/scrapingdog`) into this project.
  - [ ] Implement job data extraction, storage in `linkedin_jobs`, and proper indexing.
- **Keyword & Requirement Analysis**  
  - [ ] Build keyword extraction and requirement analysis routines.
  - [ ] Identify culture traits and map them to candidate preferences.
- **Search Infrastructure**  
  - [ ] Develop vector generation for jobs.
  - [ ] Set up indexing and basic search functionality.

### Phase 3: Matching System (Week 3–4)

- **Vector Operations**  
  - [ ] Generate vectors for jobs and candidates.
  - [ ] Implement similarity computation.
- **Matching Algorithm**  
  - [ ] Develop requirement and culture fit scoring.
  - [ ] Combine scores into a composite match score.
- **Match Processing**  
  - [ ] Persist match results in `job_matches`.
  - [ ] Enable status tracking and updates.

### Phase 4: Optimization and Testing (Week 4–5)

- **Performance Optimization**  
  - [ ] Integrate caching and batch processing.
  - [ ] Optimize search indexing and vector computations.
- **Testing**  
  - [ ] Write unit and integration tests for all components.
  - [ ] Run performance benchmarks.
- **Documentation**  
  - [ ] Complete API documentation, system architecture details, and a deployment guide.

### Phase 5: Load Distribution with Watermill and NATS (Week 5–6)

- **Asynchronous Processing**  
  - [ ] Utilize Watermill with NATS Jetstream to offload job downloading and matching tasks to separate goroutines for improved responsiveness and scalability.
- **Message Persistence**  
  - [ ] Implement NATS Jetstream for message persistence and delivery guarantees, ensuring reliable processing of job data under high load.
- **Modular Architecture**  
  - [ ] Facilitate communication between system components for a more modular architecture using NATS.

## Security and Privacy Measures

### MVP Security
- [ ] Use HTTPS for all connections.
- [ ] Store passwords securely using bcrypt.
- [ ] Leverage PostgreSQL's role-based access control.
- [ ] Separate sensitive candidate data in `candidate_private_info`.

### Example Schema (MVP)
```sql
-- Candidates: Public profile
CREATE TABLE candidates (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    public_name TEXT,
    created_at TIMESTAMPTZ DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ DEFAULT current_timestamp
);

-- Private candidate data (for future encryption enhancements)
CREATE TABLE candidate_private_info (
    candidate_id UUID REFERENCES candidates(id) PRIMARY KEY,
    email TEXT,
    phone TEXT
);
```

### Future Enhancements
- **Encryption:**  
  - [ ] Gradually add encrypted columns (e.g. `encrypted_email BYTEA`) while retaining original data until migration is complete.

## Technical Details

### Database Schema Snippet
```sql
-- linkedin_jobs table (raw job data)
CREATE TABLE linkedin_jobs (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    company TEXT NOT NULL,
    source TEXT DEFAULT 'linkedin',
    created_at TIMESTAMPTZ DEFAULT current_timestamp
    -- Additional fields as needed
);
```

### Core Algorithms

#### Vector Generation Interface
```go
type VectorGenerator interface {
    JobToVector(ctx context.Context, job *LinkedinJob) ([]float64, error)
    CandidateToVector(ctx context.Context, candidate *Candidate) ([]float64, error)
    ComputeSimilarity(v1, v2 []float64) float64
}
```

#### Match Scoring Interface
```go
type MatchScorer interface {
    ScoreMatch(ctx context.Context, job *LinkedinJob, candidate *Candidate) (*Match, error)
    ScoreRequirements(ctx context.Context, requirements []JobRequirement, candidate *Candidate) float64
    ScoreCultureFit(ctx context.Context, culture []CultureTrait, preferences []Preference) float64
}
```

## Progress Tracking

- **Current Status:**
  - [ ] Phase 1: Not Started (Database design and model definitions in progress)
  - [ ] Phase 2: Not Started
  - [ ] Phase 3: Not Started
  - [ ] Phase 4: Not Started
  - [ ] Phase 5: Not Started

- **Completed:**
  - [ ] Initial design document
  - [ ] Project structure definition

- **Next Steps:**
  1. [ ] Finalize and implement the database schema.
  2. [ ] Create base models and establish RBAC.
  3. [ ] Set up task handler infrastructure.

## Notes & Reminders

- [ ] Ensure proper error handling and verbose debug logging throughout.
- [ ] Follow Go coding standards for consistency.
- [ ] Aim for over 80% test coverage.
- [ ] Document all public interfaces and functions clearly.

## Updates

- **Initial Document Creation:** [Current Date]

This plan will be updated as the project evolves. Use it as both a roadmap and a record of design decisions. 