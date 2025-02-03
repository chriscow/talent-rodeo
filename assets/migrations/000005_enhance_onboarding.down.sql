DROP INDEX IF EXISTS idx_users_incomplete_onboarding;

DROP INDEX IF EXISTS idx_users_onboarding_step;

ALTER TABLE users DROP COLUMN onboarding_step;

ALTER TABLE users DROP COLUMN onboarding_step_updated_at;

ALTER TABLE users DROP COLUMN onboarding_completed_at;

ALTER TABLE users DROP COLUMN onboarding_started_at;