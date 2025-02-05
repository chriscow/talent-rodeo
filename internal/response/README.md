# Response Package

This package provides comprehensive HTTP response handling functionality for the Talent Rodeo application, supporting template rendering, JSON responses, and response metrics.

## Overview

The response package implements a flexible and robust system for handling HTTP responses, including HTML template rendering with layouts and partials, JSON response formatting, and response metrics collection.

## Files

### templates.go
Template rendering functionality including:
- Page rendering with layouts
- Partial template rendering
- Template caching
- Error handling
- Header management

Key features:
- Layout system
- Partial templates
- Custom functions
- Buffer management
- Header control

### json.go
JSON response handling including:
- JSON marshaling
- Pretty printing
- Header management
- Content-Type setting
- Error handling

Key features:
- Indented output
- Header control
- Status code management
- Error wrapping
- Content-Type handling

### metrics.go
Response metrics collection including:
- Status code tracking
- Byte counting
- Header tracking
- Response wrapping
- Metrics aggregation

Key features:
- Response wrapping
- Metric collection
- Header tracking
- Status monitoring
- Byte counting

## Key Features

1. Template Rendering
   - Full page layouts
   - Partial templates
   - Custom functions
   - Error handling
   - Header management

2. JSON Responses
   - Pretty printing
   - Content-Type handling
   - Status codes
   - Custom headers
   - Error handling

3. Metrics Collection
   - Status code tracking
   - Response size monitoring
   - Header tracking
   - Performance metrics
   - Response wrapping

## Usage

Template rendering:
```go
// Render a full page
err := response.Page(w, http.StatusOK, data, "pages/home.html")

// Render a partial
err := response.Partial(w, http.StatusOK, data, "partials/nav.html")

// Render with options
err := response.Render(w, http.StatusOK, data, "pages/profile.html", &response.RenderOptions{
    Layout: "base",
    Headers: headers,
})
```

JSON responses:
```go
// Basic JSON response
err := response.JSON(w, http.StatusOK, data)

// JSON with headers
err := response.JSONWithHeaders(w, http.StatusOK, data, headers)
```

Metrics collection:
```go
// Create a metrics wrapper
metricsWriter := response.NewMetricsResponseWriter(w)

// Use the wrapper
handler.ServeHTTP(metricsWriter, r)

// Access metrics
statusCode := metricsWriter.StatusCode
bytesWritten := metricsWriter.BytesCount
```

## Template System

1. Layout Structure
   - Base templates
   - Content blocks
   - Partial includes
   - Custom functions
   - Error handling

2. Partial Templates
   - Reusable components
   - Shared layouts
   - Dynamic content
   - Nested templates
   - Context passing

## Error Handling

The package provides comprehensive error handling:
- Template parsing errors
- Execution errors
- JSON marshaling errors
- Write errors
- Header errors

## Performance Features

1. Template Optimization
   - Template caching
   - Buffer management
   - Error recovery
   - Memory efficiency
   - Stream processing

2. Metrics Collection
   - Low overhead
   - Real-time tracking
   - Accurate counting
   - Header monitoring
   - Status tracking

## Best Practices

1. Template Usage
   - Use layouts for consistency
   - Cache templates when possible
   - Handle errors appropriately
   - Use partial templates for components
   - Implement proper error pages

2. JSON Responses
   - Set appropriate status codes
   - Include error details when needed
   - Use consistent response formats
   - Handle null values appropriately
   - Validate response data

3. Metrics Collection
   - Monitor response sizes
   - Track status codes
   - Analyze performance patterns
   - Log unusual patterns
   - Set up alerts for issues 