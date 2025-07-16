package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	heldResponsesCollection = "held_responses"
	HeldResponseCacheTTL    = 5 * time.Minute
)

// HeldResponseStatus represents the current status of a held response
type HeldResponseStatus string

const (
	HeldResponseStatusPending  HeldResponseStatus = "pending"  // Waiting for admin review
	HeldResponseStatusApproved HeldResponseStatus = "approved" // Approved and sent
	HeldResponseStatusRevision HeldResponseStatus = "revision" // Needs revision based on feedback
	HeldResponseStatusExpired  HeldResponseStatus = "expired"  // Expired without action
)

// IsValid checks if the HeldResponseStatus is valid
func (s HeldResponseStatus) IsValid() bool {
	switch s {
	case HeldResponseStatusPending, HeldResponseStatusApproved, HeldResponseStatusRevision, HeldResponseStatusExpired:
		return true
	default:
		return false
	}
}

// String returns the string representation of the HeldResponseStatus
func (s HeldResponseStatus) String() string {
	return string(s)
}

// HeldResponse represents an email response that is being held for admin approval
type HeldResponse struct {
	ID             string // Document ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Status         HeldResponseStatus
	ExpiresAt      time.Time
	
	// Original email information
	OriginalEmailID      string
	OriginalFrom         string
	OriginalSubject      string
	OriginalBody         string
	OriginalDate         time.Time
	OriginalHeaders      map[string]string
	
	// Generated response information
	ResponseSubject      string
	ResponseBody         string
	ResponseHTMLBody     string
	ResponseType         string // confirmation, partial, escalation
	ResponseSummaries    []string
	ResponseEscalatedItems []string
	
	// Processing information
	HandlingResults      []map[string]any
	RecipientEmail       string
	
	// Admin workflow information
	AdminFeedback        string
	AdminUserID          string
	ReviewedAt           time.Time
	ApprovedAt           time.Time
	SentAt               time.Time
	RevisionCount        int
	
	// Hold criteria information
	HoldReason           string
	HoldCriteria         string
}

// IsValid validates that the HeldResponse has required fields
func (hr *HeldResponse) IsValid() error {
	if hr.OriginalEmailID == "" {
		return fmt.Errorf("original email ID cannot be empty")
	}
	if hr.OriginalFrom == "" {
		return fmt.Errorf("original from cannot be empty")
	}
	if hr.RecipientEmail == "" {
		return fmt.Errorf("recipient email cannot be empty")
	}
	if hr.ResponseSubject == "" {
		return fmt.Errorf("response subject cannot be empty")
	}
	if hr.ResponseBody == "" && hr.ResponseHTMLBody == "" {
		return fmt.Errorf("response must have either body or HTML body")
	}
	if !hr.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", hr.Status)
	}
	return nil
}

// IsExpired checks if the held response has expired
func (hr *HeldResponse) IsExpired() bool {
	return time.Now().After(hr.ExpiresAt)
}

// IsPending checks if the held response is pending review
func (hr *HeldResponse) IsPending() bool {
	return hr.Status == HeldResponseStatusPending
}

// IsApproved checks if the held response has been approved
func (hr *HeldResponse) IsApproved() bool {
	return hr.Status == HeldResponseStatusApproved
}

// NeedsRevision checks if the held response needs revision
func (hr *HeldResponse) NeedsRevision() bool {
	return hr.Status == HeldResponseStatusRevision
}

// GetPreviewText returns a truncated preview of the response body
func (hr *HeldResponse) GetPreviewText() string {
	text := hr.ResponseBody
	if text == "" {
		// Strip HTML tags for preview if no plain text
		text = hr.ResponseHTMLBody
	}
	
	const maxLength = 200
	if len(text) <= maxLength {
		return text
	}
	
	return text[:maxLength] + "..."
}

// CreateHeldResponse creates a new held response in Firestore
func CreateHeldResponse(ctx context.Context, fs *FirestoreClient, heldResponse *HeldResponse) error {
	if fs == nil {
		return fmt.Errorf("firestore client cannot be nil")
	}
	if heldResponse == nil {
		return fmt.Errorf("held response cannot be nil")
	}
	
	// Validate the held response
	if err := heldResponse.IsValid(); err != nil {
		return fmt.Errorf("invalid held response: %w", err)
	}
	
	// Generate ID if not provided
	if heldResponse.ID == "" {
		heldResponse.ID = uuid.New().String()
	}
	
	// Set timestamps
	now := time.Now()
	heldResponse.CreatedAt = now
	heldResponse.UpdatedAt = now
	
	// Set default status if not provided
	if heldResponse.Status == "" {
		heldResponse.Status = HeldResponseStatusPending
	}
	
	// Set default expiration if not provided (24 hours)
	if heldResponse.ExpiresAt.IsZero() {
		heldResponse.ExpiresAt = now.Add(24 * time.Hour)
	}
	
	// Create document in Firestore
	_, err := fs.Collection(heldResponsesCollection).Doc(heldResponse.ID).Set(ctx, heldResponse)
	if err != nil {
		return fmt.Errorf("failed to create held response: %w", err)
	}
	
	return nil
}

// GetHeldResponse retrieves a held response by ID
func GetHeldResponse(ctx context.Context, fs *FirestoreClient, id string) (*HeldResponse, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("held response ID cannot be empty")
	}
	
	doc, err := fs.Collection(heldResponsesCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get held response: %w", err)
	}
	
	var heldResponse HeldResponse
	if err := doc.DataTo(&heldResponse); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal held response: %w", err)
	}
	
	heldResponse.ID = doc.Ref.ID
	return &heldResponse, true, nil
}

