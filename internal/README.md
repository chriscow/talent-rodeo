# Internal Packages

This directory contains the internal packages of the Talent Rodeo application. These packages are not meant to be imported by external applications.

## Package Index

### auth/
Authentication and authorization package handling user authentication through LinkedIn OAuth2, session management, and access control. Features include complete OAuth2 flow implementation, secure session handling, and user profile retrieval.

### cookies/
Cookie management package providing secure cookie operations with multiple security levels including basic, signed, and encrypted cookies. Implements HMAC-SHA256 signatures and AES-GCM encryption with comprehensive error handling.

### database/
Database interface package providing robust PostgreSQL operations through `sqlx`, with features including transaction management, connection pooling, migration support, and comprehensive testing utilities.

### funcs/
Utility functions package containing html/template helpers and general-purpose operations including time formatting, string manipulation, URL handling, and numeric operations. Provides internationalization support and type-safe conversions.

### models/
Core domain models package defining data structures and business logic objects, with built-in validation, database mapping, and JSON serialization. Includes task management and message handling capabilities.

### onboarding/
User onboarding package managing the progression through various onboarding steps, including state management, timing tracking, and workflow validation from initial signup through job search initiation.

### request/
HTTP request handling package providing robust request parsing and decoding for form data, URL query parameters, and JSON payloads. Includes comprehensive validation and size limit enforcement.

### response/
HTTP response handling package managing html/template rendering with layouts, JSON responses, and response metrics collection. Features include template caching, performance monitoring, and comprehensive error handling.

### rodeo/
Core business logic package implementing automated recruiting functionality, including conversation management, candidate assessment, and workflow orchestration using AI-powered interactions.

### scrapingdog/
Web scraping package specializing in LinkedIn job listing retrieval and analysis, with features for job matching, requirement extraction, and search optimization through the ScrapingDog API.

### smtp/
Email handling package providing robust email functionality with template support and retry capabilities. Implements both HTML and plain text emails with configurable timeouts and connection pooling.

### tasks/
Background task processing system handling asynchronous operations, with specialized support for phone call scheduling and execution through Twilio integration. Features automatic retries and transaction safety.

### utils/
General utility package providing functions for prompt templates, time formatting, file operations, cryptographic functions, markdown processing, and slice manipulation with comprehensive error handling.

### validator/
Input validation package implementing type-safe validation with generic support, field-specific validation, and common validation helpers. Includes comprehensive error collection and reporting capabilities.

### vapi/
Voice API integration package managing voice synthesis, transcription, and intelligent conversation capabilities. Integrates with ElevenLabs for voice synthesis and various LLM providers for conversation intelligence.

### version/
Version management package handling application versioning and compatibility.

### voice/
Voice interaction package handling call scheduling and management.

## Package Organization

The internal packages follow these organizational principles:

1. Each package has a single, well-defined responsibility
2. Packages are designed to be loosely coupled but highly cohesive
3. Circular dependencies between packages are strictly avoided
4. Each package contains its own tests and documentation
5. Common utilities are extracted into appropriate utility packages

## Documentation

Each package contains its own README.md with detailed information about:
- Package purpose and responsibility
- File descriptions
- Key features and functionality
- Usage examples
- Testing approach
- Configuration options
- Best practices
- Performance considerations 