package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"talent-rodeo/internal/inbox"
)

// EmailResponder defines the interface for generating email responses
type EmailResponder interface {
	GenerateResponse(ctx context.Context, email *inbox.Email, results []*inbox.HandlingResult) (*inbox.EmailResponse, error)
}

// EscalationService defines the interface for escalation services
type EscalationService interface {
	SendEscalation(ctx context.Context, email *inbox.Email, unhandledItems []string) error
	IsConfigured() bool
}

// EmailHandler defines the interface for processing emails and returning handling results.
// Implementations should return a HandlingResult indicating whether they processed
// the email and what actions were taken.
type EmailHandler interface {
	HandleEmail(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error)
}

// EmailHandlerFunc is a function type that implements EmailHandler
type EmailHandlerFunc func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error)

func (f EmailHandlerFunc) HandleEmail(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
	return f(ctx, email)
}

// InboxDispatch implements EmailHandler and dispatches emails to multiple registered handlers.
// It orchestrates the full email processing workflow including response generation and escalation.
type InboxDispatch struct {
	mu                sync.RWMutex
	handlers          map[string]EmailHandler
	emailResponder    EmailResponder
	responseSender    ResponseSender
	escalationService EscalationService
	heldResponseService *HeldResponseService
	logger            *slog.Logger
	responseEnabled   bool
}

// NewInboxDispatch creates a new InboxDispatch instance with basic configuration.
func NewInboxDispatch() *InboxDispatch {
	return &InboxDispatch{
		handlers:        make(map[string]EmailHandler),
		responseEnabled: false,
	}
}

// NewEnhancedInboxDispatch creates a new InboxDispatch instance with response and escalation capabilities.
func NewEnhancedInboxDispatch(
	emailResponder EmailResponder,
	responseSender ResponseSender,
	escalationService EscalationService,
	heldResponseService *HeldResponseService,
	logger *slog.Logger,
) *InboxDispatch {
	return &InboxDispatch{
		handlers:          make(map[string]EmailHandler),
		emailResponder:    emailResponder,
		responseSender:    responseSender,
		escalationService: escalationService,
		heldResponseService: heldResponseService,
		logger:            logger,
		responseEnabled:   true,
	}
}

// SetResponseEnabled enables or disables email response generation.
func (id *InboxDispatch) SetResponseEnabled(enabled bool) {
	id.mu.Lock()
	defer id.mu.Unlock()
	id.responseEnabled = enabled
}

// IsResponseEnabled returns whether email response generation is enabled.
func (id *InboxDispatch) IsResponseEnabled() bool {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.responseEnabled
}

