package linkedin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

type SearchManager struct {
	db *sql.DB
}

func (sm *SearchManager) SearchJobs(ctx context.Context, keyword, geoID string, opts ...JobSearchOption) ([]JobListing, error) {
	options := JobSearchOptions{pageLimit: 1}
	for _, opt := range opts {
		opt(&options)
	}

	// Generate unique hash for this search configuration
	searchHash := generateSearchHash(keyword, geoID, options)

	// Check last search time
	var lastSearched time.Time
	err := sm.db.QueryRowContext(ctx,
		`SELECT last_searched_at FROM job_searches WHERE search_hash = ?`,
		searchHash).Scan(&lastSearched)

	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check search history: %w", err)
	}

	// Adjust search strategy based on last search time
	if err == sql.ErrNoRows || time.Since(lastSearched) > 72*time.Hour {
		// New search or old search: use requested options
		options.sortBy = "month" // Start with full month
	} else {
		// Recent search: only get recent updates
		options.sortBy = "day"
	}

	jobListings := []JobListing{}
	for page := 1; page <= options.pageLimit; page++ {
		pageStr := strconv.Itoa(page)
		results, err := searchJobs(ctx, geoID, keyword, pageStr, options)
		if err != nil {
			return nil, err
		}

		jobListings = append(jobListings, results...)
	}

	// Update search history
	_, err = sm.db.ExecContext(ctx,
		`INSERT INTO job_searches (keyword, geo_id, sort_by, job_type, 
         exp_level, work_type, company_filter, last_searched_at, 
         total_results, search_hash)
         VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)
         ON CONFLICT(search_hash) 
         DO UPDATE SET last_searched_at=CURRENT_TIMESTAMP, 
         total_results=?`,
		keyword, geoID, options.sortBy, options.jobType, options.expLevel,
		options.workType, options.filterByCompany, len(jobListings),
		searchHash, len(jobListings))

	return jobListings, err
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
