-- Enable foreign key support
PRAGMA foreign_keys = ON;

-- Create main jobs table
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    position TEXT NOT NULL,
    status TEXT,
    location TEXT,
    company_name TEXT NOT NULL,
    company_linkedin_id TEXT,
    posting_time TEXT,
    base_pay TEXT,
    description TEXT,
    seniority_level TEXT,
    employment_type TEXT,
    function TEXT,
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
    position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    link TEXT,
    FOREIGN KEY (original_job_id) REFERENCES jobs (id) ON DELETE CASCADE
);

-- Create people also viewed table
CREATE TABLE IF NOT EXISTS people_also_viewed (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_job_id INTEGER NOT NULL,
    position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    link TEXT,
    FOREIGN KEY (original_job_id) REFERENCES jobs (id) ON DELETE CASCADE
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_jobs_company_name ON jobs (company_name);

CREATE INDEX IF NOT EXISTS idx_jobs_position ON jobs (position);

CREATE INDEX IF NOT EXISTS idx_jobs_location ON jobs (location);

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

-- Create jobs table to store LinkedIn job listings
CREATE TABLE IF NOT EXISTS job_listings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL UNIQUE,              -- LinkedIn's job ID
    position TEXT NOT NULL,                   -- Job title/position
    link TEXT,                           -- Full URL to job posting
    company_name TEXT NOT NULL,               -- Company name
    company_profile TEXT,                     -- LinkedIn company profile URL
    location TEXT,                           -- Job location
    posting_date TEXT,                        -- Original posting date from LinkedIn
    company_logo TEXT,                        -- Company logo URL
    first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,  -- When we first discovered this job
    last_updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP -- When we last updated this job
);

-- Create indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_job_listings_company_name 
    ON job_listings(company_name);
CREATE INDEX IF NOT EXISTS idx_job_listings_location 
    ON job_listings(location);
CREATE INDEX IF NOT EXISTS idx_job_listings_posting_date 
    ON job_listings(posting_date);

-- Create a junction table to connect searches with results
CREATE TABLE IF NOT EXISTS search_results (
    search_id INTEGER NOT NULL,
    job_listing_id INTEGER NOT NULL,
    found_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (search_id, job_listing_id),
    FOREIGN KEY (search_id) REFERENCES job_searches(id) ON DELETE CASCADE,
    FOREIGN KEY (job_listing_id) REFERENCES job_listings(id) ON DELETE CASCADE
);

-- Create a trigger to update last_updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_job_listing_timestamp 
    AFTER UPDATE ON job_listings
BEGIN
    UPDATE job_listings 
    SET last_updated_at = CURRENT_TIMESTAMP 
    WHERE id = NEW.id;
END;

CREATE TABLE IF NOT EXISTS extracted_job_details (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    must_have_requirements TEXT,
    preferred_skills TEXT,
    work_location TEXT,
    company_culture TEXT,
    additional_info TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);