# Database Architecture

The database layer in Talent Rodeo is implemented using PostgreSQL with the `sqlx` library for enhanced functionality. The core database code is located in the `internal/database` package.

## Overview

The database architecture follows a clean design with:
- A core database interface (`DBTX`)
- Transaction support
- Migration management
- Comprehensive error handling
- Structured logging

## Core Components

### Database Connection (`internal/database/db.go`)

The `DB` struct is a wrapper around `sqlx.DB` that provides additional functionality:
- Connection pooling
- Transaction management
- Logging integration
- Error handling

### Database Interface

The `DBTX` interface abstracts both regular database connections and transactions:

```go
type DBTX interface {
    Exec(query string, args ...any) (sql.Result, error)
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    Get(dest any, query string, args ...any) error
    GetContext(ctx context.Context, dest any, query string, args ...any) error
    Prepare(query string) (*sql.Stmt, error)
    PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
    Query(query string, args ...any) (*sql.Rows, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    Select(dest any, query string, args ...any) error
    SelectContext(ctx context.Context, dest any, query string, args ...any) error
}
```

This interface allows for consistent interaction with both regular database connections and transactions.

## Key Database Files

### Core Database (`internal/database/db.go`)

Contains:
- Database initialization
- Transaction support
- Connection pooling configuration
- Database interface definitions
- Helper functions

### User Operations (`internal/database/users.go`)

Contains functions for:
- User creation and retrieval
- Authentication checks
- Profile management
- User preferences
- User tier management

### Task Operations (`internal/database/task.go`)

Contains functions for:
- Task creation
- Task updates
- Task retrieval
- Task queue management
- Transaction support for task processing

### Skills Management (`internal/database/skills.go`)

Contains functions for:
- Skill creation and retrieval
- Skill validation
- Skill tagging
- Skill matching

### Scheduling (`internal/database/scheduling.go`)

Contains functions for:
- Interview scheduling
- Time slot management
- Calendar integration
- Timezone handling

## Transaction Management

The database package provides transaction support via the `WithTx` method:

```go
func (db *DB) WithTx(ctx context.Context, fn func(*DB) error) error {
    tx, err := db.BeginTxx(ctx, nil)
    if err != nil {
        return err
    }
    
    txDb := &DB{
        DB:     db.DB,
        Tx:     tx,
        Logger: db.Logger,
    }
    
    defer func() {
        if p := recover(); p != nil {
            _ = tx.Rollback()
            panic(p)
        }
    }()
    
    if err := fn(txDb); err != nil {
        _ = tx.Rollback()
        return err
    }
    
    return tx.Commit()
}
```

This method ensures that transactions are properly committed or rolled back, even in the case of panics.

## Migration Management

Database migrations are stored in the `assets/migrations` directory and are managed using the `golang-migrate/migrate` package.

## Testing Utilities

The `internal/database/test_helpers.go` file contains utilities for:
- Setting up test databases
- Executing test transactions
- Creating test fixtures
- Cleaning up test data

## Best Practices

1. **Use Transactions for Consistency**
   - Ensure related operations are wrapped in transactions
   - Use the `WithTx` method for transaction safety

2. **Error Handling**
   - Check and return errors appropriately
   - Use custom error types for specific database errors
   - Log database errors with context

3. **Query Optimization**
   - Use prepared statements for repeated queries
   - Avoid N+1 query problems with joins
   - Use indexes for frequently queried columns

4. **Connection Management**
   - Configure connection pooling based on workload
   - Set appropriate connection timeouts
   - Monitor connection usage

## Files

- Core database: `internal/database/db.go`
- User operations: `internal/database/users.go`
- Task operations: `internal/database/task.go`
- Skills management: `internal/database/skills.go`
- Scheduling: `internal/database/scheduling.go`
- Test helpers: `internal/database/test_helpers.go` 