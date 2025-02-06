# Job Matching Core Design and Implementation Plan

## Overview
The Job Matching Core will leverage the existing scrapingdog package to perform searches for job postings on LinkedIn based on job titles, skills, and requirements. It will also handle summarization tasks and matching algorithms to enhance the recruitment process. We aim to find relevant job postings that match candidates' skills and preferences, minimizing cognitive load and providing a seamless experience. Job listings will present a distilled description and a list of requirements with icons indicating the candidate's match level. A premium feature will allow candidates to "reveal" the actual job and link to apply, similar to how "Hotels Tonight" operates.

Another premium feature will enable candidates to adjust sliders to indicate their preferences for each skill, requirement, or preference.

## Objectives
1. **Integrate the scrapingdog package** for job posting retrieval.
2. **Ensure compatibility with the database package** for storing and retrieving job data.
3. **Implement a processor** in the handlers package to manage job search and matching tasks.

## Data Freshness and Quality
To ensure the integrity and reliability of job postings, we will implement the following strategies:

- **Job Posting Updates**: Implement a scheduled task within the `internal/tasks` package to refresh job listings daily. This task will work with existing code in `/cmd/try/scrapingdog` and `/internal/scrapingdog` to cache previously performed searches, preventing unnecessary data fetching.

- **Other Data Sources**: The system will support multiple job data sources, allowing integration of additional job listing APIs. This flexibility mitigates risks associated with relying on a single provider and enhances the overall quality of job data available to candidates.

## PostgreSQL Data Partitioning
To manage large volumes of job postings efficiently, we will utilize PostgreSQL's table partitioning feature, introduced in version 13 and later. This approach offers several advantages over traditional methods:

- **Efficient Query Performance**: Partitioning allows PostgreSQL to prune partitions during queries, scanning only relevant date ranges. For instance, when searching for current job postings, the database automatically skips older partitions, significantly improving query performance.

- **Easy Data Archiving**: Old data can be archived by detaching and dropping entire partitions, which is nearly instantaneous and does not require scanning or rewriting data. This ensures that the system continues serving requests without interruption during archiving.

- **Manageable Index Maintenance**: Indexes are created per partition, simplifying index maintenance. When an old partition is dropped, its associated indexes are removed automatically, avoiding the overhead of index updates during deletion.

### Practical Example
Here's how this might look for job postings:

```sql
CREATE TABLE linkedin_jobs (
    id bigint,
    created_at timestamp,
    -- other columns
) PARTITION BY RANGE (created_at);

-- Create monthly partitions
CREATE TABLE linkedin_jobs_202402 PARTITION OF linkedin_jobs
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');

-- Archive by detaching old partitions
ALTER TABLE linkedin_jobs DETACH PARTITION linkedin_jobs_202301;
```

## Handling Edge Cases in Matching
If a candidate's skills don't align perfectly with a job, we will utilize the cosine similarity algorithm and other methods to display adjacent jobs that may fit the candidate. To enhance user engagement, we will allow users to provide feedback through thumbs up/down on job suggestions and comments on their preferences.

## Company Matching & Cultural Fit
We will match candidates based on company culture fit using the following strategies:

- **LLM Summarization**: Use a language model to generate a paragraph summarizing the company culture for semantic matching.
  
- **Keyword Matching**: Implement keyword matching to identify cultural preferences based on candidate input and company values.

- **Culture Preferences**: Capture culture preferences from candidates during onboarding to better align job matches with their values.

## Candidate Feedback Loop
To continuously improve the matching process, we will implement a candidate feedback loop that includes:

- **Direct Candidate Feedback**: Introduce a "Did you apply?" button to track candidates' application statuses.

- **Application Tracking System**: For the MVP, we will offer a simple application tracking system to monitor the state of applied jobs (e.g., applied, contact, screening, received offer).

- **Performance Tracking**: Track how well matches perform, including metrics such as user applications, interviews, and accepted offers.

- **Rating System**: Implement a rating system with questions like "Why didn't this job interest you?" to gather feedback quickly and easily.

## Matching Algorithm Enhancements
To enhance our matching capabilities, we will incorporate:

- **Semantic Search**: Implement semantic search techniques alongside cosine similarity and TF-IDF to improve match relevance.

- **LLM-Enhanced Ranking**: Vectorize job descriptions and candidates' experiences, ranking them by relevance using advanced language models.

- **Vector Database**: Utilize Pinecone as a vector database to store and manage vector representations of job postings and candidate profiles for efficient retrieval and matching.

