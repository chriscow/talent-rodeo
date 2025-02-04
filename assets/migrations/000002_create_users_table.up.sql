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
    summarized_resume TEXT
);

-- Create function for updating timestamps if it doesn't exist
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';