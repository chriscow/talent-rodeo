-- Drop triggers first
DROP TRIGGER IF EXISTS match_assessments_updated_at ON match_assessments;

DROP TRIGGER IF EXISTS skills_matches_updated_at ON skills_matches;

DROP TRIGGER IF EXISTS experience_matches_updated_at ON experience_matches;

DROP TRIGGER IF EXISTS location_matches_updated_at ON location_matches;

DROP TRIGGER IF EXISTS compensation_fits_updated_at ON compensation_fits;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_match_updated_at;

-- Drop tables in reverse order of creation (to handle foreign key dependencies)
DROP TABLE IF EXISTS match_recommendations;

DROP TABLE IF EXISTS compensation_fits;

DROP TABLE IF EXISTS location_matches;

DROP TABLE IF EXISTS experience_matches;

DROP TABLE IF EXISTS missing_skills;

DROP TABLE IF EXISTS matched_skills;

DROP TABLE IF EXISTS skills_matches;

DROP TABLE IF EXISTS match_assessments;

-- Drop enum type
DROP TYPE IF EXISTS confidence_level;