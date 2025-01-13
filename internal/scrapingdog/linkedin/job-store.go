package linkedin

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type JobStore struct {
	db *sqlx.DB
}

func (js *JobStore) SaveJobListings(ctx context.Context, searchID int64, listings []JobListing) error {
	tx, err := js.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, listing := range listings {
		// First, insert or update the job listing
		var jobListingID int64
		err := tx.QueryRowContext(ctx, `
            INSERT INTO job_listings (
                job_id, position, job_link, company_name, 
                company_profile, location, posting_date, company_logo
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (job_id) DO UPDATE SET
                position = excluded.position,
                job_link = excluded.job_link,
                company_name = excluded.company_name,
                company_profile = excluded.company_profile,
                location = excluded.location,
                posting_date = excluded.posting_date,
                company_logo = excluded.company_logo
            RETURNING id`,
			listing.ID, listing.Position, listing.Link, listing.CompanyName,
			listing.CompanyProfile, listing.Location, listing.PostingDate,
			listing.CompanyLogo,
		).Scan(&jobListingID)
		if err != nil {
			return fmt.Errorf("upsert job listing: %w", err)
		}

		// Then, record this job as a result of the search
		_, err = tx.ExecContext(ctx, `
            INSERT INTO search_results (search_id, job_listing_id)
            VALUES (?, ?)
            ON CONFLICT DO NOTHING`,
			searchID, jobListingID)
		if err != nil {
			return fmt.Errorf("insert search result: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (js *JobStore) GetJobsByID(searchID int64) ([]JobListing, error) {
	rows, err := js.db.Query(`
		SELECT 
			j.job_id, j.position, j.link, j.company_name,
			j.company_profile, j.location, j.posting_date, j.company_logo
		FROM job_listings j
		JOIN search_results sr ON j.id = sr.job_listing_id
		WHERE sr.search_id = ?`,
		searchID)
	if err != nil {
		return nil, fmt.Errorf("query jobs by search ID: %w", err)
	}
	defer rows.Close()

	var listings []JobListing
	for rows.Next() {
		var listing JobListing
		if err := rows.Scan(
			&listing.ID, &listing.Position, &listing.Link, &listing.CompanyName,
			&listing.CompanyProfile, &listing.Location, &listing.PostingDate,
			&listing.CompanyLogo,
		); err != nil {
			return nil, fmt.Errorf("scan job listing: %w", err)
		}
		listings = append(listings, listing)
	}
	return listings, nil
}

// GetRecentJobs retrieves jobs from recent searches
func (js *JobStore) GetRecentJobs(ctx context.Context, limit int) ([]JobListing, error) {
	rows, err := js.db.QueryContext(ctx, `
        SELECT DISTINCT 
            j.job_id, j.position, j.job_link, j.company_name,
            j.company_profile, j.location, j.posting_date, j.company_logo
        FROM job_listings j
        ORDER BY j.first_seen_at DESC
        LIMIT ?`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("query recent jobs: %w", err)
	}
	defer rows.Close()

	var listings []JobListing
	for rows.Next() {
		var listing JobListing
		if err := rows.Scan(
			&listing.ID, &listing.Position, &listing.Link, &listing.CompanyName,
			&listing.CompanyProfile, &listing.Location, &listing.PostingDate,
			&listing.CompanyLogo,
		); err != nil {
			return nil, fmt.Errorf("scan job listing: %w", err)
		}
		listings = append(listings, listing)
	}
	return listings, nil
}
