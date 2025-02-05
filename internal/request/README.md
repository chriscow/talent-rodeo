# Request Package

This package provides robust request parsing and decoding functionality for the Talent Rodeo application, handling both form data and JSON requests.

## Overview

The request package implements comprehensive request decoding functionality, supporting form data, URL query parameters, and JSON payloads. It includes robust error handling and validation to ensure request data is properly formatted and within size limits.

## Files

### forms.go
Form data handling functionality including:
- Form data decoding
- POST form processing
- Query string parsing
- URL value decoding
- Error handling

Key features:
- Form data validation
- Struct tag mapping
- Multiple form types
- Flexible decoding
- Error wrapping

### json.go
JSON request handling functionality including:
- JSON payload decoding
- Size limit enforcement
- Strict mode support
- Detailed error handling
- Type validation

Key features:
- Request body limits
- Unknown field handling
- Type validation
- Error classification
- Security checks

## Key Features

1. Form Processing
   - Standard form decoding
   - POST form handling
   - Query string parsing
   - URL value mapping
   - Struct tag support

2. JSON Processing
   - Request body limits (1MB)
   - Strict field validation
   - Type checking
   - Single value enforcement
   - Comprehensive error handling

3. Error Handling
   - Detailed error messages
   - Type-specific errors
   - Security validations
   - Size limit errors
   - Syntax checking

## Usage

Form decoding:
```go
// Decode standard form
var form struct {
    Name  string `form:"name"`
    Email string `form:"email"`
}
err := request.DecodeForm(r, &form)

// Decode query string
var query struct {
    Page   int    `form:"page"`
    Search string `form:"q"`
}
err := request.DecodeQueryString(r, &query)
```

JSON decoding:
```go
// Standard JSON decoding
var data struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}
err := request.DecodeJSON(w, r, &data)

// Strict JSON decoding (no unknown fields)
err := request.DecodeJSONStrict(w, r, &data)
```

## Error Handling

The package provides detailed error handling for various scenarios:

1. JSON Errors
   - Syntax errors
   - Type mismatches
   - Unknown fields
   - Size limits
   - Empty bodies

2. Form Errors
   - Parse errors
   - Decode errors
   - Type conversion
   - Missing fields
   - Invalid values

## Security Features

1. Request Limits
   - 1MB JSON body limit
   - Single JSON value enforcement
   - Form size limits
   - Memory usage controls

2. Validation
   - Type checking
   - Field validation
   - Size verification
   - Format checking
   - Structure validation

## Best Practices

1. Use appropriate decoder:
   - `DecodeForm` for standard forms
   - `DecodePostForm` for POST data
   - `DecodeQueryString` for URL queries
   - `DecodeJSON` for JSON data
   - `DecodeJSONStrict` for strict JSON validation

2. Handle errors appropriately:
   - Check for specific error types
   - Provide meaningful messages
   - Log relevant details
   - Return appropriate status codes 