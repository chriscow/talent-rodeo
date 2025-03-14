-- Start by dropping indexes first
-- This is good practice since indexes depend on their parent tables
DROP INDEX IF EXISTS idx_job_listings_posting_date;
DROP INDEX IF EXISTS idx_job_listings_location;
DROP INDEX IF EXISTS idx_job_listings_company_name;
DROP INDEX IF EXISTS idx_candidate_search_results_candidate;

-- Drop triggers before tables
-- Triggers depend on tables, so we remove them first to avoid dependency issues
DROP TRIGGER IF EXISTS jobs_updated_at ON jobs;
DROP TRIGGER IF EXISTS companies_updated_at ON companies;
DROP TRIGGER IF EXISTS job_applications_updated_at ON job_applications;
DROP TRIGGER IF EXISTS job_skills_updated_at ON job_skills;
DROP TRIGGER IF EXISTS job_listings_updated_at ON job_listings;
DROP TRIGGER IF EXISTS extracted_job_details_updated_at ON extracted_job_details;
DROP TRIGGER IF EXISTS user_keywords_updated_at ON user_keywords;

-- Drop custom functions
DROP FUNCTION IF EXISTS update_job_listings_timestamp();

-- Drop tables with foreign key dependencies first
DROP TABLE IF EXISTS candidate_search_results;
DROP TABLE IF EXISTS search_results;
DROP TABLE IF EXISTS job_listings;
DROP TABLE IF EXISTS job_searches;

-- Drop indexes
DROP INDEX IF EXISTS idx_people_also_viewed_company;
DROP INDEX IF EXISTS idx_similar_jobs_company;
DROP INDEX IF EXISTS idx_jobs_location;
DROP INDEX IF EXISTS idx_jobs_work_type;
DROP INDEX IF EXISTS idx_jobs_employment_type;
DROP INDEX IF EXISTS idx_jobs_seniority;
DROP INDEX IF EXISTS idx_jobs_function;
DROP INDEX IF EXISTS idx_jobs_linkedin_id;
DROP INDEX IF EXISTS idx_jobs_experience;
DROP INDEX IF EXISTS idx_jobs_remote;
DROP INDEX IF EXISTS idx_jobs_title;
DROP INDEX IF EXISTS idx_applications_user;
DROP INDEX IF EXISTS idx_applications_job;
DROP INDEX IF EXISTS idx_applications_status;
DROP INDEX IF EXISTS idx_skills_job;
DROP INDEX IF EXISTS idx_skills_skill;
DROP INDEX IF EXISTS idx_job_searches_hash;
DROP INDEX IF EXISTS idx_job_searches_last_searched;

-- Drop remaining tables in correct order (respecting foreign key constraints)
DROP TABLE IF EXISTS extracted_job_details;
DROP TABLE IF EXISTS people_also_viewed;
DROP TABLE IF EXISTS similar_jobs;
DROP TABLE IF EXISTS recruiters;
DROP TABLE IF EXISTS job_skills;
DROP TABLE IF EXISTS job_applications;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS companies;
DROP TABLE IF EXISTS user_keywords;