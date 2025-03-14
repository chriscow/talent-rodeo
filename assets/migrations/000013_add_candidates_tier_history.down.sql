DO $$ 
BEGIN
    -- Drop the foreign key constraint if it exists
    IF EXISTS (
        SELECT 1 
        FROM information_schema.table_constraints 
        WHERE constraint_name = 'candidates_tier_history_candidate_id_fkey'
    ) THEN
        ALTER TABLE candidates_tier_history
        DROP CONSTRAINT candidates_tier_history_candidate_id_fkey;
    END IF;

    -- Drop the index if it exists
    IF EXISTS (
        SELECT 1 
        FROM pg_indexes 
        WHERE tablename = 'candidates_tier_history' 
        AND indexname = 'idx_candidates_tier_history_candidate_id'
    ) THEN
        DROP INDEX idx_candidates_tier_history_candidate_id;
    END IF;

    -- Drop the table if it exists
    DROP TABLE IF EXISTS candidates_tier_history;
END $$; 