package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/smtp"
	"time"
)

// EscalationService handles admin notifications for unprocessed emails.
type EscalationService struct {
	adminEmail string
	mailer     smtp.Mailer
	logger     *slog.Logger
}

// NewEscalationService creates a new escalation service instance.
func NewEscalationService(adminEmail string, mailer smtp.Mailer, logger *slog.Logger) *EscalationService {
	logger = logger.With("component", "EscalationService", "pkg", "inbox")
	return &EscalationService{
		adminEmail: adminEmail,
		mailer:     mailer,
		logger:     logger,
	}
}

// SendEscalation notifies admin about unhandled email items.
func (es *EscalationService) SendEscalation(ctx context.Context, originalEmail *Email, unhandledItems []string) error {
	if originalEmail == nil {
		return fmt.Errorf("original email cannot be nil")
	}

	if es.adminEmail == "" {
		return fmt.Errorf("admin email not configured")
	}

	if len(unhandledItems) == 0 {
		return fmt.Errorf("no unhandled items provided")
	}

	// Validate admin email format
	if err := es.validateEmail(es.adminEmail); err != nil {
		return fmt.Errorf("invalid admin email: %w", err)
	}

	// Build escalation email content
	subject := es.buildEscalationSubject(originalEmail)
	body := es.buildEscalationBody(originalEmail, unhandledItems)
	htmlBody := es.convertToHTML(body)

	es.logger.Info("sending admin escalation",
		"admin", es.adminEmail,
		"original_from", originalEmail.From,
		"original_subject", originalEmail.Subject,
		"unhandled_count", len(unhandledItems),
	)

	// Send the escalation email
	// This call is not tested as it depends on third-party SMTP service
	err := es.mailer.Send(es.adminEmail, subject, body, htmlBody)
	if err != nil {
		es.logger.Error("failed to send admin escalation",
			"error", err,
			"admin", es.adminEmail,
			"original_from", originalEmail.From,
		)
		return fmt.Errorf("failed to send escalation email: %w", err)
	}

	es.logger.Info("admin escalation sent successfully",
		"admin", es.adminEmail,
		"original_from", originalEmail.From,
	)

	return nil
}

// validateEmail performs basic email validation
func (es *EscalationService) validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format")
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// buildEscalationSubject creates the subject line for escalation emails
func (es *EscalationService) buildEscalationSubject(originalEmail *Email) string {
	return fmt.Sprintf("Inbox Escalation: %s", originalEmail.Subject)
}

// buildEscalationBody creates the body content for escalation emails
func (es *EscalationService) buildEscalationBody(originalEmail *Email, unhandledItems []string) string {
	var body strings.Builder

	body.WriteString("An email from a user could not be fully processed and requires manual attention.\n\n")

	body.WriteString("ORIGINAL EMAIL DETAILS:\n")
	body.WriteString(fmt.Sprintf("From: %s\n", originalEmail.From))
	body.WriteString(fmt.Sprintf("Subject: %s\n", originalEmail.Subject))
	body.WriteString(fmt.Sprintf("Date: %s\n", originalEmail.Date.Format("2006-01-02 15:04:05")))
	body.WriteString(fmt.Sprintf("Email ID: %s\n", originalEmail.ID))
	if originalEmail.ThreadID != "" {
		body.WriteString(fmt.Sprintf("Thread ID: %s\n", originalEmail.ThreadID))
	}
	body.WriteString("\n")

	body.WriteString("EMAIL CONTENT:\n")
	body.WriteString(originalEmail.Body)
	body.WriteString("\n\n")

	body.WriteString("UNHANDLED ITEMS:\n")
	for i, item := range unhandledItems {
		body.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	body.WriteString("\n")

	body.WriteString("ACTION REQUIRED:\n")
	body.WriteString("Please review this email and take appropriate manual action.\n")
	body.WriteString("The user is expecting a response to their request.\n\n")

	body.WriteString("Best regards,\n")
	body.WriteString("Talent Rodeo Inbox Processing System\n")
	body.WriteString(fmt.Sprintf("Generated at: %s", time.Now().Format("2006-01-02 15:04:05")))

	return body.String()
}

// convertToHTML converts plain text to basic HTML for email
func (es *EscalationService) convertToHTML(plainText string) string {
	if plainText == "" {
		return ""
	}

	// Simple conversion: replace newlines with <br> tags and wrap in HTML
	html := strings.ReplaceAll(plainText, "\n", "<br>\n")
	return fmt.Sprintf("<html><body><pre style=\"font-family: monospace; white-space: pre-wrap;\">%s</pre></body></html>", html)
}

// IsConfigured returns true if the escalation service is properly configured
func (es *EscalationService) IsConfigured() bool {
	return es.adminEmail != "" && es.mailer != nil
}
