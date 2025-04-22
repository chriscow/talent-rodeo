# F004 Developer Audit: Job Matching (Refactored)

## Overview

This audit assesses the F004 Job Matching feature specification for completeness, clarity, and alignment with MVP goals. The feature describes a refactored approach to job matching that aims to improve scalability, cost efficiency, and extensibility through centralized search criteria and deduplication of searches.

## Strengths

1. **Clear Value Proposition**: The specification clearly articulates the primary benefits of the refactored approach, including deduplication of searches, efficient job result sharing, and improved scalability.

2. **Well-Defined Data Model**: The data structures for Criteria, User, and JobDetail are well defined with clear relationships between them.

3. **Focused MVP Reminder**: The document begins with an important reminder that this is an MVP, which helps keep the implementation grounded.

4. **Complete Pipeline Definition**: The job matching pipeline is broken down into clear, sequential steps that are easy to understand.

5. **Future Enhancements Section**: The spec explicitly identifies features that are out of scope for the MVP, which helps prevent scope creep.

## Follow-Up Questions

1. **Implementation Timeline and Priorities**:
   - What are the priority components to implement first? Is there a phased implementation approach?

ANSWER: follow up question: Do you have a suggestion on how to incrementally implement this so we can
develop it iteratively without breaking current existing functionality?

2. **Data Model Details**:
   - For the `ID` field in Criteria, what specific slugification algorithm should be used to ensure consistency?

    ANSWER: a criterial will be unique based on "query" and "location" (geoid)

   - How will the system handle changes to a Criteria after users have already subscribed to it?

    ANSWER: The criteria should be consider immutable.  If there is a change, it becomes a new criteria

   - Is there a maximum number of Criteria a user can subscribe to?

   ANSWER: No

3. **Search & Scheduling Specifics**:
   - What is the exact scheduling mechanism for the regular checks (cron job, timer, etc.)?

    ANSWER: the "cmd/web/search" service already has a timer ticker to check for searches.

   - How should the system handle rate limiting from the search provider (e.g., ScrapingDog)?

    ANSWER:  The internal/search/providers/scrapingdog implementation already handles and manages rate limiting

   - What is the retry policy for failed searches?

   ANSWER: The `search.Criteria` LastSearchedAt field will not be updated if it didn't complete. It will get caught and searched on the next go around.

4. **Feature Extraction & Embedding**:
   - Which specific LLM provider and model should be used for feature extraction?

    ANSWER: OpenAI (we can add to internal/openai.go)

   - What is the exact format for storing extracted features and embeddings?

    ANSWER: I don't know yet.  We will need to discuss at implementation time. I think extracted features will be an array of strings. Embeddings is an array of floats. Those can be stored as-is in Firestore.

   - Are there performance requirements for the LLM processing time?

   ANSWER: No

   - How should the system handle LLM failures or rate limiting?

    ANSWER: Yes

5. **Preliminary Matching Thresholds**:
   - What are the specific similarity thresholds for the preliminary vector matching?

   ANSWER: cosine similarity of .9 or better.  Should be defined as a constant.

   - How many jobs should proceed to the more expensive LLM assessment stage?

   ANSWER: all maches with above threshold

   - How is "fast vector and keyword matching" specifically implemented?

   ANSWER: Send the embedding of the user's synthesized job description to pinecone to 
   search for JobDetail ExtractedData.Summary embedding. Also use sparse vector matching (two queries)
   We need to review the API documentation at Pinecone to see how sparse vectors work.

6. **Error Handling & Edge Cases**:
   - How should the system handle errors in each step of the pipeline?

    ANSWER: Let's drill down on the possible errors that can happen and where do we can figure this out

   - What happens when a user subscribes to a Criteria that has no matching jobs?

   ANSWER: If the criteria has no jobs and it has been searched, that specific criteria simply matches nothing

   - How are job listings that are no longer available handled?

    ANSWER: We need to modify our JobDetail firestore implementation to have a TTL and have them expire in
    30 days. If its no longer available, the user will see that if they follow through.


## Suggestions for Simplification

1. **Phased LLM Integration**: Consider implementing a simpler first phase that uses keyword matching only, before adding the more complex vector embedding and LLM assessment components. This would allow for earlier testing of the core functionality.

ANSWER: AGREE

2. **Reduce Initial Relationship Complexity**: For the MVP, consider simplifying the many-to-many relationship implementation. Store the job IDs directly in the Criteria document for smaller datasets, and only implement subcollections if/when scale demands it.

ANSWER: How does firestore handle `job_details` documents that expire via TTL when other documents reference them?

3. **Simplified Preliminary Matching**: For the MVP, consider using a simpler keyword-based matching approach without vector embeddings. Vector search can be added in a subsequent iteration once the basic functionality is validated.

ANSWER: AGREE

4. **Manual Feature Extraction**: For early testing, consider manually extracting features for a small set of jobs rather than implementing the full LLM pipeline immediately.

ANSWER: NO.  This is easy enough to accomplish with an LLM.  I will show you.

5. **Fixed Schedule**: Start with a simple fixed schedule (e.g., daily) rather than implementing dynamic scheduling based on popularity metrics.

ANSWER: AGREE
