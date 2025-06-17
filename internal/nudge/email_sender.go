package nudge

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal"
	"talent-rodeo/internal/smtp"

	"github.com/getsentry/sentry-go"
)

// SMTPEmailSender implements EmailSender using SMTP
type SMTPEmailSender struct {
	smtpConfig *smtp.Config
	logger     *slog.Logger
}

// NewSMTPEmailSender creates a new SMTP email sender
func NewSMTPEmailSender(smtpConfig *smtp.Config, logger *slog.Logger) *SMTPEmailSender {
	return &SMTPEmailSender{
		smtpConfig: smtpConfig,
		logger:     logger,
	}
}

// SendEmail sends an email using SMTP
func (s *SMTPEmailSender) SendEmail(ctx context.Context, recipient string, email *GeneratedEmail, nudgeType NudgeType) error {
	mailer, err := smtp.NewMailer(
		s.smtpConfig.Host,
		s.smtpConfig.Port,
		s.smtpConfig.Username,
		s.smtpConfig.Password,
		s.smtpConfig.From,
	)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_new",
			"component":  "nudge_send_email",
		})
		s.logger.Error("failed to create SMTP mailer", "error", err, "recipient", recipient)
		return fmt.Errorf("sendEmail: smtp.NewMailer: %w", err)
	}

	err = mailer.Send(recipient, email.Subject, email.PlainBody, email.HTMLBody)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_send",
			"component":  "nudge_send_email",
		})
		s.logger.Error("failed to send nudge email", "error", err, "recipient", recipient)
		return fmt.Errorf("sendEmail: mailer.Send: %w", err)
	}

	return nil
}

// GetEmailTemplatePath returns the HTML template path for the nudge type
func GetEmailTemplatePath(nudgeType NudgeType) string {
	switch nudgeType {
	case NudgeTypeNoResume:
		return "shirely-no-resume.html"
	case NudgeTypeNoCall:
		return "shirely-no-call.html"
	case NudgeTypeFiveStarMatches:
		return "shirely-five-star-matches.html"
	default:
		return ""
	}
}
