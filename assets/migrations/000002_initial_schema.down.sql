-- Drop all tables in reverse order

-- Drop tasks table and trigger
DROP TRIGGER IF EXISTS update_tasks_timestamp ON tasks;

DROP TABLE IF EXISTS tasks;

-- Drop availability table and trigger
DROP TRIGGER IF EXISTS update_availability_timestamp ON availability;

DROP TABLE IF EXISTS availability;

-- Drop onboarding table and trigger
DROP TRIGGER IF EXISTS update_onboarding_states_timestamp ON onboarding_states;

DROP TABLE IF EXISTS onboarding_states;

-- Drop linkedin_jobs table and trigger
DROP TRIGGER IF EXISTS update_linkedin_jobs_timestamp ON linkedin_jobs;

DROP TABLE IF EXISTS linkedin_jobs;

-- Drop sessions table
DROP TABLE IF EXISTS sessions;

-- Drop users table and trigger
DROP TRIGGER IF EXISTS update_users_timestamp ON users;

DROP TABLE IF EXISTS users;

-- Drop the update_timestamp function
DROP FUNCTION IF EXISTS update_timestamp ();