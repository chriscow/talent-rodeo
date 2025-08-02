package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/inbox"
	"talent-rodeo/internal/smtp"
	"time"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient defines the interface for OpenAI API operations.
// This interface allows for mocking in tests and abstracts the OpenAI API integration.
type OpenAIClient interface {
	// IsSearchAdjustmentRequest analyzes an email to determine if it contains a search adjustment request.
	// Returns true if the email contains a search request, along with a summary of the requested changes.
	// Returns an error if the analysis fails.
	IsSearchAdjustmentRequest(ctx context.Context, email *inbox.Email) (bool, string, error)

	// UpdateStatement merges email adjustment requests with the current user statement.
	// Takes the current statement, email content, and adjustment request summary as input.
	// Returns the updated statement or an error if the update fails.
	UpdateStatement(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error)
}

// openAIClient implements OpenAIClient using the OpenAI API.
type openAIClient struct {
	client *openai.Client
	logger *slog.Logger
}

// NewOpenAIClient creates a new OpenAI client instance.
// Takes an API key and logger as parameters.
// Returns an OpenAIClient interface that can be used for email analysis and statement updates.
func NewOpenAIClient(apiKey string, logger *slog.Logger) OpenAIClient {
	logger = logger.With("component", "OpenAIClient", "pkg", "feedback")
	return &openAIClient{
		client: openai.NewClient(apiKey),
		logger: logger,
	}
}

// IsSearchAdjustmentRequest analyzes an email to determine if it contains a search adjustment request.
func (c *openAIClient) IsSearchAdjustmentRequest(ctx context.Context, email *inbox.Email) (bool, string, error) {
	c.logger.Debug("analyzing email for search adjustment request",
		"email_id", email.ID,
		"from", email.From,
		"subject", email.Subject,
		"body_length", len(email.Body),
	)

	prompt := fmt.Sprintf(`Analyze this email to determine if the user is requesting changes to their job search parameters.

Subject: %s
From: %s
Body: %s

Instructions:
- Return "YES" if the email contains requests to modify job search criteria (location, role type, skills, experience level, etc.)
- Return "NO" if the email is unrelated to job search adjustments
- If "YES", provide a brief summary of the requested changes

Format your response as: [YES/NO]: [summary if YES, otherwise empty]

Examples:
- "YES: User wants to add remote work options and focus on senior roles"
- "NO"
- "YES: User wants to exclude contract positions and add Python skills"`,
		email.Subject, email.From, email.Body)

	c.logger.Debug("calling OpenAI for email analysis",
		"email_id", email.ID,
		"prompt_length", len(prompt),
		"max_tokens", 150,
	)

	// TODO: Need to allocate cost for this call to the user
	response, err := c.callOpenAI(ctx, prompt, 150)
	if err != nil {
		c.logger.Error("OpenAI analysis failed",
			"email_id", email.ID,
			"error", err,
		)
		return false, "", fmt.Errorf("failed to analyze email: %w", err)
	}

	c.logger.Debug("received OpenAI analysis response",
		"email_id", email.ID,
		"response", response,
		"response_length", len(response),
	)

	// Parse the response
	parts := strings.SplitN(response, ":", 2)
	if len(parts) < 1 {
		c.logger.Error("invalid OpenAI response format",
			"email_id", email.ID,
			"response", response,
		)
		return false, "", fmt.Errorf("invalid response format: %s", response)
	}

	decision := strings.TrimSpace(parts[0])
	summary := ""
	if len(parts) > 1 {
		summary = strings.TrimSpace(parts[1])
	}

	isRequest := strings.ToUpper(decision) == "YES"

	c.logger.Info("email analysis completed",
		"email_id", email.ID,
		"is_search_request", isRequest,
		"decision", decision,
		"summary", summary,
	)

	return isRequest, summary, nil
}

