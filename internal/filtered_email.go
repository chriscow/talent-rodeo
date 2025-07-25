package internal

import (
	"context"
	"fmt"
	"time"

	"talent-rodeo/internal/inbox"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	filteredEmailsCollection = "filtered_emails"
	FilteredEmailCacheTTL    = 5 * time.Minute
)

// FilteredEmailReason represents why an email was filtered
type FilteredEmailReason string

const (
	FilteredEmailReasonUserNotFound FilteredEmailReason = "user_not_found" // User not registered
	FilteredEmailReasonInvalidEmail FilteredEmailReason = "invalid_email"  // Email format invalid
	FilteredEmailReasonSpam         FilteredEmailReason = "spam_detected"  // Detected as spam
	FilteredEmailReasonDuplicate    FilteredEmailReason = "duplicate"      // Duplicate email
	FilteredEmailReasonOther        FilteredEmailReason = "other"          // Other reason
)

// String returns the string representation of the FilteredEmailReason
func (r FilteredEmailReason) String() string {
	return string(r)
}

// IsValid checks if the FilteredEmailReason is valid
func (r FilteredEmailReason) IsValid() bool {
	switch r {
	case FilteredEmailReasonUserNotFound, FilteredEmailReasonInvalidEmail,
		FilteredEmailReasonSpam, FilteredEmailReasonDuplicate, FilteredEmailReasonOther:
		return true
	default:
		return false
	}
}

// FilteredEmail represents an email that was filtered out by the system
type FilteredEmail struct {
	ID              string              `firestore:"id"`
	OriginalEmailID string              `firestore:"original_email_id"`
	From            string              `firestore:"from"`
	Subject         string              `firestore:"subject"`
	Body            string              `firestore:"body"`
	FilterName      string              `firestore:"filter_name"`     // Which filter rejected it
	FilterReason    FilteredEmailReason `firestore:"filter_reason"`   // Why it was filtered
	ReasonDetails   string              `firestore:"reason_details"`  // Additional details
	ExtractedEmail  string              `firestore:"extracted_email"` // Email extracted from From field
	CreatedAt       time.Time           `firestore:"created_at"`
	UpdatedAt       time.Time           `firestore:"updated_at"`
}

// CreateFilteredEmail creates a new filtered email record in Firestore
func CreateFilteredEmail(ctx context.Context, fs *FirestoreClient, filteredEmail *FilteredEmail) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}

	if filteredEmail.ID == "" {
		filteredEmail.ID = uuid.New().String()
	}

	now := time.Now()
	filteredEmail.CreatedAt = now
	filteredEmail.UpdatedAt = now

	// Validate required fields
	if filteredEmail.OriginalEmailID == "" {
		return fmt.Errorf("original email ID is required")
	}
	if filteredEmail.FilterName == "" {
		return fmt.Errorf("filter name is required")
	}
	if !filteredEmail.FilterReason.IsValid() {
		return fmt.Errorf("invalid filter reason: %s", filteredEmail.FilterReason)
	}

	_, err := fs.Collection(filteredEmailsCollection).Doc(filteredEmail.ID).Create(ctx, filteredEmail)
	if err != nil {
		return fmt.Errorf("failed to create filtered email record: %w", err)
	}

	return nil
}

// GetFilteredEmail retrieves a filtered email by ID
func GetFilteredEmail(ctx context.Context, fs *FirestoreClient, id string) (*FilteredEmail, bool, error) {
	if fs == nil {
		return nil, false, fmt.Errorf("firestore client is nil")
	}

	doc, err := fs.Collection(filteredEmailsCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get filtered email: %w", err)
	}

	var filteredEmail FilteredEmail
	if err := doc.DataTo(&filteredEmail); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal filtered email data: %w", err)
	}

	return &filteredEmail, true, nil
}

// ListFilteredEmails retrieves filtered emails with optional filtering and pagination
func ListFilteredEmails(ctx context.Context, fs *FirestoreClient, filterName string, reason FilteredEmailReason, limit int) ([]*FilteredEmail, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	query := fs.Collection(filteredEmailsCollection).OrderBy("created_at", firestore.Desc)

	// Apply filters
	if filterName != "" {
		query = query.Where("filter_name", "==", filterName)
	}
	if reason != "" && reason.IsValid() {
		query = query.Where("filter_reason", "==", string(reason))
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var filteredEmails []*FilteredEmail
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate filtered emails: %w", err)
		}

		var filteredEmail FilteredEmail
		if err := doc.DataTo(&filteredEmail); err != nil {
			return nil, fmt.Errorf("failed to unmarshal filtered email data: %w", err)
		}
		filteredEmails = append(filteredEmails, &filteredEmail)
	}

	return filteredEmails, nil
}

// GetFilteredEmailStats returns statistics about filtered emails
func GetFilteredEmailStats(ctx context.Context, fs *FirestoreClient) (map[string]interface{}, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	stats := make(map[string]interface{})

	// Get total count
	totalQuery := fs.Collection(filteredEmailsCollection)
	totalIter := totalQuery.Documents(ctx)
	defer totalIter.Stop()

	totalCount := 0
	filterCounts := make(map[string]int)
	reasonCounts := make(map[string]int)

	for {
		doc, err := totalIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate filtered emails for stats: %w", err)
		}

		var filteredEmail FilteredEmail
		if err := doc.DataTo(&filteredEmail); err != nil {
			continue // Skip malformed documents
		}

		totalCount++
		filterCounts[filteredEmail.FilterName]++
		reasonCounts[string(filteredEmail.FilterReason)]++
	}

	stats["total_filtered"] = totalCount
	stats["by_filter"] = filterCounts
	stats["by_reason"] = reasonCounts

	return stats, nil
}

// FilteredEmailService provides business logic for managing filtered email records
type FilteredEmailService struct {
	firestoreClient *FirestoreClient
}

// NewFilteredEmailService creates a new filtered email service
func NewFilteredEmailService(firestoreClient *FirestoreClient) *FilteredEmailService {
	return &FilteredEmailService{
		firestoreClient: firestoreClient,
	}
}

// RecordFilteredEmail records an email that was filtered out
func (s *FilteredEmailService) RecordFilteredEmail(ctx context.Context, email *inbox.Email, filterName string, reason FilteredEmailReason, reasonDetails, extractedEmail string) error {
	filteredEmail := &FilteredEmail{
		OriginalEmailID: email.ID,
		From:            email.From,
		Subject:         email.Subject,
		Body:            email.Body,
		FilterName:      filterName,
		FilterReason:    reason,
		ReasonDetails:   reasonDetails,
		ExtractedEmail:  extractedEmail,
	}

	return CreateFilteredEmail(ctx, s.firestoreClient, filteredEmail)
}

// GetStats returns comprehensive statistics about filtered emails
func (s *FilteredEmailService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return GetFilteredEmailStats(ctx, s.firestoreClient)
}

// GetFilteredEmailsByFilter returns filtered emails for a specific filter
func (s *FilteredEmailService) GetFilteredEmailsByFilter(ctx context.Context, filterName string, limit int) ([]*FilteredEmail, error) {
	return ListFilteredEmails(ctx, s.firestoreClient, filterName, "", limit)
}

// GetFilteredEmailsByReason returns filtered emails for a specific reason
func (s *FilteredEmailService) GetFilteredEmailsByReason(ctx context.Context, reason FilteredEmailReason, limit int) ([]*FilteredEmail, error) {
	return ListFilteredEmails(ctx, s.firestoreClient, "", reason, limit)
}
