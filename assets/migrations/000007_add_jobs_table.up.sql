-- Enable foreign key support
PRAGMA foreign_keys = ON;

-- Create background_jobs table
CREATE TABLE IF NOT EXISTS background_jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    payload BLOB,
    status TEXT NOT NULL CHECK (
        status IN (
            'pending',
            'running',
            'complete',
            'failed',
            'retrying',
            'cancelled'
        )
    ),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    run_at DATETIME NOT NULL,
    error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index for finding pending jobs
CREATE INDEX idx_background_jobs_status_run_at ON background_jobs (status, run_at)
WHERE
    status IN ('pending', 'retrying');

-- Create trigger to update the updated_at timestamp
CREATE TRIGGER background_jobs_updated_at 
    AFTER UPDATE ON background_jobs
BEGIN
    UPDATE background_jobs 
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;