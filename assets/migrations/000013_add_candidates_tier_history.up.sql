DO $$ 
BEGIN
    -- Create the table if it doesn't exist
    CREATE TABLE IF NOT EXISTS candidates_tier_history (
        id SERIAL PRIMARY KEY,
        candidate_id UUID NOT NULL,
        tier tier_type NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );

    -- Add foreign key constraint if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.table_constraints 
        WHERE constraint_name = 'candidates_tier_history_candidate_id_fkey'
    ) THEN
        ALTER TABLE candidates_tier_history
        ADD CONSTRAINT candidates_tier_history_candidate_id_fkey 
        FOREIGN KEY (candidate_id) 
        REFERENCES candidates(id) 
        ON DELETE CASCADE;
    END IF;

    -- Create index on candidate_id if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 
        FROM pg_indexes 
        WHERE tablename = 'candidates_tier_history' 
        AND indexname = 'idx_candidates_tier_history_candidate_id'
    ) THEN
        CREATE INDEX idx_candidates_tier_history_candidate_id 
        ON candidates_tier_history(candidate_id);
    END IF;
END $$; 