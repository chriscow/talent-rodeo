# UI Integration Testing with Playwright-Go

This directory contains integration tests for the Talent Rodeo UI using the [Playwright-Go](https://github.com/playwright-community/playwright-go) library.

## Prerequisites

1. Install Playwright browsers:
   ```
   go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps
   ```

## Test Configuration

The integration tests are designed to work with a running instance of the application with specific configurations:

1. **Testing Mode:** Start the server with the `-testing` flag to enable automated authentication
   ```
   go run ./cmd/web -testing
   ```

2. **Environment Variables:** The tests use `godotenv` to load environment variables, similar to the application itself

## Authentication

When running in testing mode (`-testing` flag), authentication is handled automatically:

- The `testModeAuth` middleware in `middleware.go` intercepts requests and automatically authenticates them
- This bypasses the OAuth flow with LinkedIn, making UI tests simpler and more reliable
- A test user is created and authenticated for all requests

## Running the Tests

To run the integration tests:

1. Start the application in testing mode:
   ```
   go run ./cmd/web -testing
   ```

2. In a separate terminal, run the integration tests:
   ```
   go test ./internal/integration -v
   ```

## Writing Tests

When writing integration tests:

1. Use the `godotenv` package in an `init()` function to load environment variables (similar to `handlers_test.go`)
2. Create new tests that:
   - Launch a new browser with Playwright
   - Visit pages in the application
   - Perform actions and verify expected behaviors

3. Remember that with the `-testing` flag, you'll be automatically authenticated and won't need to handle login flows

## Available Pages

The application has the following main pages:

- Home: `/`
- Profile pages:
  - Resume Upload: `/profile/resume/upload`
  - Schedule Phone Call: `/profile/schedule`
  - Dashboard: `/dashboard`

## Best Practices

1. Use descriptive test names that clearly indicate what is being tested
2. Include proper cleanup in your tests
3. Use Playwright's waiting mechanisms instead of arbitrary sleep calls
4. Structure tests to be independent of each other
5. Keep tests focused on UI behavior, not internal implementation details