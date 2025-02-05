DROP TRIGGER IF EXISTS tasks_updated_at ON tasks;

DROP INDEX IF EXISTS idx_tasks_status;

DROP INDEX IF EXISTS idx_tasks_type;

DROP INDEX IF EXISTS idx_tasks_run_at;

DROP TABLE IF EXISTS tasks;