## Skills Taxonomy
To improve matching accuracy, we will implement a standardized skills taxonomy:

- **Skills Graph Database or Table**: Create a skills graph database or table mapping related skills (e.g., "Golang" maps to "Go") to ensure effective recognition and matching of different terms for the same skills.

- **Improved Matching Accuracy**: Utilize the skills taxonomy to enhance alignment between job listings and candidate resumes, even with varying terminology.

## Salary Range Matching
To address salary expectations in the matching process, we will implement:

- **Salary Range Fields**: Introduce salary range fields in job postings and candidate profiles to specify expectations and budgets.

- **Incorporate Salary into Matching Logic**: Integrate salary range data into the matching algorithm to filter out mismatched expectations, ensuring candidates see only suitable opportunities.

## Load Distribution with Watermill and NATS Jetstream
To efficiently distribute the load of downloading jobs and performing matching operations, we will utilize **Watermill** with **NATS Jetstream**. This setup will enable us to:

- **Asynchronously Process Jobs**: Offload job downloading and matching tasks to separate goroutines for improved responsiveness and scalability.

- **Leverage NATS Jetstream**: Use NATS Jetstream for message persistence and delivery guarantees, ensuring reliable processing of job data under high load.

- **Simplify Communication**: Facilitate communication between system components for a more modular architecture.

- **Utilize NATS Key/Value Store**: If beneficial, leverage NATS' Key/Value store for real-time updates and historical tracking of changes.

## Data Privacy and Compliance
The design must address how job seeker and employer data will be handled in compliance with privacy regulations. We will implement:

- **Data Retention Policies**: Establish clear policies defining how long personal data will be stored and conditions for deletion.

- **User Consent Management**: Implement processes to inform candidates and employers about data usage and allow consent management.

- **Data Security Measures**: Ensure appropriate security measures protect personal data from unauthorized access and breaches.

- **Compliance with Regulations**: Stay informed about relevant data protection regulations (e.g., GDPR, CCPA) to ensure compliance.

## Future Goals
### Recruiter Side of the Algorithm
Although the recruiter side is not yet implemented, we should consider the following questions for future development:
- Will recruiters see why a candidate is a strong match? A breakdown of strengths and gaps would help.
- Can recruiters request AI-generated introductions based on candidate strengths?

## Preliminary Tasks
### 1. Database Migration
- **Convert existing code to use PostgreSQL** if not already implemented.
- Ensure all database interactions in the scrapingdog package are compatible with the database package.

### 2. Database Code Validation
- Validate that the database code in the scrapingdog package works seamlessly with the database package.
- Implement necessary adjustments to ensure data integrity and performance.

## Design Considerations
- **Data Structure**: Define data models for job postings, including fields such as job title, company name, location, skills required, and job description.
- **Search Functionality**: Implement search functionality allowing users to query job postings based on various criteria (e.g., job title, skills).
- **Matching Algorithm**: Develop algorithms to match candidates with job postings based on their skills and preferences.

## Implementation Plan
### 1. Create a Processor
- Develop a new processor in the tasks/handlers package to handle job search, job matching, and summarization tasks.
- The processor should include methods for:
  - Searching job postings
  - Distilling job descriptions
  - Matching candidates to job postings

### Matching Algorithm
- Use the cosine similarity algorithm to match candidates to job postings.
- Develop a vector representation of job requirements and candidate skills/preferences.
- Allow candidates to adjust sliders on their skills and preferences as vector values to refine matches.
- Use the TF-IDF algorithm to distill job descriptions into concise summaries for presentation.

### 2. Logging and Error Handling
- Implement verbose debug logging throughout the processor to track states and values.
- Ensure proper error handling to provide meaningful feedback during operations.

### 3. Testing
- Write unit tests for all new functionalities to verify correctness and ensure code coverage.
- Use TDD principles to guide the development process.

## Risk Management
Identify potential risks related to data privacy, algorithm bias, and system scalability, and develop mitigation strategies to address these concerns.

## User Experience Considerations
Incorporate user experience design principles to ensure intuitive interfaces and effective user feedback mechanisms.

## Performance Metrics
Define success metrics, including user satisfaction, match accuracy, and system performance, to evaluate the effectiveness of the Job Matching Core.

## Timeline and Milestones
Outline a high-level timeline and key milestones for the implementation phases to provide clarity on project expectations.

## Conclusion
This document will serve as a living document to track progress, decisions, and changes throughout the development of the Job Matching Core. Regular updates and discussions will be encouraged to ensure alignment with project goals and objectives. 