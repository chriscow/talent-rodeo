-- Start by dropping indexes first
-- This is good practice since indexes depend on their parent tables
DROP INDEX IF EXISTS idx_onboarding_feedback_step;

DROP INDEX IF EXISTS idx_onboarding_feedback_user;

DROP INDEX IF EXISTS idx_onboarding_progress_step;

DROP INDEX IF EXISTS idx_onboarding_progress_user;

-- Drop triggers before tables
-- Triggers depend on tables, so we remove them first to avoid dependency issues
DROP TRIGGER IF EXISTS onboarding_progress_updated_at ON onboarding_progress;

DROP TRIGGER IF EXISTS onboarding_feedback_updated_at ON onboarding_feedback;

DROP TRIGGER IF EXISTS candidates_updated_at ON candidates;

DROP TRIGGER IF EXISTS ensure_candidate_role ON candidates;

-- Drop the candidates table
DROP TABLE IF EXISTS candidates;

-- Drop the onboarding_progress table
DROP TABLE IF EXISTS onboarding_progress;

-- Drop the onboarding_feedback table
DROP TABLE IF EXISTS onboarding_feedback;

-- Drop the users table
DROP TABLE IF EXISTS users;

-- Drop the user_role type
DROP TYPE IF EXISTS user_role;