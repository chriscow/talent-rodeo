CREATE TABLE users (
    id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    created TIMESTAMP NOT NULL DEFAULT (datetime('now', 'utc')),
    email TEXT NOT NULL UNIQUE,
    hashed_secret TEXT NOT NULL,
    provider TEXT NOT NULL,
    name TEXT,
    first_name TEXT,
    last_name TEXT,
    nick_name TEXT,
    description TEXT,
    user_id TEXT,
    avatar_url TEXT,
    location TEXT,
    is_superuser BOOLEAN NOT NULL DEFAULT 0,
    -- 
    resume_filename TEXT,
    full_resume TEXT,
    summarized_resume TEXT,
    --
    onboarding_step INTEGER NOT NULL DEFAULT 0
);