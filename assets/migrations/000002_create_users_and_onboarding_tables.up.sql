CREATE TYPE user_role AS ENUM( 'admin', 'candidate', 'recruiter' );

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    email TEXT NOT NULL UNIQUE,
    hashed_secret TEXT NOT NULL,
    provider TEXT NOT NULL,
    name TEXT,
    first_name TEXT,
    last_name TEXT,
    nick_name TEXT,
    description TEXT,
    user_id TEXT,
    avatar_url TEXT,
    location TEXT,
    is_superuser BOOLEAN NOT NULL DEFAULT false,
    -- 
    resume_filename TEXT,
    full_resume TEXT,
    summarized_resume TEXT,
    --
    phone TEXT,
    --
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    onboarding_started_at TIMESTAMP, -- Added onboarding column
    onboarding_completed_at TIMESTAMP, -- Added onboarding column
    onboarding_step_updated_at TIMESTAMP, -- Added onboarding column
    onboarding_step TEXT NOT NULL DEFAULT 'initial', -- Added onboarding column
    --
    user_role user_role NOT NULL DEFAULT 'candidate'
);

-- Create function for updating timestamps if it doesn't exist
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TABLE IF NOT EXISTS candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    culture_preferences TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_user_id UNIQUE (user_id)
);

CREATE INDEX idx_candidates_user_id ON candidates (user_id);

-- Function to validate candidate role
CREATE OR REPLACE FUNCTION validate_candidate_role()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM users u
        WHERE u.id = NEW.user_id AND u.role = 'candidate'
    ) THEN
        RAISE EXCEPTION 'User must have role "candidate" to be added to candidates table';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to validate role before insert or update
CREATE TRIGGER ensure_candidate_role BEFORE INSERT
OR
UPDATE ON candidates FOR EACH ROW
EXECUTE FUNCTION validate_candidate_role ();

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_candidates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER candidates_updated_at BEFORE
UPDATE ON candidates FOR EACH ROW
EXECUTE FUNCTION update_candidates_updated_at ();

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