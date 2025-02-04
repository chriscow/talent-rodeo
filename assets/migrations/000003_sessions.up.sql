CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    expiry TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sessions_expiry ON sessions (expiry);

-- Create trigger for updating timestamps
CREATE TRIGGER sessions_updated_at BEFORE
UPDATE ON sessions FOR EACH ROW
EXECUTE FUNCTION update_timestamp ();