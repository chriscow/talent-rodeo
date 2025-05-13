# Subscription Package

This package provides functionality for handling search criteria subscriptions and publishing job matches to users based on their preferences.

## Overview

The subscription package implements a background processor that periodically checks for search criteria with recent search results and publishes relevant job matches to subscribed users. It's designed to run on a configurable interval and handle errors gracefully.

## Components

### Processor

The `Processor` is responsible for periodically checking for search criteria that need to be processed and publishing new job matches to subscribed users.

#### Key Features

- Runs on a configurable timer (default: 24 hours)
- Processes search criteria with subscribers that have recent search results
- Performs semantic search for each user based on their steering query
- Creates user job links for relevant matches
- Updates the `LastPublishedAt` timestamp on search criteria
- Handles errors gracefully and continues processing other criteria/users

#### Fields

- `fs` - Firestore client for database operations
- `logger` - Logger for recording processor activities
- `ticker` - Ticker for scheduling periodic processing
- `done` - Channel to signal shutdown
- `wg` - WaitGroup to manage goroutines
- `interval` - Processing interval (defaults to 24 hours)
- `isRunning` - Flag indicating if the processor is currently running
- `cancelFunc` - Function to cancel the processing context

## Usage

### Basic Usage

```go
// Create a new processor
ctx := context.Background()
fs, _ := internal.NewFirestoreClient(ctx)
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
processor := subscription.NewProcessor(fs, logger)

// Start the processor (will run once immediately)
processor.Start(ctx)

// Stop the processor when the application is shutting down
defer processor.Stop()
```

### Custom Interval

```go
// Create a new processor with custom interval
processor := subscription.NewProcessor(fs, logger)
processor.WithInterval(12 * time.Hour)

// Start the processor
processor.Start(ctx)
```

## Process Flow

1. **Initialization**: When `Start()` is called, the processor:
   - Sets up a ticker with the configured interval
   - Runs the process function once immediately
   - Starts a goroutine to run the process function on each tick

2. **Criteria Selection**: On each processing cycle, the processor:
   - Retrieves all search criteria from Firestore
   - Filters for eligible criteria based on:
     - `SubscriberCount` > 0
     - `LastSearchedAt` within the last 24 hours
     - `LastPublishedAt` older than 24 hours or nil

3. **Criteria Processing**: For each eligible criteria, the processor:
   - Gets all subscribers (users) for that criteria
   - Gets jobs found in the last 24 hours for that criteria
   - For each user with a steering query, performs semantic search
   - Creates user job links for relevant matches
   - Updates `LastPublishedAt` timestamp

4. **User Job Processing**: For each user, the processor:
   - Skips users without a steering query
   - Performs semantic search using the user's steering query
   - Checks if the user already has each job
   - Creates user job links for new matches

## Error Handling

- Errors are logged and reported to Sentry
- Processing continues for other criteria/users when an error occurs
- The processor gracefully handles shutdown with context cancellation
- Each processing step has appropriate error handling and logging

## Implementation Details

- The processor uses a WaitGroup to track running goroutines
- A cancellable context is used to coordinate shutdown
- The processor can be safely stopped and restarted
- Batch operations are used where possible to minimize database calls 