# F004 Job Matching - Developer Questions

## Location Preferences (SF004.1)
- How will the system handle jobs with ambiguous locations or multiple locations?

ANSWER: The LinkedIn search via ScrapingDog requires a location in order to perform 
a search. It is expected that search results returned are already filtered for
the location selected by the user.

- Is the "top 5 metro areas" list defined somewhere? Where will the GeoIDs come from?

ANSWER: The top 5 metro areas will be defined by the product owner and provided
to the developer. The developer, in the mean time, should create a hard coded map 
of the top 5 metro areas from their own knowledge and make up placeholder corresponding GeoIDs 
in the mean time, including a "Remote" entry.

- For the "Fully Remote" option, how will the system identify remote jobs in search results?

ANSWER: The LinkedIn search via ScrapingDog accepts a specific "Remote" geoID.


## Match Scoring Algorithm (SF004.3)
- What is the specific implementation plan for the `jobHasSkill` function using an LLM?

ANSWER: The `jobHasSkill` function will be a simple LLM prompt that takes in a skill and the job description. The LLM will return a structured JSON response that returns true if explicitly stated 
(or a synonym thereof i.e. "Go" or "Golang") in the job description.

- Is the 30% threshold for job display based on research or is it an arbitrary starting point?

ANSWER: The 30% threshold is an arbitrary starting point. Make it a constant that is easy to find and change.

- Should there be a maximum number of jobs to process for each user to manage performance?

ANSWER: Yes, the maximum number of jobs to process for each user is defined by how 
many "pages" of job listings are defined for the user's tier.  If they are a free user,
they will only be able to search 3 pages of job listings at a time. A paid user will be
able to search 50 pages of job listings at a time.

## LLM Match Assessment (SF004.4)
- How will the system handle LLM cost and rate limits? Is there a budget per user?

ANSWER: LLMs are rate limited by the total number of tokens processed within a time window.
For now, we will let the LLM API handle the rate limiting and let us know if we hit a limit. 
We should exponentially back off if we hit a rate limit.

## Job Card UI (SF004.5)
- What is the behavior if no new jobs are available after a user votes on a job?

ANSWER: The system will show a message to the user that no new jobs are available.

- Are there mockups or wireframes for the job card UI?

ANSWER: No, you can provide a preliminary job card UI at development time.


## Results Dashboard (SF004.6)
- What are the specific requirements for the super user view?

ANSWER: The super user view will contain any and all information about the user's job search history.

- How will the system determine the "Next scheduled search time" for free vs. paid users?

ANSWER: The next schedule search time for a free user is 3 days from the last search. The next schedule search time for a paid user is 1 day from the last search.

## General Questions
- How will we monitor and analyze the effectiveness of the job matching feature?

ANSWER: We will monitor and analyze the effectiveness of the job matching feature by 
looking at the user's feedback on the job cards.

- Has the LLM prompt been tested for consistency and quality of results?

ANSWER: No, the LLM prompt has not been tested for consistency and quality of results.
