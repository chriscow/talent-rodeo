## Questions About F004 Implementation:

1. **Integration with NATS Messaging**: The feature spec mentions LLM analysis via NATS messaging similar to F003a (Job Data Extraction), but doesn't clearly specify how the Job Matching feature should integrate with the existing NATS infrastructure. Should developers create a new NATS subject/stream for match assessments, or use the existing ones?

**1. ANSWER:** The feature should use the same NATS infrastructure as F003a (Job Data Extraction). There is a constant defined `/handlers/types.go` called `rodeoStream` that 
is the name of the stream. Please review the other handlers in `/internal/handlers` 
and especially `generic_handler.go` to understand how the NATS and LLM integration works.

2. **LLM Prompt Design**: What specific prompts should be used when calling the LLM for match assessment? The spec mentions using the `github.com/chriscow/minds` library but doesn't provide guidance on prompt templates or expected input/output formats.

**2. ANSWER:** There is a preliminary implementation in `/internal/handlers/assessment.go`. 
Please review this and provide me follow-up questions.

3. **Match Assessment Data Structure**: While the spec mentions database schemas for storing match results, it doesn't define the exact format of the match assessment data structure that will be returned by the LLM. What fields should be included in the match assessment response?

**3. ANSWER:** Review `/internal/handlers/assessment.go`. Please review this and provide me follow-up questions.

4. **Scoring Algorithm Implementation**: The spec mentions a weighted scoring algorithm (+10 for "Must Have" skills, +5 for "Preferred" skills), but doesn't clarify how to normalize scores to a 0-100% range. Is there a specific normalization formula to use?

**4. ANSWER:** that's a good question. Let's mention this fact in the spec and leave
it as an open question for now and we will answer it at the time of implementation.

5. **UI Components**: The spec mentions implementing a "basic search results dashboard template" but doesn't provide design mockups or detailed UI requirements. What specific UI components and layouts should be implemented?

**5. ANSWER:** I envision a grid of job cards with the following columns:
- Job Title
- Company Name
- Company Logo (if available)
- An LLM generated overview of the job and why it matches the user's profile
- A button to view the job details

At the top of the page in a non-scrolling header, we will have a status bar detailing
how many jobs we searched through, how many matches we found, and when the next search
will be run. (searches frequency is a function of the user's subscription tier)

6. **Preference Filtering Logic**: While the spec mentions filtering based on location, compensation, and remote work preferences, it doesn't specify the exact matching logic. For example, should location matching use exact matching, radius-based proximity, or something else?

**6. ANSWER:** Doing a LinkedIn search requires a `geoID` which is a location ID.
At the moment, we don't have a good way to get the geoID for a location. GeoIDs can 
represent a city, state, zip code or even a metro area. We need to figure out how to 
get the geoID for a location.  We need to leave this as an open question for now.
Please add this fact to the spec.

7. **Storage and Retrieval Flow**: The feature requires storing match results in database tables, but the flow between LLM analysis, database storage, and UI retrieval isn't clearly defined. What's the complete data flow from match calculation to UI display?

**7. ANSWER:** The LLM analysis is done in the `/internal/handlers/assessment.go` file.
It was only a prototype and likely needs adjusted.  The LLM analysis will result in 
a structured response that will be stored in the database.  Part of this structured 
response are various judgements on the match and an explaination as to why. We will also 
have another LLM instance that performs an audit and critisizes the first LLM's evaluation
and then a final LLM will generate a summary of the job, the match and why.

8. **Error Handling**: How should the system handle cases where the LLM fails to produce a valid match assessment or when filtering criteria result in no matches?

**8. ANSWER:** If I am logged in as a super user (see the 'users' table in the database),
I can see all jobs and matches and I can see the LLM's audit and summary. For quality 
purposes I need to be able to see the LLM's audit and summary for any match. We also 
need to do evals on the LLM's performance so it can be improved. This feature needs 
a specification written. Make a note to add this to the spec.

9. **Performance Considerations**: Given that this involves potentially processing large numbers of jobs with LLM analysis, are there specific performance targets or throttling mechanisms needed?

**9. ANSWER:** this is a valid question but I do not have an answer at this time.
We will need to see how the MVP performs and then we can make decisions on how to
improve or determine what is needed.

10. **Incremental Implementation Path**: Since all three subfeatures are partially implemented, what's the recommended order of implementation to maintain a shippable state throughout development?

**10. ANSWER:** We will implement "Match Result Display" first but stub out the 
functionality of the other two to support this. It is helpful to be able to see 
the results of the other using our own UI.
