# F006: User Notification System

## Feature Overview
The User Notification System provides real-time notifications and status updates to both users and administrators. 
It leverages Firestore's real-time capabilities to deliver immediate feedback about system events, errors, 
and actionable items. This system replaces the current error field in the user state with a more comprehensive 
notification structure that includes actionable items and diagnostic information.

## Implementation Status
- Notification Data Model: **Not Started**
- Real-time Updates: **Not Started**
- Admin Dashboard Integration: **Not Started**
- User Interface Integration: **Not Started**

## Detailed Specifications

### SF006.1: Notification Data Model

#### Description
Implements a subcollection-based notification system in Firestore that stores user-specific notifications
with rich content including HTML, links, and severity levels.

#### Implementation Details
- Notifications stored as subcollection `notifications` under each user document
- Each notification includes:
  - ID: Unique identifier
  - CreatedAt: Timestamp
  - UpdatedAt: Timestamp
  - Type: enum (error, warning, info, success)
  - Title: Brief description
  - Message: Detailed message (can include safe HTML)
  - ActionURL: Optional URL for user action
  - ActionText: Text for the action button
  - Severity: enum (critical, high, medium, low)
  - Status: enum (unread, read, actioned)
  - Source: Component that generated the notification
  - Metadata: JSON object with additional context
  - AdminOnly: Boolean flag for admin-visible notifications
  - ExpiresAt: Optional expiration timestamp

#### Current Implementation
To be implemented in:
- `/internal/notifications/types.go`: Data structures and enums
- `/internal/notifications/notifications.go`: CRUD operations
- `/internal/notifications/realtime.go`: Real-time update handlers

### SF006.2: Real-time Updates Implementation

#### Description
Implements Firestore's real-time update capabilities to push notifications instantly to both user and admin interfaces.

#### Implementation Details
- Uses Firestore's snapshot listeners for real-time updates
- Implements server-side notification aggregation
- Provides WebSocket endpoint for client real-time updates
- Handles connection management and reconnection logic

#### Implementation Plan
```go
// Example implementation structure
type NotificationListener struct {
    ctx        context.Context
    cancel     context.CancelFunc
    userID     string
    wsConn     *websocket.Conn
    snapshots  []*firestore.DocumentSnapshot
}

func NewNotificationListener(userID string) *NotificationListener {
    // Initialize real-time listener
}

func (nl *NotificationListener) Start() error {
    // Start listening for Firestore changes
}
```

### SF006.3: Admin Dashboard Integration

#### Description
Provides administrators with a comprehensive view of system notifications across all users,
with filtering, search, and diagnostic capabilities.

#### Implementation Details
- Admin dashboard with real-time notification feed
- Filtering by severity, type, and status
- Search functionality across notification content
- Bulk actions for notification management
- Analytics on notification patterns and trends

### SF006.4: User Interface Integration

#### Description
Integrates notifications into the user interface with a notification center and real-time toast notifications.

#### Implementation Details
- Notification bell icon with unread count
- Toast notifications for new items
- Notification center with:
  - Infinite scroll
  - Read/unread status
  - Action buttons
  - Filtering options
- Notification preferences management

## Dependencies
- Firestore real-time capabilities
- WebSocket support in the application
- Frontend notification components
- Admin dashboard framework

## Feature Dependencies
- F001: User Onboarding (for user document structure)
- F002: Initial Assessment Call (for call-related notifications)
- F003: Job Search (for search-related notifications)

## Migration Plan
1. Create new notification subcollections
2. Implement data model and CRUD operations
3. Add real-time listeners
4. Migrate existing error states to notification system
5. Update UI components
6. Deploy admin dashboard features

## Example Use Cases

### Error Notifications
```json
{
  "id": "notif_123",
  "type": "error",
  "title": "Resume Processing Failed",
  "message": "We couldn't process your resume due to formatting issues. <a href='/help/resume-guidelines'>Learn more</a>",
  "actionURL": "/upload-resume",
  "actionText": "Upload New Resume",
  "severity": "high",
  "source": "resume_processor",
  "metadata": {
    "error_code": "PARSE_ERROR",
    "file_name": "resume.pdf"
  }
}
```

### Status Update Notifications
```json
{
  "id": "notif_124",
  "type": "info",
  "title": "Job Search Progress",
  "message": "We've found 5 new matches based on your profile",
  "actionURL": "/job-matches",
  "actionText": "View Matches",
  "severity": "low",
  "source": "job_matcher"
}
```

## Future Enhancements
- Push notifications integration
- Email notification digests
- Advanced notification rules engine
- Machine learning for notification prioritization
- Integration with external notification services
- Notification templates system
- A/B testing for notification effectiveness 