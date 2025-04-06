package scrapingdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"time"

	"cloud.google.com/go/firestore"
)

// initializeWorker starts a single worker goroutine and returns its ID
func (p *Provider) initializeWorker(ctx context.Context, workerID int) {
	p.logger.Debug("starting worker", "worker_id", workerID)
	go func() {
		for {
			select {
			case <-ctx.Done():
				p.logger.Debug("worker stopping due to context cancellation", "worker_id", workerID)
				return
			case search := <-p.searchQueue:
				p.logger.Debug("worker received search request",
					"worker_id", workerID,
					"user_id", search.UserID,
					"keywords", search.Criteria.Keywords,
					"location", search.Criteria.Location)
				if err := p.processQueuedSearch(ctx, search); err != nil {
					if err == context.Canceled {
						p.logger.Debug("search processing cancelled",
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
					p.logger.Debug("worker completed search request",
						"worker_id", workerID,
						"user_id", search.UserID)
				}
			}
		}
	}()
}

// // StartSearchWorkers starts background workers to process the search queue
// func (p *Provider) StartSearchWorkers(ctx context.Context, numWorkers int) {
// 	p.logger.Info("starting search workers", "num_workers", numWorkers)
// 	for i := 0; i < numWorkers; i++ {
// 		workerID := i
// 		p.initializeWorker(ctx, workerID)
// 	}
// }

// processJobListings handles the retrieval and processing of job listings
// func (p *Provider) processJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
// 	var jobs []search.JobListing
// 	var err error

// 	// First check cache
// 	if p.store != nil {
// 		p.logger.Debug("checking cache for jobs")
// 		jobs, err = p.store.GetCachedJobs(ctx, criteria)
// 		if err != nil {
// 			if errors.Is(err, context.Canceled) {
// 				return nil, context.Canceled
// 			}
// 			return nil, fmt.Errorf("failed to get cached jobs: %w", err)
// 		}
// 		if len(jobs) > 0 {
// 			p.logger.Debug("found cached jobs", "num_jobs", len(jobs))
// 			return jobs, nil
// 		}
// 		p.logger.Debug("no cached jobs found")
// 	}

// 	// If no cached jobs, perform the search
// 	if len(jobs) == 0 {
// 		if ctx.Err() != nil {
// 			return nil, ctx.Err()
// 		}

// 		p.logger.Debug("performing job search", "keywords", criteria.Keywords)
// 		jobs, err = p.SearchJobListings(ctx, criteria)
// 		if err != nil {
// 			if errors.Is(err, context.Canceled) {
// 				return nil, context.Canceled
// 			}
// 			return nil, fmt.Errorf("failed to search jobs: %w", err)
// 		}
// 		p.logger.Debug("job search completed", "num_jobs", len(jobs))
// 	}

// 	return jobs, nil
// }

// processJobDetailsAndSave handles the processing of individual job details and saving to user jobs
func (p *Provider) processJobDetailsAndSave(ctx context.Context, jobID string, searchHash string, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.logger.Debug("processing job details",
		"user_id", userID,
		"job_id", jobID)

	if err := p.processJobDetails(ctx, jobID); err != nil {
		return fmt.Errorf("job %s: %w", jobID, err)
	}

	// Create user job entry
	userJob := &internal.UserJob{
		ID:         jobID,
		SearchHash: searchHash,
		Status:     "new",
		JobRef:     p.fs.Collection("job_details").Doc(jobID),
	}
	if err := internal.SaveUserJob(ctx, p.fs, userID, userJob); err != nil {
		return fmt.Errorf("failed to save user job %s: %w", jobID, err)
	}

	return nil
}

// processQueuedSearch handles the complete job search flow for a queued search request
func (p *Provider) processQueuedSearch(ctx context.Context, qs QueuedSearch) error {
	// Check if context is already canceled before starting work
	if ctx.Err() != nil {
		return ctx.Err()
	}

	p.logger.Debug("processing queued search",
		"user_id", qs.UserID,
		"keywords", qs.Criteria.Keywords,
		"location", qs.Criteria.Location)

	// Process job listings
	jobs, err := p.processJobListings(ctx, qs.Criteria)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return fmt.Errorf("failed to process job listings: %w", err)
	}

	// Generate search hash before processing job details
	searchHash := p.store.GetSearchHash(qs.Criteria.Keywords, qs.Criteria.Location)

	// Process each job's details
	var processingErrors []error
	for _, job := range jobs {
		if err := p.processJobDetailsAndSave(ctx, job.ProviderJobID, searchHash, qs.UserID); err != nil {
			processingErrors = append(processingErrors, err)
			p.logger.Error("failed to process job details",
				"error", err,
				"user_id", qs.UserID,
				"job_id", job.ProviderJobID)
			continue
		}
	}

	// If we had any errors during processing, return them
	if len(processingErrors) > 0 {
		// Update user's search status with error
		if err := p.updateUserSearchError(ctx, qs.UserID, fmt.Sprintf("failed to process %d jobs", len(processingErrors))); err != nil {
			p.logger.Error("failed to update user search error status",
				"error", err,
				"user_id", qs.UserID)
		}
		return fmt.Errorf("failed to process %d jobs: %v", len(processingErrors), processingErrors)
	}

	// Final context check before updating completion
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Update user's search completion status
	p.logger.Debug("updating user search completion status", "user_id", qs.UserID)
	if err := p.updateUserSearchCompletion(ctx, qs.UserID); err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return fmt.Errorf("failed to update user search completion: %w", err)
	}
	p.logger.Debug("search processing completed successfully", "user_id", qs.UserID)

	return nil
}

