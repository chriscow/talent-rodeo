-- Drop provider indexes
DROP INDEX IF EXISTS idx_provider_jobs_provider;

DROP INDEX IF EXISTS idx_provider_job_listings_provider;

-- Remove provider columns
ALTER TABLE provider_jobs DROP COLUMN provider;

ALTER TABLE provider_job_listings DROP COLUMN provider;

-- Revert foreign key references
ALTER TABLE job_applications RENAME COLUMN provider_job_id TO job_id;

ALTER TABLE job_skills RENAME COLUMN provider_job_id TO job_id;

ALTER TABLE recruiters RENAME COLUMN provider_job_id TO job_id;

ALTER TABLE similar_jobs
RENAME COLUMN original_provider_job_id TO original_job_id;

ALTER TABLE people_also_viewed
RENAME COLUMN original_provider_job_id TO original_job_id;

ALTER TABLE extracted_job_details
RENAME COLUMN provider_job_id TO job_id;

-- Revert table names
ALTER TABLE provider_jobs RENAME TO jobs;

ALTER TABLE provider_job_listings RENAME TO job_listings;

-- Revert sequence names
ALTER SEQUENCE provider_jobs_id_seq RENAME TO jobs_id_seq;

ALTER SEQUENCE provider_job_listings_id_seq
RENAME TO job_listings_id_seq;