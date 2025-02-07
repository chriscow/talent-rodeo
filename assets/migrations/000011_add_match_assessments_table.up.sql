-- Create enum for confidence levels
CREATE TYPE confidence_level AS ENUM('low', 'medium', 'high');

-- Create table for match assessments
CREATE TABLE IF NOT EXISTS match_assessments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    candidate_id UUID NOT NULL REFERENCES candidates (id) ON DELETE CASCADE,
    job_id TEXT NOT NULL, -- External job ID from providers
    score DECIMAL(4, 3) NOT NULL CHECK (
        score >= 0
        AND score <= 1
    ),
    confidence confidence_level NOT NULL,
    analysis TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for skills matches
CREATE TABLE IF NOT EXISTS skills_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    assessment_id UUID NOT NULL REFERENCES match_assessments (id) ON DELETE CASCADE,
    score DECIMAL(4, 3) NOT NULL CHECK (
        score >= 0
        AND score <= 1
    ),
    gap_analysis TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for matched skills
CREATE TABLE IF NOT EXISTS matched_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    skills_match_id UUID NOT NULL REFERENCES skills_matches (id) ON DELETE CASCADE,
    skill_name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for missing skills
CREATE TABLE IF NOT EXISTS missing_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    skills_match_id UUID NOT NULL REFERENCES skills_matches (id) ON DELETE CASCADE,
    skill_name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for experience matches
CREATE TABLE IF NOT EXISTS experience_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    assessment_id UUID NOT NULL REFERENCES match_assessments (id) ON DELETE CASCADE,
    score DECIMAL(4, 3) NOT NULL CHECK (
        score >= 0
        AND score <= 1
    ),
    years_match BOOLEAN NOT NULL,
    level_match BOOLEAN NOT NULL,
    role_alignment TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for location matches
CREATE TABLE IF NOT EXISTS location_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    assessment_id UUID NOT NULL REFERENCES match_assessments (id) ON DELETE CASCADE,
    score DECIMAL(4, 3) NOT NULL CHECK (
        score >= 0
        AND score <= 1
    ),
    compatible BOOLEAN NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for compensation fits
CREATE TABLE IF NOT EXISTS compensation_fits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    assessment_id UUID NOT NULL REFERENCES match_assessments (id) ON DELETE CASCADE,
    score DECIMAL(4, 3) NOT NULL CHECK (
        score >= 0
        AND score <= 1
    ),
    compatible BOOLEAN NOT NULL,
    explanation TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create table for recommendations
CREATE TABLE IF NOT EXISTS match_recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    assessment_id UUID NOT NULL REFERENCES match_assessments (id) ON DELETE CASCADE,
    area TEXT NOT NULL,
    suggestion TEXT NOT NULL,
    priority TEXT NOT NULL CHECK (
        priority IN ('high', 'medium', 'low')
    ),
    time_to_ready TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for foreign keys and common queries
CREATE INDEX idx_match_assessments_candidate_id ON match_assessments (candidate_id);

CREATE INDEX idx_match_assessments_job_id ON match_assessments (job_id);

CREATE INDEX idx_match_assessments_score ON match_assessments (score);

CREATE INDEX idx_skills_matches_assessment_id ON skills_matches (assessment_id);

CREATE INDEX idx_experience_matches_assessment_id ON experience_matches (assessment_id);

CREATE INDEX idx_location_matches_assessment_id ON location_matches (assessment_id);

CREATE INDEX idx_compensation_fits_assessment_id ON compensation_fits (assessment_id);

CREATE INDEX idx_match_recommendations_assessment_id ON match_recommendations (assessment_id);

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_match_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Add triggers for all tables with updated_at
CREATE TRIGGER match_assessments_updated_at BEFORE
UPDATE ON match_assessments FOR EACH ROW
EXECUTE FUNCTION update_match_updated_at ();

CREATE TRIGGER skills_matches_updated_at BEFORE
UPDATE ON skills_matches FOR EACH ROW
EXECUTE FUNCTION update_match_updated_at ();

CREATE TRIGGER experience_matches_updated_at BEFORE
UPDATE ON experience_matches FOR EACH ROW
EXECUTE FUNCTION update_match_updated_at ();

CREATE TRIGGER location_matches_updated_at BEFORE
UPDATE ON location_matches FOR EACH ROW
EXECUTE FUNCTION update_match_updated_at ();

CREATE TRIGGER compensation_fits_updated_at BEFORE
UPDATE ON compensation_fits FOR EACH ROW
EXECUTE FUNCTION update_match_updated_at ();