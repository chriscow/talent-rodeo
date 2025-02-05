# Utils Package

This package provides a collection of utility functions used throughout the Talent Rodeo application, focusing on common operations and helper functions.

## Overview

The utils package implements various utility functions for handling prompts, time formatting, file operations, cryptographic functions, markdown processing, and slice manipulation.

## Files

### prompts.go
Prompt template functionality:
- YAML header parsing
- Template management
- Hash verification
- File operations
- Template execution

### relative-time.go
Time formatting utilities:
- Relative time parsing
- Human-readable durations
- Time calculations
- Age calculations
- Time formatting

### markdown.go
Markdown processing:
- HTML conversion
- Parser configuration
- Renderer options
- Extension support
- Link handling

### crypto.go
Cryptographic utilities:
- SHA256 hashing
- Hash encoding
- Error handling
- Data validation
- Security functions

### files.go
File operation utilities:
- File copying
- Backup creation
- Path handling
- Error management
- File system operations

### slices.go
Slice manipulation:
- Generic slice shuffling
- Random operations
- Type-safe functions
- Collection utilities
- Order manipulation

## Key Features

1. Prompt Management
   - YAML header support
   - Template validation
   - Hash verification
   - File persistence
   - Error handling

2. Time Handling
   - Relative time parsing
   - Human-readable formats
   - Duration calculations
   - Time comparisons
   - Format conversions

3. File Operations
   - Safe file copying
   - Backup management
   - Path manipulation
   - Error handling
   - System integration

## Usage

Prompt handling:
```go
prompt, err := utils.CreateTemplate("templates/example.txt")
result, err := prompt.Execute(data)
```

Time formatting:
```go
relativeTime := utils.RelativeTime(timestamp, time.Now())
parsedTime, err := utils.TimeFromRelative("2 weeks ago", time.Now())
```

File operations:
```go
err := utils.CopyFile(src, dst)
err := utils.CreateBackup(basePath, filename)
```

Markdown processing:
```go
html := utils.markdownToHTML(markdownContent)
```

Cryptographic operations:
```go
hash, err := utils.SHA256Hash(data)
```

Slice manipulation:
```go
items := []string{"a", "b", "c"}
utils.ShuffleSlice(items)
```

## Template System

1. Header Format
   ```yaml
   name: template_name
   version: 1.0.0
   format: text
   sha256: hash
   ```

2. Template Usage
   ```go
   prompt, err := utils.CreateTemplate(path)
   if err != nil {
       // Handle error
   }
   result, err := prompt.Execute(data)
   ```

## Time Formatting

The package supports various time formats:
- Relative time (e.g., "2 weeks ago")
- Duration calculations
- Human-readable output
- Timezone handling
- Age calculations

## File Operations

1. File Copying
   - Safe copy operations
   - Error handling
   - Path validation
   - Permission handling
   - Resource cleanup

2. Backup Management
   - Incremental backups
   - Version tracking
   - Path handling
   - Error recovery
   - File verification

## Best Practices

1. Template Usage
   - Validate templates
   - Handle errors
   - Check hashes
   - Clean up resources
   - Log operations

2. Time Handling
   - Use appropriate formats
   - Handle timezones
   - Validate input
   - Consider edge cases
   - Format consistently

3. File Operations
   - Check permissions
   - Handle errors
   - Validate paths
   - Clean up resources
   - Log operations 