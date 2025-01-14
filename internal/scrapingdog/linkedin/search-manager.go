package linkedin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
)

type SearchManager struct {
	db       *sqlx.DB
	jobStore *JobStore
	logger   *slog.Logger
}

func NewSearchManager(db *sqlx.DB, logger *slog.Logger) *SearchManager {
	return &SearchManager{db: db, jobStore: &JobStore{db: db}, logger: logger}
}

func (sm *SearchManager) SearchJobs(ctx context.Context, keyword, geoID string, opts ...JobSearchOption) ([]JobListing, error) {
	options := JobSearchOptions{pageLimit: 1}
	for _, opt := range opts {
		opt(&options)
	}

	searchHash := generateSearchHash(keyword, geoID, options)

	// Modified query to also return whether this was a new insert
	const query = `
INSERT INTO job_searches (
    keyword, geo_id, sort_by, job_type, exp_level, 
    work_type, company_filter, last_searched_at, search_hash,
    total_results
) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, 0)
ON CONFLICT(search_hash) DO UPDATE SET 
    last_searched_at = CASE 
        WHEN datetime(last_searched_at, '+4 hours') < datetime('now')
        THEN datetime('now')
        ELSE last_searched_at 
    END
RETURNING 
    id, 
    last_searched_at,
    CAST((julianday('now') - julianday(last_searched_at)) * 3600 AS INTEGER) as age_seconds,
    (SELECT COUNT(*) FROM search_results WHERE search_id = job_searches.id) > 0 as has_cached_results
    `

	var (
		searchID         int64
		lastSearched     time.Time
		ageSeconds       int64
		hasCachedResults bool
	)

	err := sm.db.QueryRowContext(ctx, query,
		keyword, geoID, options.sortBy, options.jobType,
		options.expLevel, options.workType,
		options.filterByCompany, searchHash).Scan(&searchID, &lastSearched, &ageSeconds, &hasCachedResults)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert search record: %w", err)
	}

	age := time.Duration(ageSeconds) * time.Second

	// Return cached results only if they exist and are fresh
	if hasCachedResults && age < 4*time.Hour {
		jobListings, err := sm.jobStore.GetJobsByID(searchID)
		if err != nil {
			return nil, fmt.Errorf("failed to get cached job listings: %w", err)
		}

		sm.logger.Info("Returning cached results", "search_id", searchID, "count", len(jobListings))
		return jobListings, nil
	}

	// For new searches or stale cached results, determine search scope
	var searchScope string
	if !hasCachedResults {
		// New search - get everything from the past month
		searchScope = "month"
	} else {
		// Existing but stale search - use age-based strategy
		switch {
		case age > 72*time.Hour:
			searchScope = "month"
		case age > 48*time.Hour:
			searchScope = "week"
		default:
			searchScope = "day"
		}
	}
	sm.logger.Debug("Determining search scope", "search_id", searchID, "age (seconds)", ageSeconds, "scope", searchScope)
	options.sortBy = searchScope

	// Perform the search with determined scope
	jobListings := make([]JobListing, 0, options.pageLimit*50)
	for page := 1; page <= options.pageLimit; page++ {
		pageStr := fmt.Sprintf("%d", page)
		results, err := searchJobs(ctx, geoID, keyword, pageStr, options)
		if err != nil {
			return nil, fmt.Errorf("failed to search jobs on page %d: %w", page, err)
		}
		jobListings = append(jobListings, results...)
	}

	sm.logger.Debug("Search results", "search_id", searchID, "count", len(jobListings))

	// Save the results and update total count
	if _, err := sm.db.ExecContext(ctx, `
        UPDATE job_searches 
        SET total_results = ? 
        WHERE id = ?
    `, len(jobListings), searchID); err != nil {
		log.Printf("failed to update total results count: %v", err)
	}

	if err := sm.jobStore.SaveJobListings(ctx, searchID, jobListings); err != nil {
		log.Printf("failed to save job listings for search %d: %v", searchID, err)
	}

	return jobListings, nil
}

func (sm *SearchManager) CleanupOldSearches(ctx context.Context) error {
	_, err := sm.db.ExecContext(ctx, `
        DELETE FROM job_searches 
        WHERE last_searched_at < CURRENT_TIMESTAMP - INTERVAL '7 days'
    `)
	return err
}

func (sm *SearchManager) GetJobDetail(ctx context.Context, jobID string) (JobDetail, error) {
	// First try to get the job from our database
	var job JobDetail
	err := sm.db.QueryRowContext(ctx, `
        SELECT 
            position, status, location, company_name,
            company_linkedin_id, posting_time, base_pay, description,
            seniority_level, employment_type, function, industries
        FROM jobs 
        WHERE id = ?`, jobID).Scan(
		&job.Position, &job.Status, &job.Location, &job.CompanyName,
		&job.CompanyLinkedInID, &job.PostingTime, &job.BasePay, &job.Description,
		&job.SeniorityLevel, &job.EmploymentType, &job.JobFunction, &job.Industries)

	if err == nil {
		// If we found the job, get its related data
		job.Recruiters, err = sm.getRecruiters(ctx, jobID)
		if err != nil {
			return JobDetail{}, fmt.Errorf("getting recruiters: %w", err)
		}

		job.SimilarJobs, err = sm.getSimilarJobs(ctx, jobID)
		if err != nil {
			return JobDetail{}, fmt.Errorf("getting similar jobs: %w", err)
		}

		job.PeopleAlsoViewed, err = sm.getPeopleAlsoViewed(ctx, jobID)
		if err != nil {
			return JobDetail{}, fmt.Errorf("getting people also viewed: %w", err)
		}

		return job, nil
	}

	if err != sql.ErrNoRows {
		return JobDetail{}, fmt.Errorf("querying database: %w", err)
	}

	// If we didn't find it, fetch from API
	job, err = sm.fetchJobDetailFromAPI(ctx, jobID)
	if err != nil {
		return JobDetail{}, err
	}

	// Save the new job detail to our database
	err = sm.saveJobDetail(ctx, jobID, job)
	if err != nil {
		// Log the error but return the job detail anyway
		log.Printf("error saving job detail: %v", err)
	}

	return job, nil
}

