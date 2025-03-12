We are building a web app for job seekers. The app uses LLMs to automate the process of reviewing job description and finding jobs that are ideally suited for the user.

There are many properties of a job and many properties of a candidate's preferences. Everything from the skills, knowledge, experience and educational requirements to preferred or nice to have. There are also candidate preferences for location, remote work, compensation, benefits and even culture!

Each of these can become a search term when searching for a job or somewhat of a filter to determine the fitness of a job match to a candidate's preferences.

We are going to make a web page that displays a list of search criteria (keywords or tags) as well as preferences for filtering. Each item will have sliders to indicate the degree that the item is either desireable or not desireable.  We will assume that if it's not in the list, the candidate doesn't have a preference.

The skills handler in internal/handlers/skills.go is responsible for extracting the skills from the resume and interview and storing them in the database. We will display each skill as a separate slider on the web page. The value of the slider indicates the strength of the candidate's desire for the skill in their search.

For example, we might have a search term called "Golang" with a slider with a range from 1 to 5.  A one means that, although the candidate has skills in Golang they do not want any jobs that use Golang and those should be filtered out.  A value of 5 means the job MUST require Golang as a primary requirement and we would use it as a search term.

Some items are not search terms and are purely filters or terms used to evaluate the desire for the term.  For instance, a particular industry may be desireable but not a must-have. Perhaps the candidate wants "Health Care" as an industry with a value of "4" because it is very desireable but not a must-have.

There will be other criteria we will consider such as "compensation".  If given, it must be in the range indicated by the candidate.

Once all of the preferences are set, the user can click a button to search for jobs.  The app will primarily use the skills to search for jobs. A search is started by sending a message to NATS.  Review the internal/handlers/search.go file for the details.

The search will store the search result (`rodeo.search.result`).