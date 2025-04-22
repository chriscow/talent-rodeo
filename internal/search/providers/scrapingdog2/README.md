# ScrapingDog Provider (Refactored)

This package is a refactored implementation of the ScrapingDog job search provider, following improved software design principles including clear separation of concerns, consistent naming, and better error handling.

## Components

### Core Components

1. **API Client (`api_client.go`)**: 
   - Handles all direct API interactions with ScrapingDog
   - Makes HTTP requests with retry logic
   - Parses API responses

2. **Job Store (`store.go`)**: 
   - Manages job data persistence
   - Caches job listings and details
   - Interacts with Firestore
   - Provides clear data access methods

3. **Provider (`provider.go`)**: 
   - Implements the JobProvider interface
   - Coordinates between API Client and JobStore
   - Handles high-level search operations

4. **Worker Manager (`workers.go`)**: 
   - Handles background processing
   - Manages worker goroutines
   - Controls queue and concurrency
   - Provides clean shutdown

### Supporting Files

- **Types (`types.go`)**: Common types and constants
- **Errors (`errors.go`)**: Error definitions and handling
- **Utils (`utils.go`)**: Common utility functions

## Usage

```go
import (
    "context"
    "log/slog"
    "talent-rodeo/internal/search/providers/scrapingdog2"
)

func main() {
    // Create a new provider
    provider := scrapingdog2.NewScrapingDogProvider(
        "your-api-key",
        scrapingdog2.WithFirestore(firestoreClient),
        scrapingdog2.WithJobEventHandler(eventHandler),
        scrapingdog2.WithProviderLogger(logger),
    )

    // Start background workers
    ctx := context.Background()
    provider.StartWorkers(ctx, 5)
    defer provider.StopWorkers(ctx)

    // Perform a search
    criteria := search.Criteria{
        Keywords: []string{"software engineer"},
        Location: "remote",
    }
    
    // Synchronous search
    jobs, err := provider.SearchJobs(ctx, criteria)
    if err != nil {
        log.Fatal(err)
    }
    
    // Asynchronous search
    err = provider.QueueSearch(ctx, "user123", criteria)
    if err != nil {
        log.Fatal(err)
    }
}
```

## Design Improvements

1. **Clear Method Naming**: All methods follow a consistent, descriptive naming pattern.
2. **Separated Responsibilities**: Each component has a focused, single responsibility.
3. **Simplified Worker Logic**: Worker implementation with clear error handling patterns.
4. **Reduced Duplication**: Common operations extracted into reusable helper functions.
5. **Clear Interface Implementation**: Components explicitly implement required interfaces.
6. **Centralized Error Handling**: Unified approach to error management.
7. **Better Separation of Concerns**: Clear boundaries between API calls, job processing, and data storage.

## Configuration Options

The provider supports various configuration options:

- `WithBaseURL(url string)`: Sets a custom base URL for the API
- `WithHTTPClient(client *http.Client)`: Sets a custom HTTP client
- `WithFirestore(fs *firestore.Client)`: Sets a Firestore client
- `WithJobEventHandler(handler search.JobEventHandler)`: Sets an event handler
- `WithProviderLogger(logger *slog.Logger)`: Sets a custom logger 