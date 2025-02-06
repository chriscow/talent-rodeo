-- Rename tables to be provider-agnostic
ALTER TABLE jobs RENAME TO provider_jobs;

ALTER TABLE job_listings RENAME TO provider_job_listings;

-- Update foreign key references
ALTER TABLE job_applications RENAME COLUMN job_id TO provider_job_id;

ALTER TABLE job_skills RENAME COLUMN job_id TO provider_job_id;

ALTER TABLE recruiters RENAME COLUMN job_id TO provider_job_id;

ALTER TABLE similar_jobs
RENAME COLUMN original_job_id TO original_provider_job_id;

ALTER TABLE people_also_viewed
RENAME COLUMN original_job_id TO original_provider_job_id;

ALTER TABLE extracted_job_details
RENAME COLUMN job_id TO provider_job_id;

-- Add provider column to track job source
ALTER TABLE provider_jobs
ADD COLUMN provider TEXT NOT NULL DEFAULT 'linkedin';

ALTER TABLE provider_job_listings
ADD COLUMN provider TEXT NOT NULL DEFAULT 'linkedin';

-- Add indexes for provider columns
CREATE INDEX idx_provider_jobs_provider ON provider_jobs (provider);

CREATE INDEX idx_provider_job_listings_provider ON provider_job_listings (provider);

-- Update sequence names
ALTER SEQUENCE jobs_id_seq RENAME TO provider_jobs_id_seq;

ALTER SEQUENCE job_listings_id_seq
RENAME TO provider_job_listings_id_seq;