# SMTP Package

This package provides email functionality for the Talent Rodeo application, implementing a robust mailer with template support and retry capabilities.

## Overview

The smtp package implements a reliable email sending system with support for both HTML and plain text emails, templated content, and automatic retries. It uses the `go-mail` package for SMTP functionality.

## Files

### mailer.go
Email sending functionality including:
- SMTP client configuration
- Template processing
- Email composition
- Retry logic
- Error handling

Key features:
- HTML and plain text support
- Template-based emails
- Configurable timeouts
- Automatic retries
- SMTP authentication

## Key Features

1. Email Configuration
   - SMTP server settings
   - Authentication
   - Timeout configuration
   - From address management
   - Connection pooling

2. Template Support
   - HTML templates
   - Plain text templates
   - Subject templating
   - Dynamic content
   - Template functions

3. Reliability Features
   - Automatic retries (3 attempts)
   - Error handling
   - Connection management
   - Timeout handling
   - Graceful failures

## Usage

Initialize mailer:
```go
mailer, err := smtp.NewMailer(
    "smtp.example.com",
    587,
    "username",
    "password",
    "from@example.com",
)
```

Send templated email:
```go
err := mailer.Send(
    "recipient@example.com",
    data,
    "templates/welcome.html",
)
```

## Template Structure

Templates should include:
1. Subject Template
   - Named "subject"
   - Single line
   - Supports variables

2. Plain Text Body
   - Named "plainBody"
   - Text-only format
   - Full message content

3. HTML Body (Optional)
   - Named "htmlBody"
   - Rich HTML format
   - Alternative content

## Configuration

1. SMTP Settings
   - Host configuration
   - Port selection
   - Authentication details
   - Timeout settings (default: 10s)
   - From address

2. Retry Settings
   - 3 attempts maximum
   - 2-second delay between attempts
   - Automatic fallback
   - Error preservation
   - Success tracking

## Error Handling

The package provides comprehensive error handling:
- Connection errors
- Authentication failures
- Template errors
- Send failures
- Timeout issues

## Best Practices

1. Template Usage
   - Always provide plain text version
   - Test templates before deployment
   - Use consistent naming
   - Include all required sections
   - Validate dynamic content

2. Configuration
   - Use environment variables
   - Validate SMTP settings
   - Test connection at startup
   - Monitor timeouts
   - Log failures

3. Error Management
   - Handle all error cases
   - Log retry attempts
   - Monitor failure rates
   - Implement fallbacks
   - Alert on persistent failures 