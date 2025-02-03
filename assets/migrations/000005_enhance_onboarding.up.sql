-- Convert onboarding_step to use text type and add tracking columns
ALTER TABLE users ADD COLUMN onboarding_started_at TIMESTAMP;

ALTER TABLE users ADD COLUMN onboarding_completed_at TIMESTAMP;

ALTER TABLE users ADD COLUMN onboarding_step_updated_at TIMESTAMP;

ALTER TABLE users
ADD COLUMN onboarding_step TEXT NOT NULL DEFAULT 'initial';

-- Create an index for quick lookups of users in specific onboarding states
CREATE INDEX idx_users_onboarding_step ON users (onboarding_step);

-- Create an index for finding incomplete onboarding
CREATE INDEX idx_users_incomplete_onboarding ON users (onboarding_started_at)
WHERE
    onboarding_completed_at IS NULL;