-- Drop triggers first
DROP TRIGGER IF EXISTS job_search_criteria_updated_at ON job_search_criteria;

DROP TRIGGER IF EXISTS scheduling_preferences_updated_at ON scheduling_preferences;

DROP TRIGGER IF EXISTS appointments_updated_at ON appointments;

-- Drop indexes
DROP INDEX IF EXISTS idx_job_search_criteria_user;

DROP INDEX IF EXISTS idx_appointments_status;

DROP INDEX IF EXISTS idx_appointments_user_time;

-- Drop tables in reverse order
DROP TABLE IF EXISTS job_search_criteria;

DROP TABLE IF EXISTS scheduling_preferences;

DROP TABLE IF EXISTS appointments;