// UpdateStatement merges email adjustment requests with the current user statement.
func (c *openAIClient) UpdateStatement(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
	c.logger.Debug("updating user statement with OpenAI",
		"current_length", len(currentStatement),
		"email_length", len(emailContent),
		"adjustment_request", adjustmentRequest,
	)

	prompt := fmt.Sprintf(`Update the user's job search statement based on their email request.

Current Statement: %s

Email Content: %s

Adjustment Request: %s

Instructions:
- Update the statement to incorporate the requested changes
- The email request should override any conflicting preferences in the current statement
- Maintain the first-person perspective
- Keep the statement concise and professional
- Focus on concrete job search criteria (location, role type, skills, experience level, etc.)
- Ensure the updated statement is coherent and well-structured

Return only the updated statement without any additional explanation.`,
		currentStatement, emailContent, adjustmentRequest)

	c.logger.Debug("calling OpenAI for statement update",
		"prompt_length", len(prompt),
		"max_tokens", 500,
	)

	updatedStatement, err := c.callOpenAI(ctx, prompt, 500)
	if err != nil {
		c.logger.Error("OpenAI statement update failed",
			"error", err,
		)
		return "", fmt.Errorf("failed to update statement: %w", err)
	}

	result := strings.TrimSpace(updatedStatement)
	c.logger.Info("statement update completed",
		"old_length", len(currentStatement),
		"new_length", len(result),
		"change_ratio", float64(len(result))/float64(max(len(currentStatement), 1)),
	)

	return result, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// callOpenAI makes a request to the OpenAI API.
func (c *openAIClient) callOpenAI(ctx context.Context, prompt string, maxTokens int) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens: maxTokens,
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("openai api call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from openai")
	}

	return resp.Choices[0].Message.Content, nil
}

// UserService defines the interface for user operations.
// This interface abstracts user data access for testing and modularity.
type UserService interface {
	// GetUserByEmail retrieves a user by their email address.
	// Returns the user, a boolean indicating if found, and any error that occurred.
	GetUserByEmail(ctx context.Context, email string) (*internal.User, bool, error)

	// SetUserStatement updates a user's search statement.
	// Takes the user ID and new statement as parameters.
	// Returns an error if the update fails.
	SetUserStatement(ctx context.Context, userID, statement string) error

	// CreateStatementVersionFromFeedback creates a new statement version from application feedback.
	// Uses the versioning system to properly track changes and maintain audit trail.
	CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error)
	// CreateStatementVersionFromInboxFeedback creates a new statement version from inbox email feedback.
	// Uses inbox-specific processing with raw email content and adjustment requests.
	CreateStatementVersionFromInboxFeedback(ctx context.Context, userID, emailContent, adjustmentRequest, userEmail, promptFile string) (*internal.UserStatementVersion, error)
}

// firestoreUserService implements UserService using Firestore.
type firestoreUserService struct {
	fsClient *internal.FirestoreClient
}

func (f *firestoreUserService) GetUserByEmail(ctx context.Context, email string) (*internal.User, bool, error) {
	return internal.GetUserByEmail(ctx, f.fsClient, email)
}

func (f *firestoreUserService) SetUserStatement(ctx context.Context, userID, statement string) error {
	return internal.SetUserStatement(ctx, f.fsClient, userID, statement)
}

func (f *firestoreUserService) CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
	return internal.EnhanceStatementWithAppFeedback(ctx, f.fsClient, userID, feedbackData, promptFile)
}

func (f *firestoreUserService) CreateStatementVersionFromInboxFeedback(ctx context.Context, userID, emailContent, adjustmentRequest, userEmail, promptFile string) (*internal.UserStatementVersion, error) {
	return internal.EnhanceStatementWithInboxFeedback(ctx, f.fsClient, userID, emailContent, adjustmentRequest, userEmail, promptFile)
}

// SearchAdjustmentEmailData represents the data used in search adjustment notification emails.
// This struct contains all the information needed to generate a comprehensive notification
// about a user's search statement update via email feedback.
type SearchAdjustmentEmailData struct {
	BaseURL           string    // Base URL of the application
	UserID            string    // Unique identifier of the user
	UserEmail         string    // Email address of the user
	UserName          string    // Display name of the user
	OriginalRequest   string    // Original email content from the user
	PreviousStatement string    // User's statement before the update
	UpdatedStatement  string    // User's statement after the update
	AdjustmentSummary string    // Summary of what changes were requested
	ProcessedAt       time.Time // When the adjustment was processed
}

