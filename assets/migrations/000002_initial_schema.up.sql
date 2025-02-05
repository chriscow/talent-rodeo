-- Flattened schema migration
-- This combines all previous migrations into a single file

-- Create the update_timestamp function for all tables to use
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    hashed_password VARCHAR(255) NOT NULL,
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    phone_number VARCHAR(20),
    created_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add trigger for users table
CREATE TRIGGER update_users_timestamp BEFORE
UPDATE ON users FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

-- Create sessions table
CREATE TABLE IF NOT EXISTS sessions (
    token VARCHAR(43) PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expiry TIMESTAMP
    WITH
        TIME ZONE NOT NULL,
        created_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create LinkedIn jobs table
CREATE TABLE IF NOT EXISTS linkedin_jobs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id VARCHAR(255) NOT NULL,
    company_name VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    location VARCHAR(255),
    listing_url TEXT NOT NULL,
    apply_url TEXT,
    description TEXT,
    salary_min INTEGER,
    salary_max INTEGER,
    salary_currency VARCHAR(10),
    salary_period VARCHAR(20),
    experience_level VARCHAR(50),
    employment_type VARCHAR(50),
    job_function VARCHAR(100),
    industries TEXT[],
    benefits TEXT[],
    skills TEXT[],
    qualifications TEXT[],
    responsibilities TEXT[],
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, job_id)
);

-- Add trigger for linkedin_jobs table
CREATE TRIGGER update_linkedin_jobs_timestamp BEFORE
UPDATE ON linkedin_jobs FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

-- Create onboarding tables
CREATE TABLE IF NOT EXISTS onboarding_states (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    current_step VARCHAR(50) NOT NULL,
    data JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        UNIQUE (user_id)
);

-- Add trigger for onboarding_states table
CREATE TRIGGER update_onboarding_states_timestamp BEFORE
UPDATE ON onboarding_states FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

-- Create scheduling tables
CREATE TABLE IF NOT EXISTS availability (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day_of_week INTEGER NOT NULL CHECK (
        day_of_week >= 0
        AND day_of_week <= 6
    ),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    created_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        UNIQUE (
            user_id,
            day_of_week,
            start_time,
            end_time
        )
);

-- Add trigger for availability table
CREATE TRIGGER update_availability_timestamp BEFORE
UPDATE ON availability FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

-- Create tasks table
CREATE TABLE IF NOT EXISTS tasks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    data JSONB NOT NULL DEFAULT '{}',
    scheduled_for TIMESTAMP
    WITH
        TIME ZONE,
        completed_at TIMESTAMP
    WITH
        TIME ZONE,
        created_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP
    WITH
        TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add trigger for tasks table
CREATE TRIGGER update_tasks_timestamp BEFORE
UPDATE ON tasks FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();