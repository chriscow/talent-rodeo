-- Create tasks table for background job processing
CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (
        status IN (
            'pending',
            'in_progress',
            'completed',
            'failed'
        )
    ),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    run_at TIMESTAMP NOT NULL,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX idx_tasks_status ON tasks (status);

CREATE INDEX idx_tasks_type ON tasks (type);

CREATE INDEX idx_tasks_run_at ON tasks (run_at);

-- Create trigger for updating timestamps
CREATE TRIGGER tasks_updated_at BEFORE
UPDATE ON tasks FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();