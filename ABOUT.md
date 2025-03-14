We are building a web app for job seekers. The app uses LLMs to automate the process of reviewing job description and finding jobs that are ideally suited for the user.

There are many properties of a job and many properties of a candidate's preferences. Everything from the skills, knowledge, experience and educational requirements to preferred or nice to have. There are also candidate preferences for location, remote work, compensation, benefits and even culture!

Each of these can become a search term when searching for a job or somewhat of a filter to determine the fitness of a job match to a candidate's preferences.

We are going to make a web page that displays a list of search criteria (keywords or tags) as well as preferences for filtering. Each item will have sliders to indicate the degree that the item is either desireable or not desireable.  We will assume that if it's not in the list, the candidate doesn't have a preference.

The skills handler in internal/handlers/skills.go is responsible for extracting the skills from the resume and interview and storing them in the database. We will display each skill as a separate slider on the web page. The value of the slider indicates the strength of the candidate's desire for the skill in their search.

For example, we might have a search term called "Golang" with a slider with a range from 1 to 5.  A one means that, although the candidate has skills in Golang they do not want any jobs that use Golang and those should be filtered out.  A value of 5 means the job MUST require Golang as a primary requirement and we would use it as a search term.

Some items are not search terms and are purely filters or terms used to evaluate the desire for the term.  For instance, a particular industry may be desireable but not a must-have. Perhaps the candidate wants "Health Care" as an industry with a value of "4" because it is very desireable but not a must-have.

There will be other criteria we will consider such as "compensation".  If given, it must be in the range indicated by the candidate.

## Search Functionality

1. Manual First Search:
   - Once all of the preferences are initially set and verified by the user, they 
     can click a "Ready to Search" button to initiate an immediate search
   - Progress is tracked in real-time using a user-specific key-value store
   - Results are displayed on a dedicated progress page
   - Manual searches are no longer available after the first search is performed.
   - If the user updates their preferences, they can click a "Ready to Search" button 
     but the search will be performed on the next scheduled search.

2. Automatic Periodic Search:
   - After the first search is performed, the system automatically performs 
     searches based on the user's subscription tier:
     * Free Tier: Every three days
     * Basic Tier: Daily searches
     * Pro Tier: Four times daily searches
   - Only one search runs at a time per user
   - Each search uses the latest preferences and skills from the user's profile
   - Results are stored and accessible through the dashboard

## Search Process Details

When a user initiates a search, the following process occurs:

1. The search scheduler (internal/tasks/handlers/search_scheduler.go) is triggered
2. The scheduler does the following:
   - Determines the user's tier (free, mid, or top)
   - Calculates when the next search should run based on tier:
     * Free tier: Every 72 hours
     * Mid tier: Every 24 hours  
     * Top tier: Every 6 hours
   - Determines how many pages to search based on tier:
     * Free tier: 3 pages
     * Mid tier: 50 pages
     * Top tier: 200 pages
   - Creates and schedules search tasks for each page
   - Maintains state about the last page searched for continuity between searches

3. Search Criteria Formation:
   - Skills from the candidate_skills table are used to form search queries
   - Each skill has a preference rating (1-5) that determines its importance:
     * 5: Must Have skills - used as primary search keywords
     * 4: Preferred skills - used as secondary search keywords
     * 3: No Preference skills - may be included in search but not prioritized
     * 2: Avoid skills - used to filter out unwanted results
     * 1: Must Avoid skills - used to strictly filter out results

4. Search Execution:
   - The search.go handler receives search tasks via NATS messages
   - It works with search providers to fetch job listings for each page
   - Job descriptions are retrieved and stored in the database
   - For each saved job description, a NATS message is published for further processing

5. Job Analysis Process:
   - Requirements are extracted from job descriptions
   - Skills, experience levels, and other requirements are identified
   - The job is evaluated against the candidate's profile
   - The assessment.go handler performs comprehensive candidate-job matching
   - A match score is calculated based on skills, experience, and preferences

Once all of the preferences are set, the user can click a button to search for jobs. The app will primarily use the skills to search for jobs. A search is started by sending a message to NATS. Review the internal/handlers/search.go file for the details.

The search will store the search result (`rodeo.search.result`). Results are automatically pruned based on tier limits:
- Free Tier: First 3 pages analyzed
- Mid Tier: Up to 50 pages
- Top Tier: Up to 200 pages

## Search Progress Tracking

Each search, whether manual or automatic, is tracked using a dedicated key-value store that:
- Shows real-time progress of job analysis
- Displays counts of total jobs found and good matches
- Automatically cleans up after completion
- Prevents duplicate searches from running simultaneously

This ensures users always have visibility into their search status while maintaining system efficiency.