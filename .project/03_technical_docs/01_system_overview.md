# System Overview

Talent Rodeo is a recruiting automation platform built using Go, with a PostgreSQL database backend. The application follows a clean architecture approach with distinct layers for HTTP handling, business logic, and data storage.

## System Architecture

The application is structured as a web application with the following key components:

1. **Web Server**: Based on Go's standard `net/http` package with the Flow router
2. **Database**: PostgreSQL for data persistence
3. **Background Processing**: Custom task system for asynchronous operations
4. **Search Services**: Modular job search system with provider-based architecture
5. **Authentication**: OAuth2-based authentication with LinkedIn
6. **Voice Integration**: Phone call scheduling and management through Twilio

## Key Components

### HTTP Layer (`cmd/web/`)
- Application entry point: `cmd/web/main.go`
- Route definitions: `cmd/web/routes.go`
- HTTP handlers: `cmd/web/handlers.*.go`
- Middleware: `cmd/web/middleware.go`
- Error handling: `cmd/web/errors.go`

### Database Layer (`internal/database/`)
- Core database operations: `internal/database/db.go`
- User management: `internal/database/users.go`
- Task storage: `internal/database/task.go`
- Scheduling: `internal/database/scheduling.go`
- Skills management: `internal/database/skills.go`

### Task Processing (`internal/tasks/`)
- Task processor: `internal/tasks/processor.go`
- Task handlers: `internal/tasks/handlers/*`

### Search Services (`internal/services/search/`)
- Provider interfaces: `internal/services/search/provider.go`
- Job data structures: `internal/services/search/job.go`
- Provider implementations: `internal/services/search/providers/`

### Authentication (`internal/auth/`)
- OAuth2 implementation
- Session management
- User authentication

## Communication Flow

1. HTTP requests are received by the web server (`cmd/web/main.go`)
2. Requests are routed to appropriate handlers (`cmd/web/routes.go`)
3. Handlers process the request and interact with business logic
4. Business logic interacts with the database layer
5. Background tasks are scheduled and processed asynchronously
6. Responses are returned to clients

## Deployment Architecture

The application is designed to be deployed as a single binary with a PostgreSQL database. The deployment can be containerized using Docker with the included `.devcontainer` configuration.

## Development Workflow

The application includes:
- Makefile for common development tasks
- Taskfile.yml for task runner integration
- Migrations for database schema management
- Test utilities for integration testing

## File Structure

The main directories are:
- `cmd/web/`: Application entry point and HTTP handling
- `internal/`: Internal packages for business logic
- `assets/`: Static assets, templates, and email files
- `.project/`: Project documentation and specifications 