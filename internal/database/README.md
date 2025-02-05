# Database Package

This package provides a robust database interface for the Talent Rodeo application, built on top of `sqlx` with PostgreSQL support.

## Overview

The database package implements a wrapper around `sqlx.DB` that provides additional functionality, transaction support, and a clean interface for database operations.

## Files

### db.go
Core database functionality including:
- Database connection and configuration
- Transaction management
- Connection pooling
- Migration support
- Interface definitions for database operations (DBTX interface)
- Helper methods for common database operations

### users.go
User-related database operations including:
- User CRUD operations
- User authentication
- User profile management

### task.go
Task management functionality:
- Task creation and updates
- Task status management
- Task scheduling
- Task execution tracking

### scheduling.go
Scheduling-related database operations:
- Schedule management
- Time-based operations
- Scheduling algorithms and constraints

### onboarding.go
Onboarding process database operations:
- User onboarding workflow
- Onboarding state management
- Progress tracking

### test_helpers.go
Testing utilities including:
- Test fixtures
- Database mocking
- Test setup and teardown helpers

### task_test.go
Tests for task-related functionality:
- Task CRUD operation tests
- Task scheduling tests
- Error handling tests

### database_test.go
Core database functionality tests:
- Connection tests
- Transaction tests
- Migration tests

## Key Features

1. Connection Pooling
   - Configurable connection limits
   - Connection lifetime management
   - Idle connection management

2. Transaction Support
   - Transaction wrapper with automatic rollback on error
   - Nested transaction support
   - Context-aware transactions

3. Migration Support
   - Automatic migrations
   - Migration version tracking
   - Rollback support

4. Logging
   - Structured logging with slog
   - Configurable log levels
   - Operation tracking

## Usage

The package is designed to be initialized at application startup with configuration options:

```go
db, err := database.New(dsn, true, database.WithLogger(logger))
```

Transaction example:
```go
err := db.WithTx(ctx, func(tx *database.DB) error {
    // Perform operations within transaction
    return nil
})
``` 