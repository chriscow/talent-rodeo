# Cookies Package

This package provides secure cookie management functionality for the Talent Rodeo application, implementing various levels of cookie security including basic, signed, and encrypted cookies.

## Overview

The cookies package offers a comprehensive set of functions for handling HTTP cookies with different security levels, from basic cookie operations to encrypted cookie handling with AES-GCM encryption.

## Files

### cookies.go
Core cookie management functionality including:
- Basic cookie reading and writing
- HMAC-signed cookie operations
- AES-GCM encrypted cookie operations
- Base64 encoding/decoding
- Error handling

Key features:
- Cookie size validation
- Secure cryptographic operations
- Comprehensive error handling
- Multiple security levels

## Key Features

1. Basic Cookie Operations
   - Write and read cookies
   - Base64 encoding/decoding
   - Size validation (4KB limit)
   - Error handling

2. Signed Cookies
   - HMAC-SHA256 signatures
   - Tamper detection
   - Name binding
   - Signature verification

3. Encrypted Cookies
   - AES-GCM encryption
   - Secure nonce generation
   - Name binding
   - Authenticated encryption

## Usage

Basic cookie operations:
```go
// Writing a cookie
err := cookies.Write(w, http.Cookie{
    Name:  "session",
    Value: "123",
})

// Reading a cookie
value, err := cookies.Read(r, "session")
```

Signed cookies:
```go
// Writing a signed cookie
err := cookies.WriteSigned(w, http.Cookie{
    Name:  "session",
    Value: "123",
}, secretKey)

// Reading a signed cookie
value, err := cookies.ReadSigned(r, "session", secretKey)
```

Encrypted cookies:
```go
// Writing an encrypted cookie
err := cookies.WriteEncrypted(w, http.Cookie{
    Name:  "session",
    Value: "123",
}, secretKey)

// Reading an encrypted cookie
value, err := cookies.ReadEncrypted(r, "session", secretKey)
```

## Security Considerations

1. Cookie Signing
   - HMAC-SHA256 for signatures
   - Name binding prevents swapping
   - Signature verification required
   - Tamper detection built-in

2. Cookie Encryption
   - AES-GCM for authenticated encryption
   - Secure nonce generation
   - Name binding prevents swapping
   - Automatic integrity checking

3. General Security
   - Size limits enforced
   - Base64 encoding
   - Comprehensive error handling
   - No padding oracle vulnerabilities

## Error Handling

The package defines and handles several types of errors:
- `ErrValueTooLong`: Cookie value exceeds 4KB limit
- `ErrInvalidValue`: Invalid cookie format or tampering detected
- Cryptographic operation errors
- Encoding/decoding errors 