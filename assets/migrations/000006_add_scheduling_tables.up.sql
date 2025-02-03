-- Enable foreign key support
PRAGMA foreign_keys = ON;

-- Create appointments table
CREATE TABLE IF NOT EXISTS appointments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    scheduled_time DATETIME NOT NULL,
    timezone TEXT NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN (
            'scheduled',
            'completed',
            'cancelled',
            'failed'
        )
    ),
    call_sid TEXT, -- Twilio call SID
    transcript TEXT,
    transcript_summary TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Create user scheduling preferences table
CREATE TABLE IF NOT EXISTS scheduling_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    timezone TEXT NOT NULL,
    preferred_days TEXT, -- JSON array of preferred days (e.g., ["monday", "wednesday"])
    preferred_time_start TEXT, -- Time in 24hr format (e.g., "09:00")
    preferred_time_end TEXT, -- Time in 24hr format (e.g., "17:00")
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Create job search criteria table
CREATE TABLE IF NOT EXISTS job_search_criteria (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    criteria_type TEXT NOT NULL, -- e.g., 'work_location', 'company_size', 'culture'
    criteria_value TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'phone_interview', -- Where this criteria came from
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Create indexes for common queries
CREATE INDEX idx_appointments_user_time ON appointments (user_id, scheduled_time);

CREATE INDEX idx_appointments_status ON appointments (status);

CREATE INDEX idx_job_search_criteria_user ON job_search_criteria (user_id);

-- Create triggers to update the updated_at timestamp
CREATE TRIGGER appointments_updated_at 
    AFTER UPDATE ON appointments
BEGIN
    UPDATE appointments 
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

CREATE TRIGGER scheduling_preferences_updated_at 
    AFTER UPDATE ON scheduling_preferences
BEGIN
    UPDATE scheduling_preferences 
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

CREATE TRIGGER job_search_criteria_updated_at 
    AFTER UPDATE ON job_search_criteria
BEGIN
    UPDATE job_search_criteria 
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;