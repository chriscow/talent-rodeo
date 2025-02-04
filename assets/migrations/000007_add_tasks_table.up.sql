CREATE TABLE tasks (
    id BIGINT PRIMARY KEY,
    type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL CHECK (
        status IN (
            'pending',
            'running',
            'complete',
            'failed',
            'cancelled',
            'retrying'
        )
    ),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    run_at TIMESTAMP NOT NULL,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Add constraint to prevent tasks with run_at in the past
    CONSTRAINT tasks_run_at_future_check CHECK (run_at >= created_at)
);

-- Index for finding pending tasks
CREATE INDEX tasks_pending_idx ON tasks (status, run_at)
WHERE
    status IN ('pending', 'retrying');

-- Use the standard update_timestamp function
CREATE TRIGGER tasks_updated_at BEFORE
UPDATE ON tasks FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();