package scrapingdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/utils"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
)

// initializeWorker starts a single worker goroutine and returns its ID
func (p *Provider) initializeWorker(ctx context.Context, workerID int) {
	p.logger.Debug("scrapingdog: starting worker", "worker_id", workerID)
	go func() {
		for {
			select {
			case <-ctx.Done():
				p.logger.Debug("scrapingdog: worker stopping due to context cancellation", "worker_id", workerID)
				return
			case search := <-p.searchQueue:
				p.logger.Debug("scrapingdog: worker received search request",
					"worker_id", workerID,
					"user_id", search.UserID,
					"keywords", search.Criteria.Keywords,
					"location", search.Criteria.Location)
				if err := p.processQueuedSearch(ctx, search); err != nil {
					if err == context.Canceled {
						p.logger.Debug("scrapingdog: search processing cancelled",
							"worker_id", workerID,
							"user_id", search.UserID,
							"error", err)
					} else {
						p.logger.Error("failed to process queued search",
							"worker_id", workerID,
							"user_id", search.UserID,
							"error", err)
					}
				} else {
					p.logger.Debug("scrapingdog: worker completed search request",
						"worker_id", workerID,
						"user_id", search.UserID)
				}
			}
		}
	}()
}

// processQueuedSearch handles the complete job search flow for a queued search request
func (p *Provider) processQueuedSearch(ctx context.Context, qs QueuedSearch) error {
	// Check if context is already canceled before starting work
	if ctx.Err() != nil {
		return ctx.Err()
	}

	p.logger.Debug("scrapingdog: processing queued search",
		"user_id", qs.UserID,
		"keywords", qs.Criteria.Keywords,
		"location", qs.Criteria.Location)

	// Process job listings
	listings, err := p.processJobListings(ctx, qs.Criteria)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		p.notifySearchErrors(ctx, qs.UserID, 0, []error{err})
		return fmt.Errorf("failed to process job listings: %w", err)
	}

	// Generate search hash before processing job details
	searchHash := p.store.GetSearchHash(qs.Criteria.Keywords, qs.Criteria.Location)

	// Process each job's details
	var processingErrors []error
	newJobCount := 0

	testing := os.Getenv("TESTING_MODE") == "true"
	if testing {
		listings = listings[:3]
	}

	for _, listing := range listings {
		if err := p.processJobDetailsAndSave(ctx, listing, searchHash, qs.UserID, qs.Criteria); err != nil {
			processingErrors = append(processingErrors, err)
			p.logger.Error("failed to process job details",
				"error", err,
				"user_id", qs.UserID,
				"job_id", listing.ProviderJobID)
			continue
		}
		newJobCount++
	}

	// If we had any errors during processing, include them in completion notification
	if len(processingErrors) > 0 {
		// Update user's search status with error
		errMsg := fmt.Sprintf("failed to process %d jobs", len(processingErrors))
		if err := p.updateUserSearchError(ctx, qs.UserID, errMsg); err != nil {
			p.logger.Error("failed to update user search error status",
				"error", err,
				"user_id", qs.UserID)
		}

		// Notify completion with errors
		p.notifySearchErrors(ctx, qs.UserID, newJobCount, processingErrors)
		return fmt.Errorf("failed to process %d jobs: %v", len(processingErrors), processingErrors)
	}

	// Final context check before updating completion
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Update user's search completion status
	p.logger.Debug("scrapingdog: updating user search completion status", "user_id", qs.UserID)
	if err := p.updateUserSearchCompletion(ctx, qs.UserID); err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return fmt.Errorf("failed to update user search completion: %w", err)
	}

	// Notify successful completion
	p.notifySearchComplete(ctx, qs.UserID, newJobCount)

	p.logger.Debug("scrapingdog: search processing completed successfully",
		"user_id", qs.UserID,
		"total_jobs", newJobCount)

	return nil
}

