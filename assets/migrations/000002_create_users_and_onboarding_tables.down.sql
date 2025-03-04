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

-- Drop tables in reverse order of creation
DROP TABLE IF EXISTS onboarding_feedback;

DROP TABLE IF EXISTS onboarding_progress;

DROP TABLE IF EXISTS candidate_preferences;

DROP TABLE IF EXISTS candidate_skills;

DROP TABLE IF EXISTS candidates;

DROP TABLE IF EXISTS users;

-- Drop functions
DROP FUNCTION IF EXISTS update_timestamp;

DROP FUNCTION IF EXISTS validate_candidate_role;

DROP FUNCTION IF EXISTS update_candidates_updated_at;

-- Drop types
DROP TYPE IF EXISTS role;