-- Enable foreign key support
PRAGMA foreign_keys = ON;

-- Create main jobs table
CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_position TEXT NOT NULL,
    job_status TEXT,
    job_location TEXT,
    company_name TEXT NOT NULL,
    company_linkedin_id TEXT,
    posting_time TEXT,
    base_pay TEXT,
    job_description TEXT,
    seniority_level TEXT,
    employment_type TEXT,
    job_function TEXT,
    industries TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create recruiters table
CREATE TABLE IF NOT EXISTS recruiters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    photo_url TEXT,
    name TEXT NOT NULL,
    title TEXT,
    profile_url TEXT,
    FOREIGN KEY (job_id) REFERENCES jobs (id) ON DELETE CASCADE
);

-- Create similar jobs table
CREATE TABLE IF NOT EXISTS similar_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_job_id INTEGER NOT NULL,
    job_position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    job_link TEXT,
    FOREIGN KEY (original_job_id) REFERENCES jobs (id) ON DELETE CASCADE
);

-- Create people also viewed table
CREATE TABLE IF NOT EXISTS people_also_viewed (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_job_id INTEGER NOT NULL,
    job_position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    job_link TEXT,
    FOREIGN KEY (original_job_id) REFERENCES jobs (id) ON DELETE CASCADE
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_jobs_company_name ON jobs (company_name);

CREATE INDEX IF NOT EXISTS idx_jobs_position ON jobs (job_position);

CREATE INDEX IF NOT EXISTS idx_jobs_location ON jobs (job_location);

CREATE INDEX IF NOT EXISTS idx_similar_jobs_company ON similar_jobs (company);

CREATE INDEX IF NOT EXISTS idx_people_also_viewed_company ON people_also_viewed (company);

-- Create trigger to update the updated_at timestamp
CREATE TRIGGER IF NOT EXISTS jobs_updated_at 
    AFTER UPDATE ON jobs
BEGIN
    UPDATE jobs 
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;


CREATE TABLE IF NOT EXISTS job_searches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword TEXT NOT NULL,
    geo_id TEXT NOT NULL,
    sort_by TEXT,
    job_type TEXT,
    exp_level TEXT,
    work_type TEXT,
    company_filter TEXT,
    last_searched_at DATETIME NOT NULL,
    total_results INTEGER,
    search_hash TEXT NOT NULL,
    UNIQUE(search_hash)
);

CREATE INDEX IF NOT EXISTS idx_job_searches_hash ON job_searches(search_hash);
CREATE INDEX IF NOT EXISTS idx_job_searches_last_searched ON job_searches(last_searched_at);