// processJobDetailsAndSave handles the processing of individual job details and saving to user jobs
func (p *Provider) processJobDetailsAndSave(ctx context.Context, listing search.JobListing, searchHash string, userID string, criteria search.Criteria) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.logger.Debug("scrapingdog: processing job details",
		"user_id", userID,
		"job_id", listing.ProviderJobID,
		"criteria", criteria)

	if err := p.processJobDetails(ctx, listing.ProviderJobID, listing.URL); err != nil {
		return fmt.Errorf("job %s: %w", listing.ProviderJobID, err)
	}

	// this is the full job ID with provider prefix that is in Fires
	fullJobID := funcs.Slugify(fmt.Sprintf("%s %s", providerName, listing.ProviderJobID))

	// Get the job details to get company info
	jobDetails, err := p.store.GetCachedJobDetails(ctx, fullJobID)
	if err != nil {
		return fmt.Errorf("failed to get cached job details for %s: %w", listing.ProviderJobID, err)
	}

	// Create user job entry
	userJob := &internal.UserJob{
		ID:         fullJobID,
		SearchHash: searchHash,
		Status:     internal.JobStatusUnprocessed, // Set to unprocessed initially, will be set to "new" after analysis
		JobRef:     p.fs.Collection("job_details").Doc(fullJobID),
		Criteria:   criteria,
	}

	// Set company info from job details
	if jobDetails != nil {
		userJob.CompanyName = jobDetails.Company
		if jobDetails.CompanyInfo != nil {
			userJob.CompanyLogo = jobDetails.CompanyInfo.LogoURL
		}
	}

	if err := internal.CreateUserJob(ctx, p.fs, userID, userJob); err != nil {
		return fmt.Errorf("failed to save user job %s: %w", listing.ProviderJobID, err)
	}

	p.notifySearchNewJob(ctx, userID, fullJobID)

	return nil
}

func (p *Provider) notifySearchErrors(_ context.Context, userID string, totalJobs int, errors []error) {
	if p.eventHandler != nil {
		if err := p.eventHandler.OnSearchCompleted(userID, totalJobs, errors); err != nil {
			p.logger.Error("failed to notify user of error",
				"error", err,
				"user_id", userID)
		}
	}
}

func (p *Provider) notifySearchComplete(_ context.Context, userID string, totalJobs int) {
	if p.eventHandler != nil {
		if err := p.eventHandler.OnSearchCompleted(userID, totalJobs, nil); err != nil {
			p.logger.Error("failed to notify user of error",
				"error", err,
				"user_id", userID)
		}
	}
}

func (p *Provider) notifySearchNewJob(_ context.Context, userID, jobID string) {
	if p.eventHandler != nil {
		if err := p.eventHandler.OnNewJob(userID, jobID); err != nil {
			p.logger.Error("failed to notify user of new jobs",
				"error", err,
				"user_id", userID)
		}
	}
}

// processJobDetails gets and stores job details
func (p *Provider) processJobDetails(ctx context.Context, jobID string, url string) error {
	var details *search.JobDetail
	var err error

	// Create the full job ID with provider prefix
	fullJobID := funcs.Slugify(fmt.Sprintf("%s %s", providerName, jobID))

	p.logger.Debug("scrapingdog: checking job details in cache",
		"provider_job_id", jobID,
		"full_job_id", fullJobID)

	// First check cache using the full job ID
	details, err = p.store.GetCachedJobDetails(ctx, fullJobID)
	if err != nil {
		return fmt.Errorf("failed to get cached job details: %w", err)
	}

	// If not in cache, get from ScrapingDog
	if details == nil {
		p.logger.Debug("scrapingdog: job details not found in cache, fetching from API",
			"provider_job_id", jobID,
			"full_job_id", fullJobID)
		details, err = p.GetJobDetails(ctx, jobID)
		if err != nil {
			return fmt.Errorf("failed to get job details: %w", err)
		}

		// Set the URL from the job listing since it's not available in the details response
		if url != "" {
			details.URL = url
		}
	} else {
		p.logger.Debug("scrapingdog: job details found in cache",
			"provider_job_id", jobID,
			"full_job_id", fullJobID)

		// If we have a URL and the cached details don't, update it
		if url != "" && details.URL == "" {
			details.URL = url
		}
	}

	// Check if the job details already have a summary
	if details.ExtractedData == nil {
		p.logger.Debug("scrapingdog: generating summary for job details",
			"provider_job_id", jobID,
			"full_job_id", fullJobID)

		// Generate summary
		if err := p.summarizeJobDetails(ctx, details); err != nil {
			p.logger.Warn("failed to summarize job details",
				"job_id", fullJobID,
				"error", err)
			// Continue without summary rather than failing the whole operation
		}
	}

	// Store in both Firestore and Cloudflare KV
	if err := p.store.SaveJobDetails(ctx, details); err != nil {
		return fmt.Errorf("failed to save job details: %w", err)
	}

	return nil
}

