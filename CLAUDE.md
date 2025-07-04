# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Essential Commands

**Development Server:**
```bash
task live:server        # Live reload development server on port 2304
task run               # Build and run production server
task build             # Build all binaries (web, cli, mcp)
```

**Testing:**
```bash
task test              # Run all Go tests with race detection
task test:ui           # Run Playwright UI tests (optional test name)
task audit             # Quality control: fmt, vet, staticcheck, govulncheck
task test-cover        # Run tests with coverage report
```

If you run `go test` directly, you need to set the `FIRESTORE_EMULATOR_HOST` environment variable to `localhost:8200` and load the `.env` file `set -a; source .env; set +a`.

**Database:**
```bash
task firebase          # Start Firebase emulators for local development
task migrations-up     # Apply database migrations
task migrations-new    # Create new migration: task migrations-new -- migration_name
```

**Github:**
The github command line tools are available in the `gh` command.
```bash
gh auth login
gh repo clone https://github.com/chriscow/talent-rodeo
gh pr create --title "Your PR title" --body "Your PR body"
```

## Architecture Overview

This is a **Go web application** using:
- **Firestore** as primary database (NoSQL document store)
- **Server-side rendered HTML** templates with embedded static assets
- **LinkedIn OAuth2** for authentication
- **SCS sessions** with Firestore storage
- **Custom task processor** for background jobs
- **Pusher** for real-time WebSocket communication

## Project Structure

```
cmd/
├── web/        # Main web application (handlers, middleware, routes)
├── cli/        # Command-line tools
└── mcp/        # Model Context Protocol server

internal/       # Core business logic packages
├── auth/       # Authentication (LinkedIn OAuth2, sessions)
├── firestore.go # Firestore database client
├── search/     # Job search pipeline with providers
├── tasks/      # Background task processing system
├── transcript/ # Voice call transcript processing
└── ...

assets/
├── templates/  # HTML templates (base.html, pages/, partials/)
├── static/     # CSS, JS, images (embedded in binary)
├── migrations/ # Database schema migrations
└── efs.go     # Embedded filesystem for assets
```

## Key Development Patterns

**Handler Structure:**
```go
func (app *application) handlerName(w http.ResponseWriter, r *http.Request) {
    // All handlers are methods on *application struct
    // Access dependencies via app.db, app.logger, etc.
}
```

**Database Operations:**
- Primary: Firestore via `app.firestoreClient` or `internal/firestore.go`
- Testing: Firestore emulator (auto-started in tests)
- Migrations: PostgreSQL-compatible but currently using Firestore

**Template Rendering:**
```go
data := app.newTemplateData()
data["key"] = value
err := response.Page(w, http.StatusOK, data, "pages/template.html")
```

**Background Tasks:**
```go
// Queue background task
task := &internal.Task{
    Type: "webhook_call",
    Data: map[string]interface{}{"url": url},
}
err := app.taskProcessor.QueueTask(r.Context(), task)
```

## Environment Setup

**Required Environment Variables:**
```bash
# Core
BASE_URL=http://localhost:2304
FIREBASE_PROJECT_ID=your-project
FIREBASE_CREDENTIALS=base64-encoded-service-account-json

# Authentication
LINKEDIN_AUTH_CLIENT_ID=
LINKEDIN_AUTH_CLIENT_SECRET=
SESSION_SECRET=32-char-random-string

# APIs
OPENAI_API_KEY=
GEMINI_API_KEY=
PINECONE_API_KEY=
SCRAPINGDOG_API_KEY=
VAPI_API_KEY=

# Infrastructure
PUSHER_APP_ID=
PUSHER_KEY=
PUSHER_SECRET=
PUSHER_CLUSTER=us2
```

**Environment Loading:**
- Uses `godotenv` to load `.env` files
- Supports multiple env files: `.env`, `.prod.env`, `.dev.env`
- Environment validation on startup with detailed error messages

## Testing Strategy

**Test Types:**
1. **Unit Tests**: Fast, use Firestore emulator, isolated per package. Use only the standard library, not third party libraries.
2. **Integration Tests**: Full HTTP stack with `internal/integration/test_framework.go`
3. **UI Tests**: Playwright browser automation in `tools/` directory

