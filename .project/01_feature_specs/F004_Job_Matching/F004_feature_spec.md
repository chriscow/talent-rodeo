# F004: Job Matching (Refactored)

## IMPORTANT - this is an MVP. Do not over-engineer this feature. 

## Overview

The refactored Job Matching feature for Talent Rodeo is designed for scalability, cost efficiency, and extensibility. Instead of duplicating search logic and results for each user, the system now centralizes job search criteria ("Criteria") and allows users to subscribe to shared Criteria. This enables deduplication of searches, efficient job result sharing, and a robust foundation for advanced matching and analytics.

Key improvements:
- **Deduplicated Criteria**: Each unique search query (normalized and location-specific) is stored once and shared among all interested users.
- **User Subscriptions**: Users subscribe to Criteria, enabling personalized job feeds without redundant searches.
- **Many-to-Many Relationships**: Criteria and JobDetail (job result) are linked in a many-to-many fashion, supporting rich analytics and efficient storage.
- **LLM-Driven Matching**: Job features are extracted and normalized using LLMs, with both vector and keyword (sparse vector) representations for fast, flexible matching.
- **Efficient Scheduling**: Search frequency is dynamically managed based on Criteria popularity and user demand.

This document describes the new data model, search and scheduling logic, job matching pipeline, and implementation requirements for the MVP.

---

## Data Model

### Criteria (internal/search/types.go)
- Represents a unique, normalized search query and location (GeoID).
- Uniqueness is enforced by normalizing the query (lowercase, trimmed) and location, then slugifying the combination.
- Fields:
  - `ID` (slugified string)
  - `Query` (normalized string)
  - `GeoID` (GeoID numeric string)
  - `SubscriberCount` (int)
  - `CreatedAt`, `UpdatedAt`
  - `LastSearchedAt` (timestamp of when criteria was last searched)
  - (Optional) Human-readable location for display

### User
- Each user stores an array of subscribed Criteria IDs (`[]string`).
- Users can subscribe to existing Criteria or create new ones if needed.
- The user's "Statement" (first-person job preference narrative) is embedded and stored for semantic search.
- A synthesized job description based on the user's preferences to use as a symantic similarity search against job details (see below)

### JobDetail
- Represents a unique job listing, with extracted features and metadata.
- Additional Fields for internal/search/types.go JobDetail struct:
  - `ExtractedData` (existing field, struct ExtractedData internal/search/types.go)
  - `Features` (keywords, normalized)
  - `VectorEmbedding` (dense vector embedding of ExtractedData.Summary)
  - `SparseKeywords` (for sparse vector search)

### Relationships
- **Criteria ↔ JobDetail**: Many-to-many.
  - Each Criteria has a subcollection of unique JobDetail references to `job_details` 
  - Each JobDetail has a subcollection of unique Criteria references
- **User ↔ Criteria**: Many-to-many (user stores array of Criteria document references).

---

## Search & Scheduling

- The system maintains a registry of all unique Criteria.
- On a regular schedule, the search service:
  - Enumerates all Criteria with at least one subscriber that have a LastSearchedAt date older than 24 hours or nil.
  - Queues searches for the search provider (e.g., ScrapingDog).
- Search frequency can be adjusted based on Criteria popularity (subscriber count) and system heuristics.
- Criteria with zero subscribers are not searched.

---

## Job Matching Pipeline

1. **Search Execution**
   - For each Criteria, the search provider fetches job listings for the specified query and location (GeoID).
   - Results are deduplicated and stored as JobDetail documents.

2. **Feature Extraction & Embedding**
   - An LLM extracts and normalizes features (skills, requirements, etc.) from each JobDetail.
   - Features are stored as keywords and embedded as dense vectors (Pinecone) and sparse vectors (for keyword search).

3. **User Statement Embedding**
   - Each user's "Statement" is embedded and stored for semantic search against job vectors.

4. **Preliminary Matching**
   - Fast vector and keyword matching is performed between user statements and job features to filter jobs before LLM-based analysis.

5. **LLM Match Assessment**
   - For jobs passing the preliminary filter, an LLM performs a qualitative match assessment, producing:
     - Comprehensive analysis (chain-of-thought reasoning)
     - Concise match summary
     - Key matching points and gaps
     - Final binary determination (Match/No Match)
     - Explanation for the determination
   - Results are stored in the user's job match records.

6. **Result Storage**
   - Each Criteria maintains a subcollection of JobDetails.
   - Each JobDetail maintains a subcollection of Criteria.
   - User-specific match assessments are stored per user/job pair.

---

## User Experience

- Users can subscribe to multiple Criteria (searches) representing different job interests.
- Job results are personalized based on the user's subscriptions and match assessments.
- Users can view matches, non-matches (with explanations), and provide feedback.
- The system supports multiple job searches per user, with each search mapped to a shared Criteria.

---

## Implementation Notes

- **Criteria Deduplication**: Use normalized, slugified query+GeoID for uniqueness.
- **Subscription Logic**: On user search creation, check for existing Criteria; subscribe or create as needed.
- **Job Linking**: Maintain many-to-many links between Criteria and JobDetail in Firestore.
- **Feature Extraction**: Use LLMs for both job and user statement feature extraction and embedding.
- **Vector Search**: Integrate with Pinecone for dense and sparse vector search.
- **Scheduling**: Implement a scheduler that only searches Criteria with active subscribers.
- **Extensibility**: The model supports future enhancements (e.g., more advanced filters, ML-based recommendations).

---

## Future Enhancements

- Advanced filtering (compensation, company size, etc.)
- Personalized recommendations using ML
- Multi-stage LLM analysis
- Historical match tracking and analytics
- Enhanced user feedback integration