// SearchJobListings searches for jobs based on the provided criteria
func (p *Provider) SearchJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	// Initialize store if needed
	if p.fs != nil && p.store == nil {
		p.logger.Debug("scrapingdog: initializing job store")
		store, err := NewJobStore(ctx, p.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize job store: %w", err)
		}
		p.store = store
		p.logger.Debug("scrapingdog: job store initialized successfully")
	}

	if p.store != nil {
		p.logger.Debug("scrapingdog: checking cache for job listings",
			"keywords", criteria.Keywords,
			"location", criteria.Location)
		cachedJobs, err := p.store.GetCachedJobs(ctx, criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to get cached jobs: %w", err)
		}
		if len(cachedJobs) > 0 {
			p.logger.Debug(internal.Green("scrapingdog: found cached job listings"),
				"num_jobs", len(cachedJobs))
			return cachedJobs, nil
		}
		p.logger.Debug("scrapingdog: no cached job listings found")
	}

	p.logger.Debug("scrapingdog: performing job search request",
		"keywords", criteria.Keywords,
		"location", criteria.Location)
	jobs, err := p.getJobListings(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs: %w", err)
	}

	p.logger.Debug("scrapingdog: job search request completed",
		"num_jobs", len(jobs))

	if p.store != nil {
		p.logger.Debug("scrapingdog: saving job listings to cache",
			"num_jobs", len(jobs))
		err := p.store.SaveJobListings(ctx, criteria, jobs)
		if err != nil {
			return nil, fmt.Errorf("failed to save jobs: %w", err)
		}
		p.logger.Debug("scrapingdog: job listings saved to cache successfully")
	}
	return jobs, nil
}

