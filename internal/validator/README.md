# Validator Package

This package provides comprehensive validation functionality for the Talent Rodeo application, implementing type-safe validation with generic support.

## Overview

The validator package implements a flexible validation system with support for both general and field-specific validation, including type-safe generic validators and common validation helpers.

## Files

### validator.go
Core validation functionality:
- Validator structure
- Error management
- Field validation
- Error collection
- Validation checks

### helpers.go
Validation helper functions:
- String validation
- Email validation
- URL validation
- Generic comparisons
- Collection validation

## Key Features

1. Core Validation
   - Error collection
   - Field-specific errors
   - Validation checks
   - Error reporting
   - State management

2. Generic Validation
   - Type-safe comparisons
   - Collection validation
   - Range checking
   - Pattern matching
   - Value constraints

3. Helper Functions
   - String validation
   - Email validation
   - URL validation
   - Pattern matching
   - Collection operations

## Usage

Basic validation:
```go
v := validator.Validator{}

// Add general error
v.AddError("Something went wrong")

// Add field error
v.CheckField(user.Age >= 18, "age", "Must be 18 or older")

// Check for errors
if v.HasErrors() {
    // Handle errors
}
```

Helper functions:
```go
// String validation
isValid := validator.NotBlank(value)
isValid := validator.MinRunes(value, 3)
isValid := validator.MaxRunes(value, 100)

// Email validation
isValid := validator.IsEmail(email)

// URL validation
isValid := validator.IsURL(url)

// Generic validation
isValid := validator.Between(value, min, max)
isValid := validator.In(value, allowedValues...)
isValid := validator.NotIn(value, blockedValues...)
```

## Validation Types

1. String Validation
   - Non-empty checks
   - Length validation
   - Pattern matching
   - Email format
   - URL format

2. Generic Validation
   - Range checks
   - Value inclusion
   - Value exclusion
   - Duplicate detection
   - Collection validation

3. Field Validation
   - Field-specific errors
   - Custom messages
   - Multiple checks
   - Error aggregation
   - State tracking

## Error Handling

The package provides two types of errors:
1. General Errors
   - Application-level errors
   - Non-field-specific issues
   - Global validation failures

2. Field Errors
   - Field-specific issues
   - Validation failures
   - Format errors
   - Constraint violations

## Best Practices

1. Validation Usage
   - Check all required fields
   - Use appropriate helpers
   - Handle all error cases
   - Provide clear messages
   - Validate early

2. Error Management
   - Collect all errors
   - Use field-specific errors
   - Provide context
   - Clear error messages
   - Proper error types

3. Helper Functions
   - Use type-safe generics
   - Validate inputs
   - Handle edge cases
   - Reuse common patterns
   - Document constraints

## Regular Expressions

The package includes pre-compiled regular expressions:
```go
var (
    RgxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)
```

## Performance Considerations

1. Regular Expressions
   - Pre-compiled patterns
   - Efficient matching
   - Pattern caching
   - Length checks first
   - Early returns

2. Collection Operations
   - Efficient algorithms
   - Memory management
   - Early termination
   - Type safety
   - Cache usage 