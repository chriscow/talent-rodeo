
# Onboarding

user logs in via LinkedIn Oauth (``assets/templates/pages/home.html``)

## Resume Upload & Processing
- user uploads resume and enters phone number (`assets/templates/pages/profile-resume-upload.html`)
- system validates the resume to prevent fraud (`internal/resume.go` AnnotateResume)
- system annotates the resume creating a YAML summary with LLM comments (`internal/resume.go` AnnotateResume)
- system extracts preliminary search terms from resume to use in conversation with user (`internal/resume.go` getResumeSearchAnalysis)
- renders page "Call Me Now"  (`assets/templates/pages/profile-schedule-phone-call.html`)


## Outbound Call Scheduling 
- user clicks "Call Me Now" button.  Page posts to `/profile/schedule/call-now` (`cmd/web/handlers.schedule.go` callNow)
- handler creates an appointment (`internal/appointment.go`)
- handler creates a webhook task (`CreateAndEnqueueTask`) to callback url in `VAPI_START_CALL_PATH` ()
- handler redirects to `/profile`
- task processor processes webhook task at url `VAPI_START_CALL_PATH` (`/vapi/outbound-call` handler: `cmd/web/handlers.vapi.outbound-call.go` makeOutboundCall)
    - Call.Name: appointment.ID
    - Call.Customer.Name: user.ID,


## End of Call Report Processing 
- when call is finished, VAPI calls webhook with `end-of-call` on route: `/webhooks/vapi` (handler: `cmd/web/handlers.vapi.webhooks.go` handleVapiWebhook)
- webhook handler calls `cmd/web/handlers.appointment.go` processCompletedAppointment
- processCompletedAppointment sets CallID and various other fields and updates Firestore
- starts background task `transcript.ProcessCompletedAppointment` (`internal/transcript/processor.go`)


## Completed Appointment Processing
(`internal/transcript/processor.go`)
- set user status to `UserStatusCallProcessing`
- set appointment status to `AppointmentStatusProcessing` (if the server restarts, this status means it will get picked up and continued)
- get the transcript from VAPI using appointment `CallID` `vapi.GetCall()`
- create transcript extract using LLM via `extract()`
- extract work preferences using LLM via `getWorkPreferences()`
- extract search queries using LLM via `getSearchQueries()`
- generate user statement using LLM via `generateUserStatement()`
- generate job description using LLM via `getJobDescription()`
- finally, subscribe user to the search criteria via `subscribeSearchCriteria()`

> **NOTE** `subscribeSearchCriteria()` is not implemented


# Search

// SearchJob represents a queued job search with a provider.
type SearchJob struct {
	ID        string
	Criteria  SearchCriteria
	Provider  string
	Status    string
	Error     error
	CreatedAt time.Time
	UpdatedAt *time.Time
}

## Search Jobs
(`internal/search_job.go`)
- `SearchJob` is a Firestore document that represents a job search.  It is used to store the search criteria, the search provider name, and the status of the search.
- When a search gets queued, we first save a `SearchJob` document.  Then we enqueue a task to process the search.
- to recover from a server restart, we look for all `SearchJob` documents that have a status of `SearchJobStatusQueued` or `SearchJobStatusProcessing` and requeue them. (see `internal/search/scheduler.go` `requeueIncompleteSearches`)


## Search Scheduler
(`internal/search/scheduler.go`)

There is a search processor that uses a timer.Tick to check if any searches need
to happen.  We should probably change this where if we have "nothing" scheduled 
to search, we query for all search criteria and determien the date/time of the next search and sleep until then.

## Search Criteria
(`internal/criteria.go`)
- `SearchCriteria` are stored as Firestore documents in the `search_criteria` top level collection.  SearchCriteria define the query to search for and the location.  These two fields define a unique search criteria
- User's subscribe to a SearchCriteria.  When new search results are available for a subscribed criteria, those results get queued for per-user processing.

- If a criteria has no subscribers, it does not get searched.

If a new criteria is added, it will get searched immediately and then put into the
search rotation.

If a user newly subscribes to a criteria, that user will get processed immediately
from a previous search.

## The Search Provider (ScrapingDog)
`internal/search/providers/scrapingdog`
- The ScrapingDog provider, by default, retrieves new jobs that were posted in the last 24 hours.
- Searches are queued to the provider and there is a semiphore that limits the number of concurrent searches that can be run to prevent rate limiting (see `internal/search/providers/workers.go` `WorkerManager` struct)
- Searches are cached (see `internal/search/providers/provider.go` search.JobCache)
-

## The Job Search Sink
The search provider uses an interface `JobEventHandler`.  When search results are 
found, various methods on the interface are called.

The main application will have initialized the search provider with the `JobSink` 
(`internal/job_sink.go`) which implements that interface.  

The `JobSink`, in a transaction, is responsible for taking the search results (`OnJobDetail`) as they come in and saving them to the Firestore collection. Additionally, it links the `JobDetail` with the `SearchCritera` that found it (see `LinkJobDetailToCriteriaTx`)

If a job with the provider's job id already exists, we update our copy of the job with the latest version and ensure it is linked to the criteria.

### IDEAS

- Should the `JobSink` post to a webhook URL to handle post processing? Or just a goroutine


# Think about this

## JobDetail Post Processor

- What happens to a new job when it is found? 
- Who should the sink notify once the job is saved? 
- What transformations need to happen to the job?

### IDEAS

- Vector embeddings (dense and sparse DONE)
- Vector steering (dense and sparse DONE)
- Rewrite in standard form
- Extract structured data
- We can do subscriber specific processing `OnSearchCompleted` (use SearchJob record)
- Similarity search against users Pinecone namespace?



## Subscription Processor

How do we keep from re-analyzing jobs for a user that have already been analyzed?
(maybe look at the `user_jobs` and just analyze jobs not in their set)

see `internal/subscription/processor` and `internal/subscription/README.md`




