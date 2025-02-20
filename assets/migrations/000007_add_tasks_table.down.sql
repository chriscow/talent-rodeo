DROP TRIGGER IF EXISTS tasks_updated_at ON tasks;

DROP INDEX IF EXISTS tasks_pending_idx;

DROP TABLE IF EXISTS tasks;