# Technical Documentation Index

This directory contains technical documentation for the Talent Rodeo project. These documents provide an overview of the system architecture, components, and implementation details.

## Document List

1. [System Overview](01_system_overview.md)
   - High-level system architecture
   - Key components
   - Communication flow
   - Deployment architecture
   - Development workflow
   - File structure

2. [Task Processing System](02_task_system.md)
   - Background task execution framework
   - Task data structure
   - Task processor
   - Task handlers (search, messages, phone calls)
   - Database integration
   - Task lifecycle
   - Best practices

3. [Database Architecture](03_database_architecture.md)
   - PostgreSQL with sqlx
   - Core database interface
   - Transaction support
   - Key database files
   - Transaction management
   - Migration management
   - Testing utilities
   - Best practices

4. [Search Architecture](04_search_architecture.md)
   - Provider-based design pattern
   - Job provider interface
   - Provider registry
   - Job data structures
   - Pagination support
   - Provider implementations
   - Search flow
   - Best practices

5. [HTTP Layer Architecture](05_http_layer.md)
   - Handler-based architecture
   - Middleware
   - Router configuration
   - Handler categories
   - Error handling
   - Context management
   - Request processing flow
   - Tests

## Purpose

These documents are designed to provide quick reference and refreshers on the architectural decisions and implementation details of the Talent Rodeo platform. Each document includes file paths to the relevant source files to enable quick navigation to the implementation.

## Document Structure

Each document follows a consistent structure:
1. Overview of the component
2. Core components and interfaces
3. Key implementation files
4. Component workflow
5. Best practices
6. File references

## Maintenance

These documents should be updated as the codebase evolves. When significant architectural changes are made, the corresponding documentation should be updated to reflect the current state of the system. 