# Held Response System - Manual Testing Guide

## Overview

This document provides a comprehensive manual testing guide for the held response system. The system allows administrators to review and approve email responses before they are sent to users, providing quality control and oversight for automated email communications.

## Prerequisites

### System Requirements
- Go 1.24+ installed
- Firebase emulator (optional, for local testing)
- Access to the talent-rodeo codebase
- Environment variables configured (see `.env` file)

### Initial Setup

1. **Start Firebase Emulator** (if available):
   ```bash
   task firebase
   ```

2. **Start the Web Server**:
   ```bash
   task live:server
   ```
   This will start the server on port 2304 at `http://localhost:2304`

3. **Build CLI Tools**:
   ```bash
   task build
   ```
   This creates the CLI binary at `/tmp/bin/cli`

## Configuration

### Enable Held Responses (Required for Testing)

The held response system is **disabled by default** for backward compatibility. To enable it for testing:

#### Option A: Temporary Code Change
Edit `/Users/chris/dev/talent-rodeo/cmd/web/main.go` around line 896:

```go
// Change from:
holdCriteria.Enabled = false

// To:
holdCriteria.Enabled = true
holdCriteria.HoldAll = true  // Hold everything for testing
```

#### Option B: Keyword-Based Holding
For more realistic testing, enable keyword-based holding:

```go
holdCriteria.Enabled = true
holdCriteria.HoldKeywords = []string{"test", "urgent", "important", "escalate"}
holdCriteria.DefaultExpirationHours = 24
```

After making changes, restart the server:
```bash
# Stop the server (Ctrl+C) and restart
task live:server
```

## Testing Workflow

### Phase 1: Basic CLI Testing

#### 1.1 Check System Status
```bash
/tmp/bin/cli held-responses stats
```
**Expected Output:**
```
Held Response Statistics:
=========================

Total Responses: 0

No held responses found.
```

#### 1.2 List Commands
```bash
# List all held responses
/tmp/bin/cli held-responses list

# List by specific status
/tmp/bin/cli held-responses list pending
/tmp/bin/cli held-responses list approved
/tmp/bin/cli held-responses list revision
/tmp/bin/cli held-responses list expired
```

**Expected Output:** "No [status] held responses found."

#### 1.3 Test Help Commands
```bash
# Main help
/tmp/bin/cli held-responses --help

# Subcommand help
/tmp/bin/cli held-responses list --help
/tmp/bin/cli held-responses approve --help
/tmp/bin/cli held-responses request-revision --help
```

### Phase 2: Web Interface Testing

#### 2.1 Access Admin Dashboard
1. Navigate to `http://localhost:2304/admin`
2. Look for "Held Response Management" section
3. Click "Manage Held Responses"

**Expected Result:** Admin dashboard loads with held response management option

#### 2.2 Test Admin Pages
1. **List Page**: `http://localhost:2304/admin/held-responses`
   - Should show "No pending held responses found."
   - Filter dropdown should be present

2. **Statistics Page**: `http://localhost:2304/admin/held-response-stats`
   - Should show "No held responses found."
   - Charts and metrics should be visible (empty)

### Phase 3: Create Test Data

Since the system starts empty, you need to create test data. Here are several approaches:

#### 3.1 Method 1: Programmatic Creation
Create a temporary test file:

```go
// test_data.go
package main

import (
    "context"
    "fmt"
    "time"
    "talent-rodeo/internal"
    "talent-rodeo/internal/inbox"
)

func main() {
    ctx := context.Background()
    fs, err := internal.NewFirestoreClient(ctx)
    if err != nil {
        panic(err)
    }
    defer fs.Close()

    // Create test held response
    testEmail := &inbox.Email{
        ID:      "test-001",
        From:    "test@example.com",
        Subject: "Test Urgent Issue",
        Body:    "This is a test urgent matter that needs attention",
        Date:    time.Now(),
    }

    testResponse := &inbox.EmailResponse{
        Subject: "Re: Test Urgent Issue",
        Body:    "Thank you for your inquiry. We will respond shortly.",
    }

    holdDecision := internal.HoldDecision{
        ShouldHold: true,
        Reason:     "Contains keyword: urgent",
        Criteria:   "keyword",
    }

    service := internal.NewHeldResponseService(fs, nil, nil, nil)
    heldResponse, err := service.CreateHeldResponse(ctx, testEmail, testResponse, "recipient@example.com", holdDecision)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Created test held response: %s\n", heldResponse.ID)
}
```

Run with:
```bash
go run test_data.go
```

#### 3.2 Method 2: Database Direct Insert
Use the Firestore console or CLI to insert test data directly into the `held_responses` collection.

#### 3.3 Method 3: Email Trigger (If Email Processing is Set Up)
If you have email processing configured, send an email containing the keywords "urgent", "test", or "important" to trigger the holding mechanism.

### Phase 4: Test with Data

Once you have test data, repeat the CLI and web interface tests:

#### 4.1 Verify Data Creation
```bash
/tmp/bin/cli held-responses list
/tmp/bin/cli held-responses stats
```

**Expected Output:**
```
Found 1 all held response(s):

ID: test-001
  Status: pending
  From: test@example.com
  To: recipient@example.com
  Subject: Test Urgent Issue
  Created: 2024-01-15 10:30:00
  Expires: 2024-01-16 10:30:00
  Hold Reason: Contains keyword: urgent
```

