# Funcs Package

This package provides a collection of utility functions primarily used in html/templates and general-purpose operations throughout the Talent Rodeo application.

## Overview

The funcs package implements a comprehensive set of helper functions for common operations including time formatting, string manipulation, URL handling, and numeric operations. It's designed to be used primarily with Go templates but can be used anywhere in the application.

## Files

### funcs.go
Template and utility functions including:
- Time formatting and duration calculations
- String manipulation and formatting
- URL parameter handling
- Numeric operations and formatting
- Type conversion utilities

Key features:
- Template function map (`TemplateFuncs`)
- Internationalization support
- Safe HTML handling
- URL manipulation
- Type-safe conversions

## Key Features

1. Time Functions
   - Current time (`now`)
   - Time since/until calculations
   - Duration formatting
   - Custom time formatting
   - Approximate duration descriptions

2. String Functions
   - Case conversion (upper/lower)
   - Pluralization
   - URL-safe slugs
   - Safe HTML handling
   - Null/empty checks

3. URL Functions
   - Parameter setting
   - Parameter deletion
   - Safe URL manipulation
   - Query string handling

4. Numeric Functions
   - Increment/decrement
   - Formatted integers
   - Formatted floats
   - Multiplication helper
   - Type-safe conversions

5. Boolean Functions
   - Yes/No formatting
   - Boolean checks
   - Conditional operations

## Usage

Template functions:
```go
// In your template setup
tmpl := template.New("example").Funcs(funcs.TemplateFuncs)

// In your templates
{{ formatTime "2006-01-02" .Timestamp }}
{{ pluralize .Count "item" "items" }}
{{ slugify "Hello World" }}
```

Direct function usage:
```go
// Time formatting
duration := time.Hour * 25
fmt.Println(funcs.approxDuration(duration)) // "1 day"

// Slugify
slug := funcs.Slugify("Hello World!") // "hello-world"

// URL manipulation
newURL := funcs.urlSetParam(currentURL, "page", "2")
```

## Internationalization

The package uses `golang.org/x/text/message` for internationalized number formatting:
- Locale-aware number formatting
- Decimal point handling
- Thousands separators
- Currency formatting support

## Error Handling

Functions that can fail return errors along with their results:
```go
// Example with error handling
count, err := funcs.incr(someValue)
if err != nil {
    // Handle error
}
```

## Type Safety

The package includes safe type conversion utilities:
- String to int64 conversion
- Various integer type handling
- Protection against uint64 truncation
- Error reporting for invalid conversions

## Template Safety

The package implements several safety features:
- Safe HTML escaping
- URL parameter safety
- Null pointer handling
- Type conversion safety 