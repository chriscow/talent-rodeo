package scrapingdog

import (
	"context"
	"fmt"
	"log"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/services/search"
	"time"

	"github.com/lib/pq"
)

// JobStore is responsible for managing job listings and caching them.
type JobStore struct {
	db *database.DB // Database connection
}

// NewJobStore creates a new JobStore instance.
func NewJobStore(db *database.DB) *JobStore {
	store := &JobStore{db: db}
	if err := store.validateSchema(); err != nil {
		log.Printf("WARNING: Schema validation failed: %v", err)
	}
	return store
}

// validateSchema checks if the database schema matches our expectations
func (js *JobStore) validateSchema() error {
	// Check job_listings table schema
	rows, err := js.db.QueryContext(context.Background(), `
		SELECT column_name, data_type 
		FROM information_schema.columns 
		WHERE table_name = 'job_listings'
		ORDER BY ordinal_position`)
	if err != nil {
		return fmt.Errorf("failed to get schema: %w", err)
	}
	defer rows.Close()

	log.Printf("job_listings table schema:")
	for rows.Next() {
		var columnName, dataType string
		if err := rows.Scan(&columnName, &dataType); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		log.Printf("  %s: %s", columnName, dataType)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error during schema rows iteration: %w", err)
	}

	// Check triggers
	triggerRows, err := js.db.QueryContext(context.Background(), `
		SELECT 
			t.tgname as trigger_name,
			pg_get_triggerdef(t.oid) as trigger_definition
		FROM pg_trigger t
		JOIN pg_class c ON t.tgrelid = c.oid
		WHERE c.relname = 'job_listings'`)
	if err != nil {
		return fmt.Errorf("failed to get triggers: %w", err)
	}
	defer triggerRows.Close()

	log.Printf("job_listings table triggers:")
	for triggerRows.Next() {
		var triggerName, definition string
		if err := triggerRows.Scan(&triggerName, &definition); err != nil {
			return fmt.Errorf("failed to scan trigger info: %w", err)
		}
		log.Printf("  %s: %s", triggerName, definition)
	}
	if err := triggerRows.Err(); err != nil {
		return fmt.Errorf("error during trigger rows iteration: %w", err)
	}

	return nil
}

// GetCachedJobs retrieves cached job listings based on the search criteria.
func (js *JobStore) GetCachedJobs(criteria search.Criteria) ([]search.JobListing, error) {
	log.Printf("Getting cached jobs for criteria: %+v", criteria)

	query := `
	SELECT DISTINCT
		jl.job_id,
		jl.position,
		jl.company_name,
		jl.location,
		jl.posting_date,
		jl.link,
		jl.company_profile
	FROM job_listings jl
	JOIN search_results sr ON jl.id = sr.job_listing_id
	JOIN job_searches js ON sr.search_id = js.id
	WHERE js.keyword = ANY($1)
		AND jl.location = $2 
		AND sr.found_at >= NOW() - INTERVAL '4 hours'
	ORDER BY jl.job_id`

	rows, err := js.db.QueryContext(context.Background(), query, pq.Array(criteria.Keywords), criteria.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to query cached jobs: %w", err)
	}
	defer rows.Close()

	jobMap := make(map[string]search.JobListing)
	for rows.Next() {
		var job search.JobListing
		var postedDateStr string
		if err := rows.Scan(
			&job.ProviderJobID,
			&job.Title,
			&job.Company,
			&job.Location,
			&postedDateStr,
			&job.URL,
			&job.CompanyProfile,
		); err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		log.Printf("Scanned job from DB: %+v with postedDate: %s", job, postedDateStr)

		// Parse the posting date - handle both formats
		postedDate, err := time.Parse("2006-01-02", postedDateStr)
		if err != nil {
			// Try alternate format
			postedDate, err = time.Parse("2006-01-02 15:04:05", postedDateStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse posting date: %w", err)
			}
		}
		job.PostedDate = postedDate
		job.Provider = "scrapingdog"
		job.ID = job.ProviderJobID // Use the provider job ID as the main ID

		// Store in map to ensure we get one entry per job per keyword
		jobMap[job.ID] = job
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error occurred during rows iteration: %w", err)
	}

	// Convert map to slice
	var allJobs []search.JobListing
	for _, job := range jobMap {
		allJobs = append(allJobs, job)
	}

	log.Printf("Found %d cached jobs", len(allJobs))
	return allJobs, nil
}

// SaveJobListings saves job listings to the cache.
func (js *JobStore) SaveJobListings(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	log.Printf("Saving %d jobs for criteria: %+v", len(jobs), criteria)

	return js.db.WithTx(ctx, func(tx *database.DB) error {
		// For each keyword, create a search record and link it to all jobs
		for _, keyword := range criteria.Keywords {
			log.Printf("Processing keyword: %s", keyword)

			// Create or update the job search record
			var searchID int64
			searchHash := fmt.Sprintf("%s_%s", keyword, criteria.Location)
			err := tx.QueryRowContext(ctx, `
				INSERT INTO job_searches (keyword, geo_id, search_hash, last_searched_at)
				VALUES ($1, $2, $3, NOW())
				ON CONFLICT (search_hash) DO UPDATE SET
					last_searched_at = NOW()
				RETURNING id`,
				keyword, criteria.Location, searchHash).Scan(&searchID)
			if err != nil {
				return fmt.Errorf("failed to insert/update job search: %w", err)
			}

			log.Printf("Created/updated search record with ID: %d", searchID)

			// For each job, insert/update the listing and create search result
			for _, job := range jobs {
				log.Printf("Processing job: %+v", job)

				// Format the date as a string
				postedDateStr := job.PostedDate.Format("2006-01-02")

				// Insert or update the job listing
				var jobListingID int64
				insertQuery := `
					INSERT INTO job_listings (
						job_id, position, link, company_name,
						location, posting_date, company_profile,
						first_seen_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
					ON CONFLICT (job_id) DO UPDATE SET
						position = EXCLUDED.position,
						link = EXCLUDED.link,
						company_name = EXCLUDED.company_name,
						location = EXCLUDED.location,
						posting_date = EXCLUDED.posting_date,
						company_profile = EXCLUDED.company_profile
					RETURNING id`

				log.Printf("Executing insert query with values: %s, %s, %s, %s, %s, %s, %s",
					job.ProviderJobID, job.Title, job.URL, job.Company,
					job.Location, postedDateStr, job.CompanyProfile)

				err := tx.QueryRowContext(ctx, insertQuery,
					job.ProviderJobID, job.Title, job.URL, job.Company,
					job.Location, postedDateStr, job.CompanyProfile).Scan(&jobListingID)
				if err != nil {
					return fmt.Errorf("failed to insert/update job listing: %w", err)
				}

				log.Printf("Created/updated job listing with ID: %d", jobListingID)

				// Create search result
				_, err = tx.ExecContext(ctx, `
					INSERT INTO search_results (search_id, job_listing_id)
					VALUES ($1, $2)
					ON CONFLICT DO NOTHING`,
					searchID, jobListingID)
				if err != nil {
					return fmt.Errorf("failed to insert search result: %w", err)
				}

				log.Printf("Created search result linking search %d to job %d", searchID, jobListingID)
			}
		}

		return nil
	})
}