// InboxFeedbackHandler handles incoming emails for search adjustment requests.
// It processes emails to detect search adjustment requests, updates user statements,
// and sends notifications when changes are made.
type InboxFeedbackHandler struct {
	openAIClient      OpenAIClient // Client for OpenAI API operations
	userService       UserService  // Service for user data operations
	mailer            smtp.Mailer  // Mailer for sending notifications
	notificationEmail string       // Email address to send notifications to
	logger            *slog.Logger // Logger for structured logging
}

// NewInboxFeedbackHandler creates a new inbox feedback handler.
// It sets up the handler with the necessary dependencies for processing emails,
// updating user statements, and sending notifications.
// Returns a configured InboxFeedbackHandler ready for use.
func NewInboxFeedbackHandler(openAIClient OpenAIClient, fsClient *internal.FirestoreClient, mailer smtp.Mailer, notificationEmail string, logger *slog.Logger) *InboxFeedbackHandler {
	logger = logger.With("component", "InboxFeedbackHandler", "pkg", "feedback")
	return &InboxFeedbackHandler{
		openAIClient:      openAIClient,
		userService:       &firestoreUserService{fsClient: fsClient},
		mailer:            mailer,
		notificationEmail: notificationEmail,
		logger:            logger,
	}
}

// NewInboxFeedbackHandlerWithServices creates a new inbox feedback handler with custom services.
// This constructor is primarily intended for testing, allowing injection of mock services.
// Takes custom implementations of OpenAIClient, UserService, and other dependencies.
// Returns a configured InboxFeedbackHandler with the provided services.
func NewInboxFeedbackHandlerWithServices(openAIClient OpenAIClient, userService UserService, mailer smtp.Mailer, notificationEmail string, logger *slog.Logger) *InboxFeedbackHandler {
	logger = logger.With("component", "InboxFeedbackHandlerWithServices", "pkg", "feedback")
	return &InboxFeedbackHandler{
		openAIClient:      openAIClient,
		userService:       userService,
		mailer:            mailer,
		notificationEmail: notificationEmail,
		logger:            logger,
	}
}

// HandleEmail processes an incoming email for search adjustment requests.
// This is the main entry point for email processing. It analyzes the email content,
// determines if it contains search adjustment requests, and if so, processes the
// adjustments and sends notifications.
// Returns a HandlingResult indicating what was done and any error that occurred.
func (h *InboxFeedbackHandler) HandleEmail(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
	h.logger.Info("processing email for search adjustments",
		"email_id", email.ID,
		"from", email.From,
		"subject", email.Subject)

	// Check if this email contains a search adjustment request
	isRequest, adjustmentSummary, err := h.analyzeEmail(ctx, email)
	if err != nil {
		h.logger.Error("failed to analyze email", "error", err, "email_id", email.ID)
		return nil, fmt.Errorf("failed to analyze email: %w", err)
	}

	if !isRequest {
		h.logger.Debug("email does not contain search adjustment request", "email_id", email.ID)
		return &inbox.HandlingResult{
			Handled:  false,
			Summary:  "",
			Metadata: nil,
		}, nil
	}

	h.logger.Info("search adjustment request detected",
		"email_id", email.ID,
		"summary", adjustmentSummary)

	// Process the search adjustment request
	if err := h.processSearchAdjustment(ctx, email, adjustmentSummary); err != nil {
		h.logger.Error("failed to process search adjustment",
			"error", err,
			"email_id", email.ID,
			"from", email.From)
		return nil, fmt.Errorf("failed to process search adjustment: %w", err)
	}

	h.logger.Info("search adjustment processed successfully",
		"email_id", email.ID,
		"from", email.From)

	return &inbox.HandlingResult{
		Handled: true,
		Summary: fmt.Sprintf("Updated search preferences: %s", adjustmentSummary),
		Metadata: map[string]interface{}{
			"user_email":         email.From,
			"adjustment_summary": adjustmentSummary,
			"email_subject":      email.Subject,
		},
	}, nil
}

