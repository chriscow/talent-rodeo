-- Drop index first
DROP INDEX IF EXISTS idx_candidates_subscription_end_date;

-- Drop subscription_end_date column
ALTER TABLE candidates
DROP COLUMN IF EXISTS subscription_end_date; 