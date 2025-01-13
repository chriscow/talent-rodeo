-- Drop indexes
DROP INDEX IF EXISTS idx_people_also_viewed_company;

DROP INDEX IF EXISTS idx_similar_jobs_company;

DROP INDEX IF EXISTS idx_jobs_location;

DROP INDEX IF EXISTS idx_jobs_position;

DROP INDEX IF EXISTS idx_jobs_company_name;

-- Drop triggers
DROP TRIGGER IF EXISTS jobs_updated_at;

-- Drop tables
DROP TABLE IF EXISTS people_also_viewed;

DROP TABLE IF EXISTS similar_jobs;

DROP TABLE IF EXISTS recruiters;

DROP TABLE IF EXISTS jobs;

-- Disable foreign key support
PRAGMA foreign_keys = OFF;