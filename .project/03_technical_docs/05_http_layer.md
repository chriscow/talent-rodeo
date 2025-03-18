# HTTP Layer Architecture

The HTTP layer in Talent Rodeo handles web requests and is primarily implemented in the `cmd/web` package. It follows a clean, modular design pattern with separation of concerns.

## Overview

The HTTP layer is built around the following principles:
- Handler-based architecture
- Middleware for cross-cutting concerns
- Clean routing with Flow router
- Centralized error handling
- Dependency injection via application struct

## Core Components

### Application Structure (`cmd/web/main.go`)

The `application` struct serves as a dependency container for handlers and middleware:

```go
type application struct {
    config     config
    logger     *slog.Logger
    db         *database.DB
    templates  *templates.Cache
    auth       *auth.Service
    mailClient *smtp.Client
    tasks      *tasks.Processor
    voice      *voice.Service
    wg         sync.WaitGroup
}
```

### Request Handler Pattern

Handlers are implemented as methods on the `application` struct:

```go
func (app *application) handlerName(w http.ResponseWriter, r *http.Request) {
    // Handler implementation
}
```

### Router Configuration (`cmd/web/routes.go`)

Routes are defined using the Flow router in the `routes` method:

```go
func (app *application) routes() http.Handler {
    mux := flow.New()
    
    // Middleware
    mux.Use(app.recoverPanic)
    mux.Use(app.logRequest)
    mux.Use(app.secureHeaders)
    
    // Routes
    mux.HandleFunc("/", app.home, "GET")
    mux.HandleFunc("/login", app.login, "GET", "POST")
    
    // Authentication-required routes
    mux.Group(func(mux *flow.Mux) {
        mux.Use(app.requireAuth)
        
        mux.HandleFunc("/tasks", app.listTasks, "GET")
        mux.HandleFunc("/tasks/create", app.createTask, "POST")
    })
    
    return mux
}
```

## Handler Categories

### Authentication Handlers (`cmd/web/handlers.auth.go`)

Handles:
- User login
- OAuth2 flow
- Session management
- User registration
- Authentication checks

### Task Handlers (`cmd/web/handlers.tasks.go`)

Handles:
- Task listing
- Task creation
- Task updates
- Task status checks

### Search Handlers (`cmd/web/handlers.search.go`)

Handles:
- Job search operations
- Search criteria processing
- Result pagination
- Result filtering

### Resume Handlers (`cmd/web/handlers.resume.go`)

Handles:
- Resume upload
- Resume parsing
- Resume analysis
- Skill extraction

### Interview Handlers (`cmd/web/handlers.interview.go`)

Handles:
- Interview scheduling
- Interview feedback
- Mock interviews
- Interview evaluation

### Skills Handlers (`cmd/web/handlers.skills.go`)

Handles:
- Skill management
- Skill assessment
- Skill matching
- Skill recommendations

### User Handlers (`cmd/web/handlers.user.go`)

Handles:
- User profile management
- User preferences
- Account settings
- User tiers

## Middleware

Middleware is implemented in `cmd/web/middleware.go`:

### Key Middleware Components

1. **Authentication Middleware**
   - `requireAuth`: Ensures the user is authenticated
   - `loadAuthUser`: Loads the authenticated user into the request context

2. **Security Middleware**
   - `secureHeaders`: Sets security-related HTTP headers
   - `recoverPanic`: Recovers from panics in handlers
   - `rateLimit`: Limits request rates per IP

3. **Logging Middleware**
   - `logRequest`: Logs incoming HTTP requests
   - `requestTimer`: Measures and logs request durations

4. **CORS Middleware**
   - `enableCORS`: Handles Cross-Origin Resource Sharing

## Error Handling

Error handling is centralized in `cmd/web/errors.go`:

- `app.serverError`: Logs and responds with server errors
- `app.clientError`: Responds with client errors
- `app.notFound`: Handles 404 errors
- `app.methodNotAllowed`: Handles 405 errors
- `app.badRequest`: Handles 400 errors

## Context Management

Request context is managed in `cmd/web/context.go` with:
- `contextKey` type for context values
- Helper functions for setting and retrieving values
- Type-safe context access

## Request Processing Flow

1. **Request Receipt**:
   - HTTP request is received by the server
   - Flow router matches the request to a handler
   - Middleware chain is executed

2. **Authentication and Authorization**:
   - User authentication is checked
   - Authorization rules are applied
   - User is loaded into context

3. **Handler Execution**:
   - Appropriate handler is executed
   - Request parameters are parsed
   - Business logic is invoked
   - Database operations are performed

4. **Response Generation**:
   - Response data is prepared
   - Templates are rendered
   - JSON responses are formed
   - Headers are set

5. **Error Handling**:
   - Errors are captured
   - Appropriate error responses are sent
   - Errors are logged

## Tests

Handler tests are implemented in corresponding `*_test.go` files:
- `handlers_test.go`: Base test utilities
- `handlers.auth_test.go`: Authentication tests
- `handlers.tasks_test.go`: Task handler tests
- `handlers.search_test.go`: Search handler tests
- `handlers.resume_test.go`: Resume handler tests
- `handlers.skills_test.go`: Skills handler tests
- `handlers.interview_test.go`: Interview handler tests

## Files

- Server entry point: `cmd/web/main.go`
- Route definitions: `cmd/web/routes.go`
- Middleware: `cmd/web/middleware.go`
- Base handlers: `cmd/web/handlers.go`
- Authentication handlers: `cmd/web/handlers.auth.go`
- Tasks handlers: `cmd/web/handlers.tasks.go`
- Search handlers: `cmd/web/handlers.search.go`
- Resume handlers: `cmd/web/handlers.resume.go`
- Skills handlers: `cmd/web/handlers.skills.go`
- Interview handlers: `cmd/web/handlers.interview.go`
- Error handling: `cmd/web/errors.go`
- Context management: `cmd/web/context.go`
- Server configuration: `cmd/web/server.go` 