// UpdateHeldResponse updates an existing held response
func UpdateHeldResponse(ctx context.Context, fs *FirestoreClient, heldResponse *HeldResponse) error {
	if heldResponse == nil {
		return fmt.Errorf("held response cannot be nil")
	}
	
	if heldResponse.ID == "" {
		return fmt.Errorf("held response ID cannot be empty")
	}
	
	// Validate the held response
	if err := heldResponse.IsValid(); err != nil {
		return fmt.Errorf("invalid held response: %w", err)
	}
	
	// Update timestamp
	heldResponse.UpdatedAt = time.Now()
	
	// Update document in Firestore
	_, err := fs.Collection(heldResponsesCollection).Doc(heldResponse.ID).Set(ctx, heldResponse)
	if err != nil {
		return fmt.Errorf("failed to update held response: %w", err)
	}
	
	return nil
}

// UpdateHeldResponseStatus updates the status of a held response
func UpdateHeldResponseStatus(ctx context.Context, fs *FirestoreClient, id string, status HeldResponseStatus) error {
	if id == "" {
		return fmt.Errorf("held response ID cannot be empty")
	}
	
	if !status.IsValid() {
		return fmt.Errorf("invalid status: %s", status)
	}
	
	updates := map[string]any{
		"Status":     status,
		"UpdatedAt": time.Now(),
	}
	
	// Set specific timestamps based on status
	switch status {
	case HeldResponseStatusApproved:
		updates["ApprovedAt"] = time.Now()
	case HeldResponseStatusRevision:
		updates["ReviewedAt"] = time.Now()
	}
	
	_, err := fs.Collection(heldResponsesCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "Status", Value: status},
		{Path: "UpdatedAt", Value: time.Now()},
	})
	if err != nil {
		return fmt.Errorf("failed to update held response status: %w", err)
	}
	
	return nil
}

// DeleteHeldResponse deletes a held response
func DeleteHeldResponse(ctx context.Context, fs *FirestoreClient, id string) error {
	if id == "" {
		return fmt.Errorf("held response ID cannot be empty")
	}
	
	_, err := fs.Collection(heldResponsesCollection).Doc(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete held response: %w", err)
	}
	
	return nil
}

// ListHeldResponses retrieves held responses with pagination
func ListHeldResponses(ctx context.Context, fs *FirestoreClient, status HeldResponseStatus, limit int, offset int) ([]*HeldResponse, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	
	query := fs.Collection(heldResponsesCollection).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit).
		Offset(offset)
	
	// Filter by status if provided
	if status != "" && status.IsValid() {
		query = query.Where("Status", "==", status)
	}
	
	iter := query.Documents(ctx)
	defer iter.Stop()
	
	var heldResponses []*HeldResponse
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate held responses: %w", err)
		}
		
		var heldResponse HeldResponse
		if err := doc.DataTo(&heldResponse); err != nil {
			return nil, fmt.Errorf("failed to unmarshal held response: %w", err)
		}
		
		heldResponse.ID = doc.Ref.ID
		heldResponses = append(heldResponses, &heldResponse)
	}
	
	return heldResponses, nil
}

// GetPendingHeldResponses retrieves all pending held responses
func GetPendingHeldResponses(ctx context.Context, fs *FirestoreClient) ([]*HeldResponse, error) {
	return ListHeldResponses(ctx, fs, HeldResponseStatusPending, 100, 0)
}

// GetExpiredHeldResponses retrieves held responses that have expired
func GetExpiredHeldResponses(ctx context.Context, fs *FirestoreClient) ([]*HeldResponse, error) {
	// Get expired responses with pending status
	pendingIter := fs.Collection(heldResponsesCollection).
		Where("ExpiresAt", "<", time.Now()).
		Where("Status", "==", HeldResponseStatusPending).
		Documents(ctx)
	defer pendingIter.Stop()
	
	// Get expired responses with revision status
	revisionIter := fs.Collection(heldResponsesCollection).
		Where("ExpiresAt", "<", time.Now()).
		Where("Status", "==", HeldResponseStatusRevision).
		Documents(ctx)
	defer revisionIter.Stop()
	
	var expiredResponses []*HeldResponse
	
	// Process pending responses
	for {
		doc, err := pendingIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate expired pending held responses: %w", err)
		}
		
		var heldResponse HeldResponse
		if err := doc.DataTo(&heldResponse); err != nil {
			return nil, fmt.Errorf("failed to unmarshal held response: %w", err)
		}
		
		heldResponse.ID = doc.Ref.ID
		expiredResponses = append(expiredResponses, &heldResponse)
	}
	
	// Process revision responses
	for {
		doc, err := revisionIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate expired revision held responses: %w", err)
		}
		
		var heldResponse HeldResponse
		if err := doc.DataTo(&heldResponse); err != nil {
			return nil, fmt.Errorf("failed to unmarshal held response: %w", err)
		}
		
		heldResponse.ID = doc.Ref.ID
		expiredResponses = append(expiredResponses, &heldResponse)
	}
	
	return expiredResponses, nil
}

// CountHeldResponsesByStatus returns count of held responses by status
func CountHeldResponsesByStatus(ctx context.Context, fs *FirestoreClient, status HeldResponseStatus) (int, error) {
	if !status.IsValid() {
		return 0, fmt.Errorf("invalid status: %s", status)
	}
	
	query := fs.Collection(heldResponsesCollection).Where("Status", "==", status)
	
	// Count documents
	iter := query.Documents(ctx)
	defer iter.Stop()
	
	count := 0
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to count held responses: %w", err)
		}
		count++
	}
	
	return count, nil
}