package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/smtp"
)

// Mailer defines the interface for sending emails.
// This interface allows for mocking in tests and abstracts the SMTP implementation.
type Mailer interface {
	// Send sends an email with the specified parameters
	Send(to, subject, plainBody, htmlBody string) error
}

// ResponseSender handles sending email responses to users.
type ResponseSender struct {
	mailer smtp.Mailer
	logger *slog.Logger
}

// NewResponseSender creates a new response sender instance.
func NewResponseSender(mailer smtp.Mailer, logger *slog.Logger) *ResponseSender {
	return &ResponseSender{
		mailer: mailer,
		logger: logger,
	}
}

// SendResponse sends a response email to the user.
func (rs *ResponseSender) SendResponse(ctx context.Context, response *EmailResponse, recipientEmail string) error {
	if response == nil {
		return fmt.Errorf("response cannot be nil")
	}

	if recipientEmail == "" {
		return fmt.Errorf("recipient email cannot be empty")
	}

	// Validate the response
	if err := response.IsValid(); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	// Validate and clean recipient email
	cleanRecipient, err := rs.validateAndCleanEmail(recipientEmail)
	if err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}

	// Log the sending attempt
	rs.logger.Info("sending email response",
		"recipient", cleanRecipient,
		"subject", response.Subject,
		"type", response.Type.String(),
		"handled_count", response.HandledCount(),
		"escalated_count", response.EscalatedCount(),
	)

	// Send the email using the mailer
	// This call is not tested as it depends on third-party SMTP service
	err = rs.mailer.Send(cleanRecipient, response.Subject, response.Body, response.HTMLBody)
	if err != nil {
		rs.logger.Error("failed to send email response",
			"error", err,
			"recipient", cleanRecipient,
			"subject", response.Subject,
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	rs.logger.Info("email response sent successfully",
		"recipient", cleanRecipient,
		"subject", response.Subject,
		"type", response.Type.String(),
	)

	return nil
}

// validateAndCleanEmail validates and cleans the recipient email address
func (rs *ResponseSender) validateAndCleanEmail(email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email cannot be empty")
	}

	// Clean the email address
	cleanEmail := strings.TrimSpace(email)
	
	// Handle "Name <email@domain.com>" format
	if strings.Contains(cleanEmail, "<") || strings.Contains(cleanEmail, ">") {
		start := strings.Index(cleanEmail, "<")
		end := strings.Index(cleanEmail, ">")
		if start == -1 || end == -1 || start >= end {
			return "", fmt.Errorf("malformed email format")
		}
		cleanEmail = strings.TrimSpace(cleanEmail[start+1 : end])
	}

	// Basic email validation
	if !strings.Contains(cleanEmail, "@") {
		return "", fmt.Errorf("invalid email format")
	}

	parts := strings.Split(cleanEmail, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email format")
	}

	if parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid email format")
	}

	return cleanEmail, nil
}

// SendEscalation sends an escalation email to the admin.
func (rs *ResponseSender) SendEscalation(ctx context.Context, originalEmail *Email, escalatedItems []string, adminEmail string) error {
	if originalEmail == nil {
		return fmt.Errorf("original email cannot be nil")
	}

	if adminEmail == "" {
		return fmt.Errorf("admin email cannot be empty")
	}

	if len(escalatedItems) == 0 {
		return fmt.Errorf("no escalated items provided")
	}

	// Validate and clean admin email
	cleanAdminEmail, err := rs.validateAndCleanEmail(adminEmail)
	if err != nil {
		return fmt.Errorf("invalid admin email: %w", err)
	}

	// Build escalation email
	subject := fmt.Sprintf("Inbox Escalation: %s", originalEmail.Subject)
	body := rs.buildEscalationBody(originalEmail, escalatedItems)
	htmlBody := rs.convertToHTML(body)

	rs.logger.Info("sending escalation email",
		"admin", cleanAdminEmail,
		"original_from", originalEmail.From,
		"original_subject", originalEmail.Subject,
		"escalated_items", len(escalatedItems),
	)

	// Send the escalation email
	// This call is not tested as it depends on third-party SMTP service
	err = rs.mailer.Send(cleanAdminEmail, subject, body, htmlBody)
	if err != nil {
		rs.logger.Error("failed to send escalation email",
			"error", err,
			"admin", cleanAdminEmail,
			"original_from", originalEmail.From,
		)
		return fmt.Errorf("failed to send escalation email: %w", err)
	}

	rs.logger.Info("escalation email sent successfully",
		"admin", cleanAdminEmail,
		"original_from", originalEmail.From,
	)

	return nil
}

// buildEscalationBody creates the body for escalation emails
func (rs *ResponseSender) buildEscalationBody(originalEmail *Email, escalatedItems []string) string {
	var body strings.Builder

	body.WriteString("An email from a user could not be fully processed and requires manual attention.\n\n")
	
	body.WriteString("ORIGINAL EMAIL:\n")
	body.WriteString(fmt.Sprintf("From: %s\n", originalEmail.From))
	body.WriteString(fmt.Sprintf("Subject: %s\n", originalEmail.Subject))
	body.WriteString(fmt.Sprintf("Date: %s\n", originalEmail.Date.Format("2006-01-02 15:04:05")))
	body.WriteString("\n")
	
	body.WriteString("Body:\n")
	body.WriteString(originalEmail.Body)
	body.WriteString("\n\n")
	
	body.WriteString("ESCALATED ITEMS:\n")
	for i, item := range escalatedItems {
		body.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	body.WriteString("\n")
	
	body.WriteString("Please review this email and take appropriate action.\n\n")
	body.WriteString("Best regards,\n")
	body.WriteString("Talent Rodeo Inbox System")

	return body.String()
}

// convertToHTML converts plain text to basic HTML
func (rs *ResponseSender) convertToHTML(plainText string) string {
	if plainText == "" {
		return ""
	}

	// Simple conversion: replace newlines with <br> tags and wrap in HTML
	html := strings.ReplaceAll(plainText, "\n", "<br>\n")
	return fmt.Sprintf("<html><body><pre>%s</pre></body></html>", html)
}