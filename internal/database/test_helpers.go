package database

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var (
	testDBMutex sync.Mutex
	testDBs     = make(map[string]*DB)
)

const (
	testDBPort     = 5432
	testDBUser     = "postgres"
	testDBPassword = "postgres"
	testDBPrefix   = "talent_rodeo_test"
)

// SetupTestDB creates a new test database connection.
// This function is safe for concurrent use.
func SetupTestDB(t *testing.T) *DB {
	t.Helper()

	/*
		POSTGRES_USER=postgres
		POSTGRES_PASSWORD=postgres
		POSTGRES_DB=postgres
		POSTGRES_HOSTNAME=db
	*/

	// Get database host from environment variable, default to localhost
	testDBHost := os.Getenv("POSTGRES_HOSTNAME")
	if testDBHost == "" {
		testDBHost = "db"
	}

	testDBMutex.Lock()
	defer testDBMutex.Unlock()

	// Create a unique database name for this test
	testDBName := fmt.Sprintf("%s_%d", testDBPrefix, time.Now().UnixNano())

	// If we already have a test database for this name, return it
	if db, exists := testDBs[testDBName]; exists {
		return db
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

	// Create test database
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Register cleanup function
	t.Cleanup(func() {
		cleanupTestDB(t, testDBName)
	})

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
	db := &DB{
		dbtx: sqlxDB,
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}

	// Store the database instance
	testDBs[testDBName] = db

	// Run migrations
	if err := runMigrations(db); err != nil {
		cleanupTestDB(t, testDBName)
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

func cleanupTestDB(t *testing.T, dbName string) {
	t.Helper()

	testDBMutex.Lock()
	defer testDBMutex.Unlock()

	// Get the database instance
	db, exists := testDBs[dbName]
	if !exists {
		return
	}

	// Close the database connection
	if err := db.Close(); err != nil {
		t.Logf("warning: failed to close test database connection: %v", err)
	}

	// Remove from the map
	delete(testDBs, dbName)

	// Connect to postgres to drop the test database
	adminDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
		os.Getenv("POSTGRES_HOSTNAME"), testDBPort, testDBUser, testDBPassword,
	)
	adminDB, err := sqlx.Connect("postgres", adminDSN)
	if err != nil {
		t.Logf("warning: failed to connect to postgres for cleanup: %v", err)
		return
	}
	defer adminDB.Close()

	// Terminate all connections to the test database
	_, err = adminDB.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s'
		AND pid <> pg_backend_pid();`, dbName))
	if err != nil {
		t.Logf("warning: failed to terminate existing connections: %v", err)
	}

	// Drop the test database
	_, err = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	if err != nil {
		t.Logf("warning: failed to drop test database: %v", err)
	}
}

func runMigrations(db *DB) error {
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

	migrationsPath := filepath.Join(workspaceRoot, "assets", "migrations")

	// Log migration path for debugging
	db.logger.Debug("running migrations",
		"workspace_root", workspaceRoot,
		"migration_path", migrationsPath,
	)

	// Read all migration files from the directory
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Filter and sort migration files
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Apply migrations in order
	for _, file := range migrationFiles {
		path := filepath.Join(migrationsPath, file)
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
