package internal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal/inbox"
)

// ResponseSender interface for sending email responses
type ResponseSender interface {
	SendResponse(ctx context.Context, response *inbox.EmailResponse, recipientEmail string) error
}

// HeldResponseService provides business logic for managing held email responses.
// It integrates with the existing InboxDispatch system to implement the hold-and-review workflow.
type HeldResponseService struct {
	firestoreClient *FirestoreClient
	evaluator       *HoldDecisionEvaluator
	responseSender  ResponseSender
	logger          *slog.Logger
}

// NewHeldResponseService creates a new held response service.
func NewHeldResponseService(
	firestoreClient *FirestoreClient,
	evaluator *HoldDecisionEvaluator,
	responseSender ResponseSender,
	logger *slog.Logger,
) *HeldResponseService {
	if logger != nil {
		logger = logger.With("component", "HeldResponseService", "pkg", "internal")
	}
	return &HeldResponseService{
		firestoreClient: firestoreClient,
		evaluator:       evaluator,
		responseSender:  responseSender,
		logger:          logger,
	}
}

// ShouldHoldResponse determines if a response should be held based on configured criteria.
// Returns true if the response should be held for admin review.
func (hrs *HeldResponseService) ShouldHoldResponse(ctx context.Context, originalEmail *inbox.Email, response *inbox.EmailResponse, userEmail string) bool {
	if hrs.evaluator == nil {
		return false
	}

	// Create hold decision context
	decisionContext := HoldDecisionContext{
		OriginalEmail: originalEmail,
		Response:      response,
		UserEmail:     userEmail,
		Timestamp:     time.Now(),
	}

	// Evaluate hold decision
	decision := hrs.evaluator.EvaluateHoldDecision(ctx, decisionContext)

	if decision.ShouldHold && hrs.logger != nil {
		hrs.logger.Info("response will be held",
			"email_id", originalEmail.ID,
			"reason", decision.Reason,
			"criteria", decision.Criteria,
		)
	}

	return decision.ShouldHold
}

// EvaluateHoldDecision evaluates whether a response should be held and returns the decision details.
func (hrs *HeldResponseService) EvaluateHoldDecision(ctx context.Context, originalEmail *inbox.Email, response *inbox.EmailResponse, userEmail string) HoldDecision {
	if hrs.evaluator == nil {
		return HoldDecision{
			ShouldHold: false,
			Reason:     "evaluator not configured",
			Criteria:   "no_evaluator",
		}
	}

	// Create hold decision context
	decisionContext := HoldDecisionContext{
		OriginalEmail: originalEmail,
		Response:      response,
		UserEmail:     userEmail,
		Timestamp:     time.Now(),
	}

	// Evaluate hold decision
	return hrs.evaluator.EvaluateHoldDecision(ctx, decisionContext)
}

// CreateHeldResponse creates a new held response record and stores it.
func (hrs *HeldResponseService) CreateHeldResponse(ctx context.Context, originalEmail *inbox.Email, response *inbox.EmailResponse, recipientEmail string, holdDecision HoldDecision) (*HeldResponse, error) {
	if originalEmail == nil {
		return nil, fmt.Errorf("original email cannot be nil")
	}
	if response == nil {
		return nil, fmt.Errorf("response cannot be nil")
	}
	if recipientEmail == "" {
		return nil, fmt.Errorf("recipient email cannot be empty")
	}

	now := time.Now()

	heldResponse := &HeldResponse{
		OriginalEmailID:  originalEmail.ID,
		OriginalFrom:     originalEmail.From,
		OriginalSubject:  originalEmail.Subject,
		OriginalBody:     originalEmail.Body,
		RecipientEmail:   recipientEmail,
		ResponseSubject:  response.Subject,
		ResponseBody:     response.Body,
		ResponseHTMLBody: response.HTMLBody,
		ResponseType:     response.Type.String(),
		Status:           HeldResponseStatusPending,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        holdDecision.ExpiresAt,
		HoldReason:       holdDecision.Reason,
		HoldCriteria:     holdDecision.Criteria,
		AdminFeedback:    "",
		RevisionCount:    0,
	}

	if err := CreateHeldResponse(ctx, hrs.firestoreClient, heldResponse); err != nil {
		return nil, fmt.Errorf("failed to create held response: %w", err)
	}

	if hrs.logger != nil {
		hrs.logger.Info("created held response",
			"held_response_id", heldResponse.ID,
			"email_id", originalEmail.ID,
			"recipient", recipientEmail,
			"expires_at", holdDecision.ExpiresAt,
			"reason", holdDecision.Reason,
		)
	}

	return heldResponse, nil
}