// Register adds a new handler with the given ID.
// If a handler with the same ID already exists, it will be replaced.
// Returns an error if the id is empty or the handler is nil.
func (id *InboxDispatch) Register(handlerID string, handler EmailHandler) error {
	if handlerID == "" {
		return fmt.Errorf("handler ID cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	id.handlers[handlerID] = handler
	return nil
}

// Deregister removes a handler by ID
// Returns true if the handler was found and removed, false otherwise
func (id *InboxDispatch) Deregister(handlerID string) bool {
	id.mu.Lock()
	defer id.mu.Unlock()

	if _, exists := id.handlers[handlerID]; exists {
		delete(id.handlers, handlerID)
		return true
	}
	return false
}

// GetHandlerCount returns the number of currently registered handlers
func (id *InboxDispatch) GetHandlerCount() int {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return len(id.handlers)
}

// HandleEmail implements inbox.EmailHandler and dispatches the email to all registered handlers.
// It orchestrates the full email processing workflow including response generation and escalation.
func (id *InboxDispatch) HandleEmail(ctx context.Context, email *inbox.Email) error {
	if email == nil {
		return fmt.Errorf("email cannot be nil")
	}

	if id.logger != nil {
		id.logger.Info("inbox dispatch starting email processing",
			"email_id", email.ID,
			"from", email.From,
			"subject", email.Subject,
			"handler_count", id.GetHandlerCount(),
			"response_enabled", id.IsResponseEnabled(),
		)
	}

	// Process email through all handlers
	results, err := id.processHandlers(ctx, email)
	if err != nil {
		if id.logger != nil {
			id.logger.Error("handler processing failed",
				"error", err,
				"email_id", email.ID,
				"from", email.From,
			)
		}
		return fmt.Errorf("handler processing failed: %w", err)
	}

	if id.logger != nil {
		id.logger.Info("handler processing completed",
			"email_id", email.ID,
			"results_count", len(results),
			"handled_count", id.countHandledResults(results),
			"unhandled_count", id.countUnhandledResults(results),
		)
	}

	// If response generation is disabled, just return after processing handlers
	if !id.IsResponseEnabled() {
		if id.logger != nil {
			id.logger.Debug("response generation disabled, skipping response and escalation",
				"email_id", email.ID,
			)
		}
		return nil
	}

	// Generate and send response if enabled
	if err := id.handleResponse(ctx, email, results); err != nil {
		// Log error but don't fail the entire process
		if id.logger != nil {
			id.logger.Error("failed to handle response",
				"error", err,
				"email_id", email.ID,
				"from", email.From,
			)
		}
	}

	// Handle escalation if needed
	if err := id.handleEscalation(ctx, email, results); err != nil {
		// Log error but don't fail the entire process
		if id.logger != nil {
			id.logger.Error("failed to handle escalation",
				"error", err,
				"email_id", email.ID,
				"from", email.From,
			)
		}
	}

	if id.logger != nil {
		id.logger.Info("inbox dispatch completed email processing",
			"email_id", email.ID,
			"from", email.From,
		)
	}

	return nil
}

// processHandlers runs all registered handlers and collects their results
func (id *InboxDispatch) processHandlers(ctx context.Context, email *inbox.Email) ([]*inbox.HandlingResult, error) {
	id.mu.RLock()
	handlers := make([]EmailHandler, 0, len(id.handlers))
	handlerNames := make([]string, 0, len(id.handlers))
	for name, handler := range id.handlers {
		handlers = append(handlers, handler)
		handlerNames = append(handlerNames, name)
	}
	id.mu.RUnlock()

	if id.logger != nil {
		id.logger.Debug("processing email through handlers",
			"email_id", email.ID,
			"handler_count", len(handlers),
			"handler_names", handlerNames,
		)
	}

	var results []*inbox.HandlingResult
	var errors []error

	for i, handler := range handlers {
		handlerName := handlerNames[i]
		func() {
			defer func() {
				// Recover from panics and treat them as errors
				if r := recover(); r != nil {
					if id.logger != nil {
						id.logger.Error("handler panicked",
							"handler_name", handlerName,
							"email_id", email.ID,
							"panic", r,
						)
					}
					errors = append(errors, fmt.Errorf("handler %s panicked: %v", handlerName, r))
				}
			}()

			if id.logger != nil {
				id.logger.Debug("calling handler",
					"handler_name", handlerName,
					"email_id", email.ID,
				)
			}

			result, err := handler.HandleEmail(ctx, email)
			if err != nil {
				if id.logger != nil {
					id.logger.Error("handler returned error",
						"handler_name", handlerName,
						"email_id", email.ID,
						"error", err,
					)
				}
				errors = append(errors, fmt.Errorf("handler %s failed: %w", handlerName, err))
				return
			}

			if result != nil {
				if id.logger != nil {
					id.logger.Info("handler completed",
						"handler_name", handlerName,
						"email_id", email.ID,
						"handled", result.Handled,
						"summary", result.Summary,
					)
				}
				results = append(results, result)
			} else {
				if id.logger != nil {
					id.logger.Debug("handler returned nil result",
						"handler_name", handlerName,
						"email_id", email.ID,
					)
				}
			}
		}()
	}

	// If any errors occurred, return them as a combined error
	if len(errors) > 0 {
		if id.logger != nil {
			id.logger.Error("handler processing had errors",
				"email_id", email.ID,
				"error_count", len(errors),
				"errors", errors,
			)
		}
		return nil, fmt.Errorf("errors occurred in %d handlers: %v", len(errors), errors)
	}

	return results, nil
}

// handleResponse generates and sends a response email to the user
func (id *InboxDispatch) handleResponse(ctx context.Context, email *inbox.Email, results []*inbox.HandlingResult) error {
	if id.logger != nil {
		id.logger.Debug("starting response handling",
			"email_id", email.ID,
			"results_count", len(results),
		)
	}

	if id.emailResponder == nil || id.responseSender == nil {
		if id.logger != nil {
			id.logger.Debug("response components not configured, skipping response",
				"email_id", email.ID,
				"has_responder", id.emailResponder != nil,
				"has_sender", id.responseSender != nil,
			)
		}
		return fmt.Errorf("response components not configured")
	}

	if len(results) == 0 {
		if id.logger != nil {
			id.logger.Debug("no handler results, skipping response generation",
				"email_id", email.ID,
			)
		}
		return nil
	}

	// Extract user email from the original email
	userEmail, err := id.extractUserEmail(email)
	if err != nil {
		if id.logger != nil {
			id.logger.Error("failed to extract user email for response",
				"email_id", email.ID,
				"from", email.From,
				"error", err,
			)
		}
		return fmt.Errorf("failed to extract user email: %w", err)
	}

	if id.logger != nil {
		id.logger.Debug("generating response email",
			"email_id", email.ID,
			"recipient", userEmail,
		)
	}

	// Generate response based on handling results
	response, err := id.emailResponder.GenerateResponse(ctx, email, results)
	if err != nil {
		if id.logger != nil {
			id.logger.Error("failed to generate email response",
				"email_id", email.ID,
				"recipient", userEmail,
				"error", err,
			)
		}
		return fmt.Errorf("failed to generate response: %w", err)
	}

	// Check if response should be held for admin review
	if id.heldResponseService != nil {
		holdDecision := id.heldResponseService.EvaluateHoldDecision(ctx, email, response, userEmail)
		if holdDecision.ShouldHold {
			// Create held response record
			_, err := id.heldResponseService.CreateHeldResponse(ctx, email, response, userEmail, holdDecision)
			if err != nil {
				if id.logger != nil {
					id.logger.Error("failed to create held response",
						"email_id", email.ID,
						"recipient", userEmail,
						"error", err,
					)
				}
				// Continue with sending response if hold creation fails
			} else {
				if id.logger != nil {
					id.logger.Info("response held for admin review",
						"email_id", email.ID,
						"recipient", userEmail,
						"response_type", response.Type.String(),
						"hold_reason", holdDecision.Reason,
						"hold_criteria", holdDecision.Criteria,
					)
				}
				return nil // Response was held, don't send
			}
		}
	}

	if id.logger != nil {
		id.logger.Debug("sending response email",
			"email_id", email.ID,
			"recipient", userEmail,
			"response_type", response.Type.String(),
			"subject", response.Subject,
		)
	}

	// Send response to user
	err = id.responseSender.SendResponse(ctx, response, userEmail)
	if err != nil {
		if id.logger != nil {
			id.logger.Error("failed to send email response",
				"email_id", email.ID,
				"recipient", userEmail,
				"response_type", response.Type.String(),
				"error", err,
			)
		}
		return fmt.Errorf("failed to send response: %w", err)
	}

	if id.logger != nil {
		id.logger.Info("email response sent",
			"email_id", email.ID,
			"recipient", userEmail,
			"response_type", response.Type.String(),
		)
	}

	return nil
}

// handleEscalation sends escalation emails to admin for unhandled items
func (id *InboxDispatch) handleEscalation(ctx context.Context, email *inbox.Email, results []*inbox.HandlingResult) error {
	if id.logger != nil {
		id.logger.Debug("checking escalation requirements",
			"email_id", email.ID,
			"escalation_configured", id.escalationService != nil && id.escalationService.IsConfigured(),
		)
	}

	if id.escalationService == nil || !id.escalationService.IsConfigured() {
		if id.logger != nil {
			id.logger.Debug("escalation not configured, skipping",
				"email_id", email.ID,
				"has_service", id.escalationService != nil,
			)
		}
		return nil
	}

	// Check if any items were unhandled
	unhandledItems := id.collectUnhandledItems(results)
	if len(unhandledItems) == 0 {
		if id.logger != nil {
			id.logger.Debug("all items handled, no escalation needed",
				"email_id", email.ID,
			)
		}
		return nil
	}

	if id.logger != nil {
		id.logger.Info("sending escalation for unhandled items",
			"email_id", email.ID,
			"from", email.From,
			"unhandled_count", len(unhandledItems),
			"unhandled_items", unhandledItems,
		)
	}

	// Send escalation to admin
	err := id.escalationService.SendEscalation(ctx, email, unhandledItems)
	if err != nil {
		if id.logger != nil {
			id.logger.Error("failed to send admin escalation",
				"email_id", email.ID,
				"from", email.From,
				"unhandled_count", len(unhandledItems),
				"error", err,
			)
		}
		return fmt.Errorf("failed to send escalation: %w", err)
	}

	if id.logger != nil {
		id.logger.Info("escalation sent",
			"email_id", email.ID,
			"from", email.From,
			"unhandled_count", len(unhandledItems),
		)
	}

	return nil
}

// extractUserEmail extracts the user's email address from the email
func (id *InboxDispatch) extractUserEmail(email *inbox.Email) (string, error) {
	if email.From == "" {
		return "", fmt.Errorf("email from field is empty")
	}

	// Handle "Name <email@domain.com>" format
	userEmail := email.From
	if start := findIndex(userEmail, "<"); start >= 0 {
		if end := findIndex(userEmail, ">"); end > start {
			userEmail = userEmail[start+1 : end]
		}
	}

	userEmail = trimSpace(userEmail)

	// Basic validation
	if userEmail == "" || !containsChar(userEmail, '@') {
		return "", fmt.Errorf("invalid email format: %s", email.From)
	}

	return userEmail, nil
}

// collectUnhandledItems extracts descriptions of unhandled items from results
func (id *InboxDispatch) collectUnhandledItems(results []*inbox.HandlingResult) []string {
	var unhandledItems []string

	// If no results at all, the entire email was unhandled
	if len(results) == 0 {
		unhandledItems = append(unhandledItems, "Email could not be processed by any handler")
		return unhandledItems
	}

	// Check for specific unhandled items
	hasAnyHandled := false
	for _, result := range results {
		if result.Handled {
			hasAnyHandled = true
		} else {
			unhandledItems = append(unhandledItems, "Part of the request could not be processed automatically")
		}
	}

	// If nothing was handled at all, add a general message
	if !hasAnyHandled {
		if len(unhandledItems) == 0 {
			unhandledItems = append(unhandledItems, "Email requires manual review and response")
		}
	}

	return unhandledItems
}

// Helper functions to avoid string imports
func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func containsChar(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	// Trim leading spaces
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing spaces
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

// countHandledResults counts how many results were handled
func (id *InboxDispatch) countHandledResults(results []*inbox.HandlingResult) int {
	count := 0
	for _, result := range results {
		if result.Handled {
			count++
		}
	}
	return count
}

// countUnhandledResults counts how many results were not handled
func (id *InboxDispatch) countUnhandledResults(results []*inbox.HandlingResult) int {
	count := 0
	for _, result := range results {
		if !result.Handled {
			count++
		}
	}
	return count
}
