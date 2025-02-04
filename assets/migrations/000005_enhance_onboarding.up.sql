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

-- Create onboarding_progress table
CREATE TABLE onboarding_progress (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id),
    step TEXT NOT NULL,
    completed BOOLEAN NOT NULL DEFAULT false,
    data JSONB, -- PostgreSQL's binary JSON type for better performance
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, step)
);

-- Create onboarding_feedback table
CREATE TABLE onboarding_feedback (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id),
    step TEXT NOT NULL,
    rating INTEGER CHECK (
        rating >= 1
        AND rating <= 5
    ),
    feedback TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_onboarding_progress_user ON onboarding_progress (user_id);

CREATE INDEX idx_onboarding_progress_step ON onboarding_progress (step);

CREATE INDEX idx_onboarding_feedback_user ON onboarding_feedback (user_id);

CREATE INDEX idx_onboarding_feedback_step ON onboarding_feedback (step);

-- Create triggers for updating timestamps
CREATE TRIGGER onboarding_progress_updated_at BEFORE
UPDATE ON onboarding_progress FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER onboarding_feedback_updated_at BEFORE
UPDATE ON onboarding_feedback FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();