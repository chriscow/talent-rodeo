-- Add subscription_end_date column to candidates table
ALTER TABLE candidates
ADD COLUMN subscription_end_date TIMESTAMP WITH TIME ZONE;

-- Add index for efficient subscription expiration queries
CREATE INDEX idx_candidates_subscription_end_date ON candidates(subscription_end_date); 