// analyzeEmail determines if an email contains search adjustment requests.
func (h *InboxFeedbackHandler) analyzeEmail(ctx context.Context, email *inbox.Email) (bool, string, error) {
	return h.openAIClient.IsSearchAdjustmentRequest(ctx, email)
}

// extractUserEmail extracts and validates the user's email address from an email.
// It handles both "Name <email@domain.com>" and plain "email@domain.com" formats.
// Returns the extracted email address or an error if extraction fails.
func (h *InboxFeedbackHandler) extractUserEmail(email *inbox.Email) (string, error) {
	userEmail := extractEmailAddress(email.From)
	if userEmail == "" {
		return "", fmt.Errorf("could not extract email address from: %s", email.From)
	}
	return userEmail, nil
}

// lookupUser retrieves a user by their email address.
// Returns the user if found, or an error if the user doesn't exist or lookup fails.
func (h *InboxFeedbackHandler) lookupUser(ctx context.Context, email string) (*internal.User, error) {
	user, found, err := h.userService.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	if !found {
		h.logger.Warn("user not found for email", "email", email)
		return nil, fmt.Errorf("user not found for email: %s", email)
	}

	return user, nil
}

// buildUserName constructs a display name for the user.
// It uses the Name field if available, falls back to FirstName + LastName,
// or returns "Unknown" if no name information is available.
func (h *InboxFeedbackHandler) buildUserName(user *internal.User) string {
	if user.Name != "" {
		return user.Name
	}

	if user.FirstName != "" || user.LastName != "" {
		return strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	return "Unknown"
}

// processSearchAdjustment processes a confirmed search adjustment request.
// It extracts the user email, looks up the user, and updates their statement.
// Returns an error if any step in the process fails.
func (h *InboxFeedbackHandler) processSearchAdjustment(ctx context.Context, email *inbox.Email, adjustmentRequest string) error {
	h.logger.Debug("starting search adjustment processing",
		"email_id", email.ID,
		"adjustment_request", adjustmentRequest,
	)

	// Extract email address from the From field
	userEmail, err := h.extractUserEmail(email)
	if err != nil {
		h.logger.Error("failed to extract user email",
			"email_id", email.ID,
			"from", email.From,
			"error", err,
		)
		return fmt.Errorf("failed to extract user email: %w", err)
	}

	h.logger.Debug("extracted user email",
		"email_id", email.ID,
		"user_email", userEmail,
	)

	// Get the user by email
	user, err := h.lookupUser(ctx, userEmail)
	if err != nil {
		h.logger.Error("failed to lookup user",
			"email_id", email.ID,
			"user_email", userEmail,
			"error", err,
		)
		return fmt.Errorf("failed to lookup user: %w", err)
	}

	h.logger.Info("found user for search adjustment",
		"email_id", email.ID,
		"user_id", user.ID,
		"user_email", user.Email,
		"user_name", h.buildUserName(user),
	)

	// Update the user's statement
	if err := h.updateUserStatement(ctx, user, email.Body, adjustmentRequest); err != nil {
		h.logger.Error("failed to update user statement",
			"email_id", email.ID,
			"user_id", user.ID,
			"error", err,
		)
		return fmt.Errorf("failed to update user statement: %w", err)
	}

	h.logger.Info("search adjustment processing completed successfully",
		"email_id", email.ID,
		"user_id", user.ID,
	)

	return nil
}


// updateUserStatement updates the user's statement based on the email request.
func (h *InboxFeedbackHandler) updateUserStatement(ctx context.Context, user *internal.User, emailContent, adjustmentRequest string) error {
	h.logger.Debug("starting user statement update",
		"user_id", user.ID,
		"user_email", user.Email,
		"email_content_length", len(emailContent),
		"adjustment_request", adjustmentRequest,
	)

	// Get current statement for logging and notification purposes
	currentStatement := ""
	if user.Statement != nil {
		currentStatement = *user.Statement
	}

	h.logger.Debug("current user statement",
		"user_id", user.ID,
		"current_length", len(currentStatement),
		"has_statement", user.Statement != nil,
	)

	h.logger.Debug("creating statement version from inbox email feedback",
		"user_id", user.ID,
		"email_content_length", len(emailContent),
		"adjustment_request", adjustmentRequest,
	)

	// Use inbox-specific versioning system to create new statement version
	newVersion, err := h.userService.CreateStatementVersionFromInboxFeedback(ctx, user.ID, emailContent, adjustmentRequest, user.Email, "inbox-feedback.v001.txt")
	if err != nil {
		h.logger.Error("failed to create statement version from email feedback",
			"user_id", user.ID,
			"error", err,
		)
		return fmt.Errorf("failed to create statement version from feedback: %w", err)
	}

	h.logger.Info("statement version created from email feedback",
		"user_id", user.ID,
		"version_id", newVersion.ID,
		"version_number", newVersion.VersionNumber,
		"old_length", len(currentStatement),
		"new_length", len(newVersion.StatementText),
		"first_100_chars", func() string {
			if len(newVersion.StatementText) > 100 {
				return newVersion.StatementText[:100] + "..."
			}
			return newVersion.StatementText
		}())

	// Send notification email if enabled
	h.logger.Debug("attempting to send search adjustment notification",
		"user_id", user.ID,
		"notification_enabled", os.Getenv("SEND_FEEDBACK_ADJUSTMENTS") == "true",
		"has_notification_email", h.notificationEmail != "",
		"has_mailer", h.mailer != nil,
	)

	if err := h.sendSearchAdjustmentNotification(ctx, user, emailContent, currentStatement, newVersion.StatementText, adjustmentRequest); err != nil {
		// Log the error but don't fail the processing
		h.logger.Warn("failed to send search adjustment notification",
			"error", err,
			"user_id", user.ID,
			"email", user.Email)
	}

	return nil
}

// getMapKeys returns the keys of a map as a slice for logging purposes
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// sendSearchAdjustmentNotification sends an email notification about the search adjustment.
func (h *InboxFeedbackHandler) sendSearchAdjustmentNotification(ctx context.Context, user *internal.User, emailContent, previousStatement, updatedStatement, adjustmentSummary string) error {
	// Check if notifications are enabled
	if os.Getenv("SEND_FEEDBACK_ADJUSTMENTS") != "true" {
		h.logger.Info("search adjustment notifications disabled - SEND_FEEDBACK_ADJUSTMENTS not set to true")
		return nil
	}

	// Check if we have a notification email configured
	if h.notificationEmail == "" {
		h.logger.Info("no notification email configured - ADMIN_EMAIL not set")
		return nil
	}

	// Check if we have a mailer
	if h.mailer == nil {
		h.logger.Info("no mailer configured")
		return nil
	}

	// Prepare user name
	userName := h.buildUserName(user)

	// Prepare email data
	emailData := SearchAdjustmentEmailData{
		BaseURL:           os.Getenv("BASE_URL"),
		UserID:            user.ID,
		UserEmail:         user.Email,
		UserName:          userName,
		OriginalRequest:   emailContent,
		PreviousStatement: previousStatement,
		UpdatedStatement:  updatedStatement,
		AdjustmentSummary: adjustmentSummary,
		ProcessedAt:       time.Now(),
	}

	h.logger.Debug(internal.Cyan("Sending search adjustment notification to %s", h.notificationEmail))

	// Send the notification email
	err := h.mailer.SendFromTemplate(h.notificationEmail, emailData, "search-adjustment-notification.tmpl")
	if err != nil {
		return fmt.Errorf("failed to send notification email: %w", err)
	}

	h.logger.Info("search adjustment notification sent successfully",
		"user_id", user.ID,
		"user_email", user.Email,
		"notification_email", h.notificationEmail)

	return nil
}

// extractEmailAddress extracts the email address from a "Name <email@domain.com>" format.
func extractEmailAddress(fromField string) string {
	// Handle "Name <email@domain.com>" format
	if strings.Contains(fromField, "<") && strings.Contains(fromField, ">") {
		start := strings.Index(fromField, "<")
		end := strings.Index(fromField, ">")
		if start < end {
			return strings.TrimSpace(fromField[start+1 : end])
		}
	}

	// Handle plain email address
	return strings.TrimSpace(fromField)
}