// getJobListings searches for jobs based on the provided criteria
func (p *Provider) getJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	// Acquire semaphore with context
	select {
	case p.searchSemaphore <- struct{}{}:
		defer func() { <-p.searchSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	p.logger.Debug("scrapingdog: getJobListings", "criteria", criteria)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)

	// We need to verify that "field" is not empty
	if len(criteria.Keywords) == 0 {
		return nil, fmt.Errorf("keywords are required")
	}

	// Handle keywords
	if len(criteria.Keywords) > 0 {
		params.Set("field", strings.Join(criteria.Keywords, " "))
	}

	// Handle location

	// Handle location
	if criteria.Location != "" {
		geoid, err := internal.GetGeoID(ctx, p.fs, criteria.Location)
		if err != nil || geoid.Value == "" {
			p.logger.Warn(internal.BGYellow("failed to get geo id"), "location", criteria.Location, "error", err)
			params.Set("geoid", string(remoteGEOID))
		} else {
			params.Set("geoid", string(geoid.Value))
		}
	} else {
		params.Set("geoid", string(remoteGEOID))
	}

	// Handle pagination
	if criteria.PageNumber > 0 {
		params.Set("page", fmt.Sprintf("%d", criteria.PageNumber))
	} else {
		params.Set("page", "1")
	}

	// Handle sort by date posted
	if criteria.SortBy != "" {
		params.Set("sort_by", criteria.SortBy)
	}

	// Handle job type - take first one if multiple are specified
	if len(criteria.JobType) > 0 {
		params.Set("job_type", criteria.JobType[0])
	}

	// Handle experience level - take first one if multiple are specified
	if len(criteria.Experience) > 0 {
		params.Set("exp_level", criteria.Experience[0])
	}

	// Handle work type (remote/office/hybrid)
	if criteria.WorkType != "" {
		params.Set("work_type", criteria.WorkType)
	}

	// Handle company filter
	if criteria.CompanyID != "" {
		params.Set("company_id", criteria.CompanyID)
	} else {
		params.Set("page", "1")
	}

	// Handle sort by date posted
	if criteria.SortBy != "" {
		params.Set("sort_by", criteria.SortBy)
	}

	// Handle job type - take first one if multiple are specified
	if len(criteria.JobType) > 0 {
		params.Set("job_type", criteria.JobType[0])
	}

	// Handle experience level - take first one if multiple are specified
	if len(criteria.Experience) > 0 {
		params.Set("exp_level", criteria.Experience[0])
	}

	// Handle work type (remote/office/hybrid)
	if criteria.WorkType != "" {
		params.Set("work_type", criteria.WorkType)
	}

	// Handle company filter
	if criteria.CompanyID != "" {
		params.Set("company_id", criteria.CompanyID)
	}

	// Make request
	url := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request with keywords=%v location=%s: %w",
			criteria.Keywords, criteria.Location, err)
	}

	p.logger.Info(internal.BGMagenta("scrapingdog: getJobListings: request"), "keywords", criteria.Keywords, "location", criteria.Location, "url", url)
	resp, err := p.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request with keywords=%v location=%s: %w",
			criteria.Keywords, criteria.Location, err)
	}
	defer resp.Body.Close()

	p.logger.Debug("scrapingdog: getJobListings: response received", "status_code", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error with keywords=%v location=%s status_code=%d: %w",
			criteria.Keywords, criteria.Location, resp.StatusCode, handleAPIError(resp.StatusCode))
	}

	var listings jobListings
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, fmt.Errorf("failed to decode response with keywords=%v location=%s: %w",
			criteria.Keywords, criteria.Location, err)
	}

	if len(listings) == 0 {
		p.logger.Warn(internal.BGYellow("== scrapingdog: no job listings found =="), "keywords", criteria.Keywords, "location", criteria.Location)
	} else {
		p.logger.Debug(internal.Green("scrapingdog: found job listings"), "num_listings", len(listings), "keywords", criteria.Keywords, "location", criteria.Location)
	}

	// Convert to provider jobListings
	jobListings, err := convertJobListings(listings)
	if err != nil {
		return nil, fmt.Errorf("failed to convert job listings with keywords=%v location=%s: %w",
			criteria.Keywords, criteria.Location, err)
	}

	return jobListings, nil
}