// GetPendingResponses returns all pending held responses.
func (hrs *HeldResponseService) GetPendingResponses(ctx context.Context) ([]*HeldResponse, error) {
	return GetPendingHeldResponses(ctx, hrs.firestoreClient)
}

// GetExpiredResponses returns all expired held responses.
func (hrs *HeldResponseService) GetExpiredResponses(ctx context.Context) ([]*HeldResponse, error) {
	return GetExpiredHeldResponses(ctx, hrs.firestoreClient)
}

// GetApprovedResponses returns all approved held responses.
func (hrs *HeldResponseService) GetApprovedResponses(ctx context.Context) ([]*HeldResponse, error) {
	return hrs.ListHeldResponses(ctx, HeldResponseStatusApproved, 100)
}

// GetRevisionResponses returns all held responses awaiting revision.
func (hrs *HeldResponseService) GetRevisionResponses(ctx context.Context) ([]*HeldResponse, error) {
	return hrs.ListHeldResponses(ctx, HeldResponseStatusRevision, 100)
}

// ApproveResponse approves a held response and sends it to the recipient.
func (hrs *HeldResponseService) ApproveResponse(ctx context.Context, heldResponseID, adminUserID string) error {
	// Get the held response
	heldResponse, exists, err := GetHeldResponse(ctx, hrs.firestoreClient, heldResponseID)
	if err != nil {
		return fmt.Errorf("failed to get held response: %w", err)
	}
	if !exists {
		return fmt.Errorf("held response not found: %s", heldResponseID)
	}

	if heldResponse.Status != HeldResponseStatusPending {
		return fmt.Errorf("response is not pending approval (status: %s)", heldResponse.Status)
	}

	if heldResponse.IsExpired() {
		return fmt.Errorf("response has expired")
	}

	// Parse response type from string
	responseType, err := parseResponseType(heldResponse.ResponseType)
	if err != nil {
		return fmt.Errorf("invalid response type: %w", err)
	}

	// Create EmailResponse for sending
	response := &inbox.EmailResponse{
		Type:           responseType,
		Subject:        heldResponse.ResponseSubject,
		Body:           heldResponse.ResponseBody,
		HTMLBody:       heldResponse.ResponseHTMLBody,
		GeneratedAt:    heldResponse.CreatedAt,
		Summaries:      heldResponse.ResponseSummaries,
		EscalatedItems: heldResponse.ResponseEscalatedItems,
	}

	// Send the response
	if err := hrs.responseSender.SendResponse(ctx, response, heldResponse.RecipientEmail); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	// Update the held response status
	now := time.Now()
	heldResponse.Status = HeldResponseStatusApproved
	heldResponse.ApprovedAt = now
	heldResponse.AdminUserID = adminUserID
	heldResponse.SentAt = now

	if err := UpdateHeldResponse(ctx, hrs.firestoreClient, heldResponse); err != nil {
		return fmt.Errorf("failed to update held response status: %w", err)
	}

	if hrs.logger != nil {
		hrs.logger.Info("approved and sent held response",
			"held_response_id", heldResponseID,
			"admin_user_id", adminUserID,
			"recipient", heldResponse.RecipientEmail,
		)
	}

	return nil
}

// RequestRevision marks a held response for revision with admin feedback.
func (hrs *HeldResponseService) RequestRevision(ctx context.Context, heldResponseID, adminUserID, feedback string) error {
	if feedback == "" {
		return fmt.Errorf("feedback cannot be empty when requesting revision")
	}

	// Get the held response
	heldResponse, exists, err := GetHeldResponse(ctx, hrs.firestoreClient, heldResponseID)
	if err != nil {
		return fmt.Errorf("failed to get held response: %w", err)
	}
	if !exists {
		return fmt.Errorf("held response not found: %s", heldResponseID)
	}

	if heldResponse.Status != HeldResponseStatusPending {
		return fmt.Errorf("response is not pending approval (status: %s)", heldResponse.Status)
	}

	if heldResponse.IsExpired() {
		return fmt.Errorf("response has expired")
	}

	// Update the held response with revision request
	now := time.Now()
	heldResponse.Status = HeldResponseStatusRevision
	heldResponse.AdminFeedback = feedback
	heldResponse.AdminUserID = adminUserID
	heldResponse.ReviewedAt = now
	heldResponse.RevisionCount++

	if err := UpdateHeldResponse(ctx, hrs.firestoreClient, heldResponse); err != nil {
		return fmt.Errorf("failed to update held response with revision request: %w", err)
	}

	if hrs.logger != nil {
		hrs.logger.Info("requested revision for held response",
			"held_response_id", heldResponseID,
			"admin_user_id", adminUserID,
			"feedback_length", len(feedback),
		)
	}

	return nil
}

