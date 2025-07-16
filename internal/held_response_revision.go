package internal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/inbox"
)

// RevisionProcessor handles LLM-based response revisions for held responses.
type RevisionProcessor struct {
	firestoreClient     *FirestoreClient
	heldResponseService *HeldResponseService
	emailHandler        inbox.EmailHandler
	logger              *slog.Logger
}

// NewRevisionProcessor creates a new revision processor.
func NewRevisionProcessor(firestoreClient *FirestoreClient, heldResponseService *HeldResponseService, emailHandler inbox.EmailHandler, logger *slog.Logger) *RevisionProcessor {
	return &RevisionProcessor{
		firestoreClient:     firestoreClient,
		heldResponseService: heldResponseService,
		emailHandler:        emailHandler,
		logger:              logger,
	}
}

// ProcessRevisionRequests processes all held responses that are in revision status
// and generates improved responses based on admin feedback.
func (rp *RevisionProcessor) ProcessRevisionRequests(ctx context.Context) error {
	if rp.logger != nil {
		rp.logger.Info("starting revision request processing")
	}

	// Get all responses awaiting revision
	revisionResponses, err := rp.heldResponseService.GetRevisionResponses(ctx)
	if err != nil {
		return fmt.Errorf("failed to get revision responses: %w", err)
	}

	if len(revisionResponses) == 0 {
		if rp.logger != nil {
			rp.logger.Debug("no revision requests to process")
		}
		return nil
	}

	if rp.logger != nil {
		rp.logger.Info("processing revision requests", "count", len(revisionResponses))
	}

	processed := 0
	for _, heldResponse := range revisionResponses {
		if err := rp.processRevision(ctx, heldResponse); err != nil {
			if rp.logger != nil {
				rp.logger.Error("failed to process revision",
					"held_response_id", heldResponse.ID,
					"error", err,
				)
			}
			continue
		}
		processed++
	}

	if rp.logger != nil {
		rp.logger.Info("completed revision processing",
			"total", len(revisionResponses),
			"processed", processed,
			"failed", len(revisionResponses)-processed,
		)
	}

	return nil
}

// processRevision processes a single held response revision request.
func (rp *RevisionProcessor) processRevision(ctx context.Context, heldResponse *HeldResponse) error {
	if heldResponse.AdminFeedback == "" {
		return fmt.Errorf("no admin feedback provided for revision")
	}

	// Create revision prompt with admin feedback
	_ = rp.createRevisionPrompt(heldResponse) // Placeholder for future LLM integration

	// TODO: Implement LLM revision generation
	// This requires integration with the actual email response generation system
	// For now, we just log that a revision was requested
	if rp.logger != nil {
		rp.logger.Info("revision requested - manual processing required",
			"held_response_id", heldResponse.ID,
			"feedback", heldResponse.AdminFeedback,
		)
	}
	
	// NOTE: This is a placeholder - in production, this would:
	// 1. Generate a revised response using the LLM with the admin feedback
	// 2. Update the held response with the new content
	// 3. Reset status to pending for re-approval
	return fmt.Errorf("automatic revision processing not yet implemented - manual intervention required")
}

// createRevisionPrompt creates a prompt for the LLM that includes the original response
// and admin feedback to generate an improved version.
func (rp *RevisionProcessor) createRevisionPrompt(heldResponse *HeldResponse) string {
	var promptBuilder strings.Builder

	promptBuilder.WriteString("REVISION REQUEST: The following email response was held for admin review and requires revision based on the feedback provided.\n\n")

	promptBuilder.WriteString("ORIGINAL EMAIL:\n")
	promptBuilder.WriteString(fmt.Sprintf("From: %s\n", heldResponse.OriginalFrom))
	promptBuilder.WriteString(fmt.Sprintf("Subject: %s\n", heldResponse.OriginalSubject))
	promptBuilder.WriteString(fmt.Sprintf("Body: %s\n\n", heldResponse.OriginalBody))

	promptBuilder.WriteString("PREVIOUS RESPONSE (NEEDS REVISION):\n")
	promptBuilder.WriteString(fmt.Sprintf("Subject: %s\n", heldResponse.ResponseSubject))
	if heldResponse.ResponseHTMLBody != "" {
		promptBuilder.WriteString(fmt.Sprintf("Body: %s\n\n", heldResponse.ResponseHTMLBody))
	} else {
		promptBuilder.WriteString(fmt.Sprintf("Body: %s\n\n", heldResponse.ResponseBody))
	}

	promptBuilder.WriteString("ADMIN FEEDBACK:\n")
	promptBuilder.WriteString(fmt.Sprintf("%s\n\n", heldResponse.AdminFeedback))

	promptBuilder.WriteString("REVISION INSTRUCTIONS:\n")
	promptBuilder.WriteString("Please generate an improved response that addresses the admin feedback while maintaining professionalism and helpfulness. ")
	promptBuilder.WriteString("The revised response should:\n")
	promptBuilder.WriteString("1. Address all concerns raised in the admin feedback\n")
	promptBuilder.WriteString("2. Maintain appropriate tone and professionalism\n")
	promptBuilder.WriteString("3. Be relevant to the original email inquiry\n")
	promptBuilder.WriteString("4. Follow company communication guidelines\n\n")

	promptBuilder.WriteString("Generate the revised response now:")

	return promptBuilder.String()
}

