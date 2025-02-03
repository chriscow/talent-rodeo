DROP TRIGGER IF EXISTS background_jobs_updated_at;

DROP INDEX IF EXISTS idx_background_jobs_status_run_at;

DROP TABLE IF EXISTS background_jobs;