#### 4.2 Test Detail View
```bash
/tmp/bin/cli held-responses detail test-001
```

**Expected Output:** Full details of the held response including original email and generated response.

### Phase 5: Test Approval Workflow

#### 5.1 CLI Approval
```bash
/tmp/bin/cli held-responses approve test-001
```

**Expected Output:**
```
Successfully approved and sent response test-001
```

#### 5.2 Web Interface Approval
1. Go to `http://localhost:2304/admin/held-responses/test-001`
2. Review the response details
3. Click "✓ Approve & Send Response"
4. Verify success message appears

#### 5.3 Verify Approval
```bash
/tmp/bin/cli held-responses list approved
```

**Expected Output:** The response should now appear in the approved list.

### Phase 6: Test Revision Workflow

Create another test response for revision testing:

#### 6.1 CLI Revision Request
```bash
/tmp/bin/cli held-responses request-revision test-002 "Please make this more professional and add contact information"
```

**Expected Output:**
```
Successfully requested revision for response test-002
```

#### 6.2 Web Interface Revision
1. Go to held response detail page
2. Click "Request Revision"
3. Enter feedback: "Please make the response more personalized and include specific next steps"
4. Click "Request Revision"
5. Verify success message

#### 6.3 Verify Revision Status
```bash
/tmp/bin/cli held-responses list revision
```

**Expected Output:** The response should appear in the revision list with feedback.

### Phase 7: Test Expiration

#### 7.1 Create Expired Response
Create a test response with expiration in the past, or wait for natural expiration.

#### 7.2 Mark Expired
```bash
/tmp/bin/cli held-responses mark-expired
```

**Expected Output:**
```
Successfully marked 1 response(s) as expired
```

#### 7.3 Check Expired List
```bash
/tmp/bin/cli held-responses list expired
```

### Phase 8: Error Handling Tests

#### 8.1 Invalid Response ID
```bash
/tmp/bin/cli held-responses approve invalid-id
```

**Expected Output:** Error message about response not found.

#### 8.2 Empty Revision Feedback
```bash
/tmp/bin/cli held-responses request-revision test-001 ""
```

**Expected Output:** Error message about empty feedback.

#### 8.3 Approve Already Approved
Try to approve a response that's already approved.

**Expected Output:** Error message about invalid status.

### Phase 9: Performance and Load Testing

#### 9.1 Multiple Responses
Create 10-20 test responses and test:
- List performance
- Admin interface load times
- Filtering functionality

#### 9.2 Large Response Bodies
Test with responses containing large amounts of text to ensure UI handles it gracefully.

## Expected Behaviors

### ✅ Success Indicators

1. **CLI Commands**
   - All commands execute without errors
   - Output is properly formatted
   - Status changes are reflected immediately

2. **Web Interface**
   - Pages load without errors
   - Forms submit successfully
   - Status changes are reflected in UI
   - Filtering works correctly

3. **Workflow**
   - Approval sends email and updates status
   - Revision requests update status and store feedback
   - Expiration marks old responses correctly

### ❌ Error Cases to Verify

1. **Invalid Operations**
   - Approving non-existent responses fails gracefully
   - Empty revision feedback is rejected
   - Already processed responses cannot be reprocessed

2. **System Errors**
   - Database connection errors are handled
   - Missing templates show appropriate errors
   - Authentication failures are handled

## Troubleshooting

### Common Issues and Solutions

1. **"Held response service not configured"**
   - **Cause:** Service not initialized in main.go
   - **Solution:** Verify service creation and assignment to app struct

2. **Database connection errors**
   - **Cause:** Firebase emulator not running or credentials missing
   - **Solution:** Start emulator or check environment variables

3. **Template not found errors**
   - **Cause:** Admin templates missing or path incorrect
   - **Solution:** Verify templates exist in `assets/templates/pages/admin/`

4. **CLI command not found**
   - **Cause:** Binary not built or not in PATH
   - **Solution:** Run `task build` and use full path `/tmp/bin/cli`

5. **No responses being held**
   - **Cause:** Holding is disabled or criteria not met
   - **Solution:** Enable holding in configuration and check criteria

### Debug Tips

1. **Check Server Logs**
   ```bash
   # Look for held response related logs
   grep -i "held response" server.log
   ```

2. **Verify Database State**
   - Check Firestore console for `held_responses` collection
   - Verify document structure matches expected schema

3. **Test Configuration**
   - Print hold criteria configuration on startup
   - Verify email processing pipeline includes held response check

## Test Data Cleanup

After testing, clean up test data:

```bash
# Mark all test responses as expired
/tmp/bin/cli held-responses mark-expired

# Or delete directly from database
# (Use Firestore console or CLI)
```

## Conclusion

This testing guide covers all major functionality of the held response system. Start with CLI commands for basic verification, then move to the web interface once you have test data. The system provides comprehensive administrative control over email responses with proper error handling and user feedback.

For production deployment, remember to:
1. Configure appropriate hold criteria
2. Set up proper authentication for admin access
3. Configure email delivery settings
4. Set up monitoring and alerting
5. Plan for data retention and cleanup

## Next Steps

After manual testing, consider:
1. Automated integration tests
2. Performance benchmarking
3. Security testing
4. User acceptance testing
5. Production rollout planning