// UpdateResponseWithRevision updates a held response with a revised version.
// This would typically be called after an LLM generates a new response based on admin feedback.
func (hrs *HeldResponseService) UpdateResponseWithRevision(ctx context.Context, heldResponseID string, revisedResponse *inbox.EmailResponse) error {
	if revisedResponse == nil {
		return fmt.Errorf("revised response cannot be nil")
	}

	// Get the held response
	heldResponse, exists, err := GetHeldResponse(ctx, hrs.firestoreClient, heldResponseID)
	if err != nil {
		return fmt.Errorf("failed to get held response: %w", err)
	}
	if !exists {
		return fmt.Errorf("held response not found: %s", heldResponseID)
	}

	if heldResponse.Status != HeldResponseStatusRevision {
		return fmt.Errorf("response is not in revision status (status: %s)", heldResponse.Status)
	}

	// Update the response content
	heldResponse.ResponseSubject = revisedResponse.Subject
	heldResponse.ResponseBody = revisedResponse.Body
	heldResponse.ResponseHTMLBody = revisedResponse.HTMLBody
	heldResponse.ResponseType = revisedResponse.Type.String()
	heldResponse.ResponseSummaries = revisedResponse.Summaries
	heldResponse.ResponseEscalatedItems = revisedResponse.EscalatedItems

	// Reset to pending for re-approval
	heldResponse.Status = HeldResponseStatusPending
	// Keep AdminFeedback for reference but clear AdminUserID since it needs re-review
	heldResponse.AdminUserID = ""

	if err := UpdateHeldResponse(ctx, hrs.firestoreClient, heldResponse); err != nil {
		return fmt.Errorf("failed to update held response with revision: %w", err)
	}

	if hrs.logger != nil {
		hrs.logger.Info("updated held response with revision",
			"held_response_id", heldResponseID,
			"new_subject", revisedResponse.Subject,
		)
	}

	return nil
}

// MarkExpired marks expired held responses as expired.
// This would typically be called by a background task.
func (hrs *HeldResponseService) MarkExpired(ctx context.Context) (int, error) {
	expiredResponses, err := GetExpiredHeldResponses(ctx, hrs.firestoreClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get expired responses: %w", err)
	}

	count := 0
	for _, response := range expiredResponses {
		if response.Status == HeldResponseStatusPending || response.Status == HeldResponseStatusRevision {
			if err := UpdateHeldResponseStatus(ctx, hrs.firestoreClient, response.ID, HeldResponseStatusExpired); err != nil {
				if hrs.logger != nil {
					hrs.logger.Error("failed to mark response as expired",
						"held_response_id", response.ID,
						"error", err,
					)
				}
				continue
			}
			count++
		}
	}

	if hrs.logger != nil && count > 0 {
		hrs.logger.Info("marked expired held responses",
			"count", count,
		)
	}

	return count, nil
}

// GetHeldResponse retrieves a held response by ID.
func (hrs *HeldResponseService) GetHeldResponse(ctx context.Context, heldResponseID string) (*HeldResponse, error) {
	heldResponse, exists, err := GetHeldResponse(ctx, hrs.firestoreClient, heldResponseID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("held response not found: %s", heldResponseID)
	}
	return heldResponse, nil
}

// ListHeldResponses retrieves held responses with optional filtering.
func (hrs *HeldResponseService) ListHeldResponses(ctx context.Context, status HeldResponseStatus, limit int) ([]*HeldResponse, error) {
	return ListHeldResponses(ctx, hrs.firestoreClient, status, limit, 0)
}

// GetResponsePreview returns a truncated preview of the response for display in lists.
func (hrs *HeldResponseService) GetResponsePreview(heldResponse *HeldResponse) string {
	if heldResponse == nil {
		return ""
	}
	return heldResponse.GetPreviewText()
}

// parseResponseType converts a string to inbox.ResponseType
func parseResponseType(typeStr string) (inbox.ResponseType, error) {
	switch typeStr {
	case "confirmation":
		return inbox.ResponseTypeConfirmation, nil
	case "partial":
		return inbox.ResponseTypePartial, nil
	case "escalation":
		return inbox.ResponseTypeEscalation, nil
	default:
		return inbox.ResponseTypeConfirmation, fmt.Errorf("unknown response type: %s", typeStr)
	}
}