// Helper functions to keep the main function clean
func (sm *SearchManager) getRecruiters(ctx context.Context, jobID string) ([]Recruiter, error) {
	rows, err := sm.db.QueryContext(ctx, `
        SELECT photo_url, name, title, profile_url 
        FROM recruiters 
        WHERE job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recruiters []Recruiter
	for rows.Next() {
		var r Recruiter
		if err := rows.Scan(&r.Photo, &r.Name, &r.Title, &r.ProfileURL); err != nil {
			return nil, err
		}
		recruiters = append(recruiters, r)
	}
	return recruiters, nil
}

func (sm *SearchManager) getSimilarJobs(ctx context.Context, jobID string) ([]RelatedJob, error) {
	rows, err := sm.db.QueryContext(ctx, `
        SELECT position, company, location, posting_time, link 
        FROM similar_jobs 
        WHERE original_job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []RelatedJob
	for rows.Next() {
		var j RelatedJob
		if err := rows.Scan(&j.Position, &j.Company, &j.Location, &j.PostingTime, &j.Link); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (sm *SearchManager) getPeopleAlsoViewed(ctx context.Context, jobID string) ([]RelatedJob, error) {
	rows, err := sm.db.QueryContext(ctx, `
		SELECT position, company, location, posting_time, link 
		FROM people_also_viewed 
		WHERE original_job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []RelatedJob
	for rows.Next() {
		var j RelatedJob
		if err := rows.Scan(&j.Position, &j.Company, &j.Location, &j.PostingTime, &j.Link); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (sm *SearchManager) saveJobDetail(ctx context.Context, jobID string, job JobDetail) error {
	tx, err := sm.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert main job details
	_, err = tx.ExecContext(ctx, `
        INSERT INTO jobs (
            id, position, status, location, company_name,
            company_linkedin_id, posting_time, base_pay, description,
            seniority_level, employment_type, function, industries
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		jobID, job.Position, job.Status, job.Location, job.CompanyName,
		job.CompanyLinkedInID, job.PostingTime, job.BasePay, job.Description,
		job.SeniorityLevel, job.EmploymentType, job.JobFunction, job.Industries)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	// Insert recruiters, similar jobs, and people also viewed...
	// (similar pattern for each)

	for _, r := range job.Recruiters {
		if r.Name == "" {
			continue
		}
		_, err = tx.ExecContext(ctx, `
		INSERT INTO recruiters (job_id, photo_url, name, title, profile_url)
		VALUES (?, ?, ?, ?, ?)`,
			jobID, r.Photo, r.Name, r.Title, r.ProfileURL)
		if err != nil {
			return fmt.Errorf("insert recruiter: %w", err)
		}
	}

	for _, j := range job.SimilarJobs {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO similar_jobs (original_job_id, position, company, location, posting_time, link)
		VALUES (?, ?, ?, ?, ?, ?)`,
			jobID, j.Position, j.Company, j.Location, j.PostingTime, j.Link)
		if err != nil {
			return fmt.Errorf("insert similar job: %w", err)
		}
	}

	for _, j := range job.PeopleAlsoViewed {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO people_also_viewed (original_job_id, position, company, location, posting_time, link)
		VALUES (?, ?, ?, ?, ?, ?)`,
			jobID, j.Position, j.Company, j.Location, j.PostingTime, j.Link)
		if err != nil {
			return fmt.Errorf("insert people also viewed: %w", err)
		}
	}

	return tx.Commit()
}

// Moved the API call to its own function
func (sm *SearchManager) fetchJobDetailFromAPI(ctx context.Context, jobID string) (JobDetail, error) {
	req, err := http.NewRequest("GET", "https://api.scrapingdog.com/linkedinjobs/", nil)
	if err != nil {
		return JobDetail{}, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("api_key", os.Getenv("SCRAPINGDOG_API_KEY"))
	q.Add("job_id", jobID)
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return JobDetail{}, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return JobDetail{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	jobDetail := []JobDetail{}
	if err := json.NewDecoder(resp.Body).Decode(&jobDetail); err != nil {
		return JobDetail{}, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(jobDetail) == 0 {
		return JobDetail{}, fmt.Errorf("job not found")
	}

	if len(jobDetail) > 1 {
		println("WARN: multiple job details found for ID", jobID)
	}

	return jobDetail[0], nil
}

func generateSearchHash(keyword, geoID string, opts JobSearchOptions) string {
	// Create deterministic string representation of search parameters
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s", keyword, geoID)
	if opts.jobType != "" {
		fmt.Fprintf(h, ":%s", opts.jobType)
	}
	if opts.expLevel != "" {
		fmt.Fprintf(h, ":%s", opts.expLevel)
	}
	if opts.filterByCompany != "" {
		fmt.Fprintf(h, ":%s", opts.filterByCompany)
	}
	// Add other options...
	return hex.EncodeToString(h.Sum(nil))
}
