# Auth Package

This package implements authentication functionality for the Talent Rodeo application, with a focus on LinkedIn OAuth2 integration.

## Overview

The auth package provides a complete implementation of LinkedIn OAuth2 authentication, session management, and user profile retrieval. It is built on top of the `goth` authentication library and implements the OAuth2 protocol for authenticating users through LinkedIn.

## Files

### linkedin.go
LinkedIn OAuth2 implementation including:
- OAuth2 configuration and setup
- Provider implementation for LinkedIn authentication
- User profile fetching and parsing
- Token management
- Scope handling for LinkedIn API access

Key features:
- Implements the `goth.Provider` interface
- Handles OAuth2 flow with LinkedIn
- Retrieves user profile information
- Manages access tokens
- Configurable scopes for API access

### session.go
Session management functionality including:
- Session state management
- Token storage and retrieval
- Authorization flow handling
- Session serialization/deserialization

Key features:
- Session state tracking
- Token authorization
- JSON marshaling/unmarshaling
- URL management for auth flow

## Key Features

1. LinkedIn OAuth2 Integration
   - Complete OAuth2 flow implementation
   - User profile retrieval
   - Email verification
   - Profile picture handling
   - Locale support

2. Session Management
   - Secure session handling
   - Token lifecycle management
   - Session persistence
   - Error handling

3. Configuration Options
   - Customizable scopes
   - Configurable callback URLs
   - HTTP client customization
   - Provider name customization

## Usage

Initialize the LinkedIn provider:
```go
provider := auth.New(
    clientKey,
    clientSecret,
    callbackURL,
    "openid", "profile", "email"
)
```

Begin authentication:
```go
session, err := provider.BeginAuth(state)
if err != nil {
    // Handle error
}
authURL, _ := session.GetAuthURL()
// Redirect user to authURL
```

Fetch user profile:
```go
user, err := provider.FetchUser(session)
if err != nil {
    // Handle error
}
// Use user.Email, user.Name, etc.
```

## Security Considerations

1. Token Management
   - Access tokens are never stored permanently
   - Session data is encrypted
   - Tokens are validated before use

2. Scope Management
   - Minimal scopes by default
   - Additional scopes must be explicitly requested
   - Scope validation on token receipt

3. Error Handling
   - Comprehensive error checking
   - Safe error messages
   - Proper cleanup on failures 