-- Start by dropping indexes first
-- This is good practice since indexes depend on their parent tables
DROP INDEX IF EXISTS idx_job_listings_posting_date;

DROP INDEX IF EXISTS idx_job_listings_location;

DROP INDEX IF EXISTS idx_job_listings_company_name;

-- Drop triggers before tables
-- Triggers depend on tables, so we remove them first to avoid dependency issues
DROP TRIGGER IF EXISTS update_job_listing_timestamp;

-- Drop the junction table first
-- We start with search_results since it has foreign key constraints
-- pointing to both job_listings and job_searches
DROP TABLE IF EXISTS search_results;

-- Now we can safely drop the main tables
-- Since job_listings doesn't depend on job_searches (and vice versa),
-- the order between these two doesn't matter
DROP TABLE IF EXISTS job_listings;

DROP TABLE IF EXISTS job_searches;

-- Drop the indexes from job_searches table that we created earlier
DROP INDEX IF EXISTS idx_job_searches_hash;

DROP INDEX IF EXISTS idx_job_searches_last_searched;

DROP INDEX IF EXISTS idx_job_searches_hash;

DROP INDEX IF EXISTS idx_job_searches_last_searched;

DROP TABLE IF EXISTS job_searches;

-- Drop indexes
DROP INDEX IF EXISTS idx_people_also_viewed_company;

DROP INDEX IF EXISTS idx_similar_jobs_company;

DROP INDEX IF EXISTS idx_jobs_location;

DROP INDEX IF EXISTS idx_jobs_position;

DROP INDEX IF EXISTS idx_jobs_company_name;

-- Drop triggers
DROP TRIGGER IF EXISTS jobs_updated_at;

DROP TABLE IF EXISTS extracted_job_details;

-- Drop tables
DROP TABLE IF EXISTS people_also_viewed;

DROP TABLE IF EXISTS similar_jobs;

DROP TABLE IF EXISTS recruiters;

DROP TABLE IF EXISTS jobs;

-- Disable foreign key support
PRAGMA foreign_keys = OFF;