// GetJobDetails retrieves detailed information about a specific job
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	p.logger.Debug("scrapingdog: getting job details", "job_id", jobID)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("job_id", jobID)

	// Make request
	url := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for job_id=%s: %w", jobID, err)
	}

	p.logger.Info(internal.BGMagenta("scrapingdog: GetJobDetails: request"), "job_id", jobID, "url", url)
	resp, err := p.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request for job_id=%s: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error for job_id=%s with status_code=%d: %w", jobID, resp.StatusCode, handleAPIError(resp.StatusCode))
	}

	var details []jobDetail
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode response for job_id=%s: %w", jobID, err)
	}

	if len(details) == 0 {
		return nil, fmt.Errorf("job not found with job_id=%s", jobID)
	}

	detail := details[0]
	postedTime, _ := time.Parse("2006-01-02", detail.PostingTime)

	// Parse salary range
	var salaryInfo *search.SalaryRange
	if detail.BasePay != "" {
		salaryInfo, err = parseSalaryRange(ctx, detail.BasePay)
		if err != nil {
			return nil, fmt.Errorf("failed to parse salary range: %w", err)
		}
	}

	// Convert recruiters
	recruiters := make([]search.Recruiter, len(detail.Recruiters))
	for i, r := range detail.Recruiters {
		recruiters[i] = search.Recruiter{
			Photo:      r.Photo,
			Name:       r.Name,
			Title:      r.Title,
			ProfileURL: r.ProfileURL,
		}
	}

	// Convert similar jobs
	similarJobs := make([]search.RelatedJob, len(detail.SimilarJobs))
	for i, j := range detail.SimilarJobs {
		similarJobs[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert people also viewed
	peopleAlsoViewed := make([]search.RelatedJob, len(detail.PeopleAlsoViewed))
	for i, j := range detail.PeopleAlsoViewed {
		peopleAlsoViewed[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert to provider job detail
	jobDetail := &search.JobDetail{
		// Basic job information
		ID:            funcs.Slugify(fmt.Sprintf("%s %s", providerName, jobID)),
		Provider:      providerName,
		ProviderJobID: jobID,
		Title:         detail.Position,
		Company:       detail.CompanyName,
		Location:      detail.Location,
		PostedDate:    postedTime,
		URL:           "", // Will be populated from job listing URL in processJobDetails

		// Detailed information
		FullDescription: detail.Description,
		SalaryInfo:      salaryInfo,
		EmploymentType:  detail.EmploymentType,
		Industry:        detail.Industries,
		CompanyInfo: &search.Company{
			Name:     detail.CompanyName,
			Industry: detail.Industries,
		},

		// Additional fields from ScrapingDog
		Status:           detail.Status,
		SeniorityLevel:   detail.SeniorityLevel,
		JobFunction:      detail.JobFunction,
		Recruiters:       recruiters,
		SimilarJobs:      similarJobs,
		PeopleAlsoViewed: peopleAlsoViewed,
	}

	// Generate job summary
	if err := p.summarizeJobDetails(ctx, jobDetail); err != nil {
		p.logger.Warn("failed to summarize job details", "job_id", jobID, "error", err)
		// Continue without summary rather than failing the whole operation
	}

	return jobDetail, nil
}

// updateUserSearchError updates the user's search status with an error
func (p *Provider) updateUserSearchError(ctx context.Context, userID string, errMsg string) error {
	_, err := p.fs.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "Search.Error", Value: errMsg},
		{Path: "Search.CompletedAt", Value: time.Now()},
		{Path: "Search.Status", Value: "error"},
	})
	return err
}

// summarizeJobDetails uses LLM to create a job summary directly on the JobDetail
func (p *Provider) summarizeJobDetails(ctx context.Context, jobDetail *search.JobDetail) error {
	p.logger.Debug("scrapingdog: summarizing job details", "job_id", jobDetail.ID)

	// Create OpenAI provider with GPT-4o model
	llm, err := openai.NewProvider()
	if err != nil {
		return fmt.Errorf("failed to create openai provider: %w", err)
	}

	// Create system prompt from template
	const summaryFile = "prompts/job-summary.001.txt"
	systemPrompt, err := utils.CreateTemplate(summaryFile)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	// Execute template with job details
	jobJSON, err := json.MarshalIndent(jobDetail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal job details: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleSystem,
			Content: systemPrompt.RawContent,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please summarize this job listing:\n\n%s", string(jobJSON)),
		},
	}

	schema, err := minds.NewResponseSchema("job_summary", "job_summary.001.txt", search.JobSummary{})
	if err != nil {
		return fmt.Errorf("failed to create response schema: %w", err)
	}

	response, err := llm.GenerateContent(ctx, minds.NewRequest(messages, minds.WithResponseSchema(*schema)))
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	var summary search.JobSummary
	if err := json.Unmarshal([]byte(response.String()), &summary); err != nil {
		return fmt.Errorf("failed to unmarshal summary: %w", err)
	}

	// Store the summary directly on the JobDetail
	jobDetail.ExtractedData = &summary

	return nil
}
