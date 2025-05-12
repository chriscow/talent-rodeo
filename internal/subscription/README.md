# Subscription Package

This package provides functionality for handling search criteria subscriptions and publishing job matches to users based on their preferences.

## Components

### Processor

The `Processor` is responsible for periodically checking for search criteria that need to be processed and publishing new job matches to subscribed users.

#### Key Features

- Runs on a configurable timer (default: 24 hours)
- Processes search criteria with subscribers that have recent search results
- Performs semantic search for each user based on their steering query
- Creates user job links for relevant matches
- Updates the `LastPublishedAt` timestamp on search criteria

## Usage

```go
// Create a new processor
fs, _ := internal.NewFirestoreClient(ctx)
processor := subscription.NewProcessor(fs)

// Optionally customize the interval
processor.WithInterval(12 * time.Hour)

// Start the processor (will run once immediately)
processor.Start(ctx)

// Stop the processor when the application is shutting down
defer processor.Stop()
```

## Process Flow

1. On startup and at each interval, the processor looks for eligible search criteria:
   - `SubscriberCount` > 0
   - `LastSearchedAt` within the last 24 hours
   - `LastPublishedAt` older than 24 hours or nil

2. For each eligible criteria, the processor:
   - Gets all subscribers (users) for that criteria
   - Gets jobs found in the last 24 hours for that criteria
   - For each user with a steering query, performs semantic search
   - Creates user job links for relevant matches
   - Updates `LastPublishedAt` timestamp

## Error Handling

- Errors are logged and reported to Sentry
- Processing continues for other criteria/users when an error occurs
- The processor gracefully handles shutdown 