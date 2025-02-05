-- Drop triggers first
DROP TRIGGER IF EXISTS onboarding_progress_updated_at ON onboarding_progress;

DROP TRIGGER IF EXISTS onboarding_feedback_updated_at ON onboarding_feedback;

-- Drop indexes
DROP INDEX IF EXISTS idx_users_incomplete_onboarding;

DROP INDEX IF EXISTS idx_users_onboarding_step;

DROP INDEX IF EXISTS idx_onboarding_progress_user;

DROP INDEX IF EXISTS idx_onboarding_progress_step;

DROP INDEX IF EXISTS idx_onboarding_feedback_user;

DROP INDEX IF EXISTS idx_onboarding_feedback_step;

-- Drop tables with foreign key constraints first
DROP TABLE IF EXISTS onboarding_feedback;

DROP TABLE IF EXISTS onboarding_progress;

-- Now modify the users table
ALTER TABLE users DROP COLUMN onboarding_step;

ALTER TABLE users DROP COLUMN onboarding_step_updated_at;

ALTER TABLE users DROP COLUMN onboarding_completed_at;

ALTER TABLE users DROP COLUMN onboarding_started_at;