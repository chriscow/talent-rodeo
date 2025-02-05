# Models Package

This package defines the core data structures and business logic objects used throughout the Talent Rodeo application.

## Overview

The models package contains the fundamental data structures that represent the application's domain objects. Each model is designed to map cleanly to both database tables and JSON representations, with built-in validation and business logic.

## Files

### task.go
Task model implementation including:
- Task status management
- Background job representation
- Task lifecycle methods
- Payload handling
- Retry logic

Key features:
- JSON payload serialization/deserialization
- Status tracking and validation
- Retry mechanism
- Scheduling logic
- Database mapping

### message.go
Message model implementation including:
- Phone call message representation
- Contact information
- Message content
- Timestamp tracking

Key features:
- Database mapping
- JSON serialization
- Contact details
- Message history

## Key Features

1. Task Management
   - Status constants (pending, running, complete, failed, cancelled, retrying)
   - Payload handling with JSON serialization
   - Retry mechanism with attempt tracking
   - Scheduling with run-at timing
   - Error tracking and reporting

2. Message Handling
   - Unique message identification
   - Sender/receiver tracking
   - Message content storage
   - Creation timestamp tracking
   - Database and JSON mapping

3. Common Features
   - Database tag mapping
   - JSON serialization
   - Timestamp handling
   - Error handling
   - Validation logic

## Usage

Task creation and management:
```go
// Create a new task
task, err := models.NewTask("email_send", map[string]interface{}{
    "to": "user@example.com",
    "subject": "Welcome",
})

// Check task status
if task.IsComplete() {
    // Handle completion
}

// Check retry status
if task.CanRetry() {
    // Handle retry
}
```

Message handling:
```go
// Create a new message
message := &models.Message{
    To:   "+1234567890",
    From: "+0987654321",
    Body: "Hello, world!",
}
```

## Database Integration

The models are designed to work seamlessly with the database package:
- `db` tags for SQL mapping
- `json` tags for API responses
- Consistent field naming
- Type compatibility

## Validation

Each model implements appropriate validation:
1. Task Status
   - Valid status transitions
   - Attempt limits
   - Scheduling constraints

2. Message Format
   - Required fields
   - Format validation
   - Content constraints

## Error Handling

The package provides clear error handling:
- Payload serialization errors
- Status validation errors
- Constraint violations
- Type conversion errors 