// processJobDetails gets and stores job details
func (p *Provider) processJobDetails(ctx context.Context, jobID string) error {
	var details *search.JobDetail
	var err error

	// First check Cloudflare KV cache
	details, err = p.store.GetCachedJobDetails(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get cached job details: %w", err)
	}

	// If not in cache, get from ScrapingDog
	if details == nil {
		details, err = p.GetJobDetails(ctx, jobID)
		if err != nil {
			return fmt.Errorf("failed to get job details: %w", err)
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
		p.logger.Debug("initializing job store")
		store, err := NewJobStore(ctx, p.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize job store: %w", err)
		}
		p.store = store
		p.logger.Debug("job store initialized successfully")
	}

	if p.store != nil {
		p.logger.Debug("checking cache for job listings",
			"keywords", criteria.Keywords,
			"location", criteria.Location)
		cachedJobs, err := p.store.GetCachedJobs(ctx, criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to get cached jobs: %w", err)
		}
		if len(cachedJobs) > 0 {
			p.logger.Debug("found cached job listings",
				"num_jobs", len(cachedJobs))
			return cachedJobs, nil
		}
		p.logger.Debug("no cached job listings found")
	}

	p.logger.Debug("performing job search request",
		"keywords", criteria.Keywords,
		"location", criteria.Location)
	jobs, err := p.getJobListings(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs: %w", err)
	}
	p.logger.Debug("job search request completed",
		"num_jobs", len(jobs))

	if p.store != nil {
		p.logger.Debug("saving job listings to cache",
			"num_jobs", len(jobs))
		err := p.store.SaveJobListings(ctx, criteria, jobs)
		if err != nil {
			return nil, fmt.Errorf("failed to save jobs: %w", err)
		}
		p.logger.Debug("job listings saved to cache successfully")
	}
	return jobs, nil
}

// getJobListings searches for jobs based on the provided criteria
func (p *Provider) getJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	p.logger.Debug("ScrapingDog: getting job listings", "criteria", criteria)
	// Acquire semaphore with context
	select {
	case p.searchSemaphore <- struct{}{}:
		defer func() { <-p.searchSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	p.logger.Debug("ScrapingDog: searching jobs with criteria", "criteria", criteria)

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
		geoid, err := internal.GetGeoID(p.fs, criteria.Location)
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
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	p.logger.Debug("ScrapingDog: response received with status", "status_code", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp.StatusCode)
	}

	var listings jobListings
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	p.logger.Debug("ScrapingDog: response decoded successfully", "num_listings", len(listings))

	// Convert to provider jobListings
	jobListings, err := convertJobListings(listings)
	if err != nil {
		return nil, fmt.Errorf("failed to convert job listings: %w", err)
	}

	p.logger.Debug("ScrapingDog: successfully converted job listings", "converted_listings", len(jobListings))
	return jobListings, nil
}

// GetJobDetails retrieves detailed information about a specific job
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	p.logger.Debug("ScrapingDog: getting job details", "job_id", jobID)

	// Build query parameters
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("job_id", jobID)

	// Make request
	url := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp.StatusCode)
	}

	var details []jobDetail
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(details) == 0 {
		return nil, fmt.Errorf("job not found: %s", jobID)
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
		ID:            detail.CompanyLinkedInID,
		Provider:      providerName,
		ProviderJobID: jobID,
		Title:         detail.Position,
		Company:       detail.CompanyName,
		Location:      detail.Location,
		PostedDate:    postedTime,
		URL:           "", // Not available in detail response

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

	// p.logger.Debug("GetJobDetails retrieved job details", "job_id", jobID, "details", jobDetail)
	return jobDetail, nil
}

// updateUserSearchError updates the user's search status with an error
func (p *Provider) updateUserSearchError(ctx context.Context, userID string, errMsg string) error {
	_, err := p.fs.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "search.error", Value: errMsg},
		{Path: "search.completed_at", Value: time.Now()},
		{Path: "search.status", Value: "error"},
	})
	return err
}
