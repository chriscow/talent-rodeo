-- Create companies table
CREATE TABLE companies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL, -- its entirelypossible that two companies have the same name
    description TEXT,
    website TEXT,
    industry TEXT,
    company_size TEXT,
    company_type TEXT,
    headquarters TEXT,
    founded INTEGER,
    specialties TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create jobs table
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies (id),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    location TEXT NOT NULL,
    work_type TEXT,
    employment_type TEXT,
    seniority_level TEXT,
    job_function TEXT,
    industries TEXT,
    linkedin_job_id TEXT UNIQUE,
    applies_count INTEGER,
    original_listed_time TIMESTAMP,
    experience_level TEXT,
    salary_min DECIMAL,
    salary_max DECIMAL,
    salary_currency TEXT,
    salary_period TEXT,
    benefits TEXT,
    remote_allowed BOOLEAN,
    skills_required TEXT,
    education_required TEXT,
    responsibilities TEXT,
    qualifications TEXT,
    description_html TEXT,
    description_summarized TEXT,
    company_linkedin_id TEXT,
    posting_time TEXT,
    base_pay TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create job applications table
CREATE TABLE job_applications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id),
    job_id BIGINT NOT NULL REFERENCES jobs (id),
    status TEXT NOT NULL CHECK (
        status IN (
            'interested',
            'applied',
            'interviewing',
            'offered',
            'rejected',
            'accepted',
            'declined'
        )
    ),
    notes TEXT,
    applied_date TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create job_skills table for tracking required skills
CREATE TABLE job_skills (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs (id),
    skill TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create recruiters table
CREATE TABLE recruiters (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs (id),
    photo_url TEXT,
    name TEXT NOT NULL,
    title TEXT,
    profile_url TEXT
);

-- Create similar jobs table
CREATE TABLE similar_jobs (
    id BIGSERIAL PRIMARY KEY,
    original_job_id BIGINT NOT NULL REFERENCES jobs (id),
    position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    link TEXT
);

-- Create people also viewed table
CREATE TABLE people_also_viewed (
    id BIGSERIAL PRIMARY KEY,
    original_job_id BIGINT NOT NULL REFERENCES jobs (id),
    position TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    posting_time TEXT,
    link TEXT
);

-- Create job searches table
CREATE TABLE job_searches (
    id BIGSERIAL PRIMARY KEY,
    keyword TEXT NOT NULL,
    geo_id TEXT NOT NULL,
    sort_by TEXT,
    job_type TEXT,
    exp_level TEXT,
    work_type TEXT,
    company_filter TEXT,
    last_searched_at TIMESTAMP NOT NULL,
    total_results INTEGER,
    search_hash TEXT NOT NULL UNIQUE
);

-- Create job listings table
CREATE TABLE job_listings (
    id BIGSERIAL PRIMARY KEY,
    job_id TEXT NOT NULL UNIQUE,
    position TEXT NOT NULL,
    link TEXT,
    company_name TEXT NOT NULL,
    company_profile TEXT,
    location TEXT,
    posting_date TEXT,
    company_logo TEXT,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create search results junction table
CREATE TABLE search_results (
    search_id BIGINT NOT NULL REFERENCES job_searches (id) ON DELETE CASCADE,
    job_listing_id BIGINT NOT NULL REFERENCES job_listings (id) ON DELETE CASCADE,
    found_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (search_id, job_listing_id)
);

-- Create extracted job details table
CREATE TABLE extracted_job_details (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    must_have_requirements TEXT,
    preferred_skills TEXT,
    work_location TEXT,
    company_culture TEXT,
    additional_info TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create user keywords table
CREATE TABLE user_keywords (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    keyword TEXT NOT NULL,
    priority INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, keyword)
);

-- Create indexes
CREATE INDEX idx_jobs_company ON jobs (company_id);

CREATE INDEX idx_jobs_location ON jobs (location);

CREATE INDEX idx_jobs_work_type ON jobs (work_type);

CREATE INDEX idx_jobs_employment_type ON jobs (employment_type);

CREATE INDEX idx_jobs_seniority ON jobs (seniority_level);

CREATE INDEX idx_jobs_function ON jobs (job_function);

CREATE INDEX idx_jobs_linkedin_id ON jobs (linkedin_job_id);

CREATE INDEX idx_jobs_experience ON jobs (experience_level);

CREATE INDEX idx_jobs_remote ON jobs (remote_allowed);

CREATE INDEX idx_jobs_title ON jobs (title);

CREATE INDEX idx_applications_user ON job_applications (user_id);

CREATE INDEX idx_applications_job ON job_applications (job_id);

CREATE INDEX idx_applications_status ON job_applications (status);

CREATE INDEX idx_skills_job ON job_skills (job_id);

CREATE INDEX idx_skills_skill ON job_skills (skill);

CREATE INDEX idx_similar_jobs_company ON similar_jobs (company);

CREATE INDEX idx_people_also_viewed_company ON people_also_viewed (company);

CREATE INDEX idx_job_searches_hash ON job_searches (search_hash);

CREATE INDEX idx_job_searches_last_searched ON job_searches (last_searched_at);

CREATE INDEX idx_job_listings_company_name ON job_listings (company_name);

CREATE INDEX idx_job_listings_location ON job_listings (location);

CREATE INDEX idx_job_listings_posting_date ON job_listings (posting_date);

-- Create a new trigger function specifically for job_listings
CREATE OR REPLACE FUNCTION update_job_listings_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create the new trigger using the specific function
CREATE TRIGGER job_listings_updated_at BEFORE
UPDATE ON job_listings FOR EACH ROW
EXECUTE FUNCTION update_job_listings_timestamp ();

-- Create triggers for updating timestamps
CREATE TRIGGER companies_updated_at BEFORE
UPDATE ON companies FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER jobs_updated_at BEFORE
UPDATE ON jobs FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER job_applications_updated_at BEFORE
UPDATE ON job_applications FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER job_skills_updated_at BEFORE
UPDATE ON job_skills FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER extracted_job_details_updated_at BEFORE
UPDATE ON extracted_job_details FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();

CREATE TRIGGER user_keywords_updated_at BEFORE
UPDATE ON user_keywords FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();