**Test Environment:**
- Firestore emulator automatically started via `TestMain()`
- Test users auto-created/cleaned up per test suite (see `internal/test_helpers.go`)
- Screenshots captured on UI test failures
- Race detection enabled by default

**Running Specific Tests:**
```bash
set -a; source .env; set +a; FIRESTORE_EMULATOR_HOST=localhost:8200 go test ./internal/specific_package    # Single package
set -a; source .env; set +a; FIRESTORE_EMULATOR_HOST=localhost:8200 task test:ui -- homepage              # Specific UI test
set -a; source .env; set +a; FIRESTORE_EMULATOR_HOST=localhost:8200 go test -run TestSpecificFunction     # Single test function
```

## Authentication Flow

**LinkedIn OAuth2:**
- Custom provider in `internal/auth/linkedin.go`
- Sessions stored in Firestore via SCS
- CSRF protection with `justinas/nosurf`
- Testing mode bypass: set `--testing` flag

**Middleware Stack (applied in order):**
1. `recoverPanic` - Panic recovery
2. `sentryHandler` - Error tracking
3. `securityHeaders` - Security headers
4. `logAccess` - Request logging
5. `sessionManager.LoadAndSave` - Session handling
6. `authenticate` - User authentication
7. `setSentryUser` - Error context

## Key Architectural Decisions

**Why Firestore over PostgreSQL:**
- NoSQL flexibility for varying job data structures
- Built-in real-time subscriptions
- Serverless scaling
- Strong consistency with offline support

**Background Task System:**
- Custom implementation over external queue systems
- Firestore-based task storage with status tracking
- Webhook-based scheduling for external service integration
- Graceful shutdown handling for long-running tasks

**Template-based Rendering:**
- Server-side rendering for SEO and performance
- Embedded static assets for deployment simplicity
- Custom template functions in `internal/funcs/`
- Progressive enhancement with minimal JavaScript

## Common Debugging

**Sentry Error Logging:**
- Sentry is configured in `cmd/web/main.go`
- `internal/sentry.go` are Sentry helper functions
- Please ensure all errors are logged to Sentry.


**Firestore Connection Issues:**
- Check `FIREBASE_PROJECT_ID` and `FIREBASE_CREDENTIALS`
- Verify service account has Firestore permissions
- When testing locally, add `FIRESTORE_EMULATOR_HOST=localhost:8200` to your environment variables.

**Authentication Problems:**
- Verify LinkedIn app credentials and redirect URLs
- Check session storage (Firestore-based)
- Use `--testing` flag to bypass auth in development

**Task Processing Issues:**
- Check task status in Firestore `tasks` collection
- Review webhook endpoint availability and authentication
- Monitor background goroutines with graceful shutdown

## Migration Guide

**Adding New Routes:**
1. Add handler method to `cmd/web/handlers.*.go`
2. Register route in `cmd/web/routes.go`
3. Create template in `assets/templates/pages/`

**Adding Background Tasks:**
1. Define task type in `internal/task.go`
2. Add handler in `internal/tasks/handlers/`
3. Register handler in task processor

**Database Schema Changes:**
1. Create migration: `task migrations-new -- description`
2. Implement up/down SQL in `assets/migrations/`
3. Test with `task migrations-up`

## Production Deployment

**Build Process:**
- Multi-stage Docker build with Go 1.24
- Assets embedded in binary via `assets/efs.go`
- Single binary deployment to Fly.io

**Health Checks:**
- `/health` endpoint for load balancer checks
- Graceful shutdown with configurable timeout
- Database connection validation on startup

**Available Tools:**
- Github CLI: `gh`
- Fly.io CLI: `fly`

**CRITICAL:**
- You are not to put in placeholders for any code unless you explicitly ask me first or if I ask you to.
- When you put in placeholders, you MUST use the key "TODO:" and explain why you are doing it and what the placeholder is for.
- Before you claim that something is fully implemented you must have resolved all TODOs or gotten explicit approval from me.