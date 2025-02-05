package database

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var (
	testDBMutex sync.Mutex
	testDB      *DB
)

const (
	testDBHost     = "localhost"
	testDBPort     = 5432
	testDBUser     = "postgres"
	testDBPassword = "postgres"
	testDBName     = "talent_rodeo_test"
)

// SetupTestDB creates a new test database connection.
// This function is safe for concurrent use.
func SetupTestDB(t *testing.T) *DB {
	t.Helper()

	testDBMutex.Lock()
	defer testDBMutex.Unlock()

	// If we already have a test database, close it first
	if testDB != nil {
		if err := testDB.Close(); err != nil {
			t.Logf("warning: failed to close existing test database: %v", err)
		}
		testDB = nil
	}

	// Create test database connection string
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		testDBHost, testDBPort, testDBUser, testDBPassword, testDBName,
	)

	// First connect to postgres to ensure test database exists
	adminDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
		testDBHost, testDBPort, testDBUser, testDBPassword,
	)
	adminDB, err := sqlx.Connect("postgres", adminDSN)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer adminDB.Close()

	// Terminate all connections to the test database
	_, err = adminDB.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s'
		AND pid <> pg_backend_pid();`, testDBName))
	if err != nil {
		t.Logf("warning: failed to terminate existing connections: %v", err)
	}

	// Drop and recreate test database
	_, err = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	if err != nil {
		t.Fatalf("failed to drop test database: %v", err)
	}

	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Connect to test database
	sqlxDB, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Set reasonable connection limits for tests
	sqlxDB.SetMaxOpenConns(5)
	sqlxDB.SetMaxIdleConns(5)
	sqlxDB.SetConnMaxLifetime(5 * time.Minute)

	// Create DB instance with test options
	testDB = &DB{
		dbtx: sqlxDB,
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}

	// Run migrations
	if err := runMigrations(testDB); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Register cleanup function
	t.Cleanup(func() {
		testDBMutex.Lock()
		defer testDBMutex.Unlock()

		if testDB != nil {
			if err := testDB.Close(); err != nil {
				t.Logf("warning: failed to close test database: %v", err)
			}
			testDB = nil
		}
	})

	return testDB
}

func runMigrations(db *DB) error {
	// Read and execute migration files
	var migrationFiles = []string{
		"000001_initalize_schema_migrations.up.sql",
		"000002_create_users_table.up.sql",
		"000003_sessions.up.sql",
		"000004_linkedin_jobs.up.sql",
		"000005_enhance_onboarding.up.sql",
		"000006_add_scheduling_tables.up.sql",
		"000007_add_tasks_table.up.sql",
		"000008_add_user_phone.up.sql",
	}

	// Get the absolute path to the workspace root
	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		// If WORKSPACE_ROOT is not set, try to find it by walking up the directory tree
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		for dir := currentDir; dir != "/"; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				workspaceRoot = dir
				break
			}
		}

		if workspaceRoot == "" {
			return fmt.Errorf("failed to find workspace root (directory containing go.mod)")
		}
	}

	// Log migration path for debugging
	db.logger.Debug("running migrations",
		"workspace_root", workspaceRoot,
		"migration_path", filepath.Join(workspaceRoot, "assets", "migrations"),
	)

	for _, file := range migrationFiles {
		path := filepath.Join(workspaceRoot, "assets", "migrations", file)
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		db.logger.Debug("applying migration", "file", file)
		_, err = db.Exec(string(sql))
		if err != nil {
			return fmt.Errorf("failed to execute migration file %s: %w", file, err)
		}
	}

	return nil
}

// Helper function to wait for a condition with timeout
func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()

	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for condition")
		case <-tick:
			if condition() {
				return
			}
		}
	}
}
