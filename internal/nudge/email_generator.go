package nudge

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"talent-rodeo/internal"
)

// EmailGenerator handles generation of nudge emails using dependency injection
type EmailGenerator struct {
	templateReader  TemplateReader
	promptProcessor PromptProcessor
}

// NewEmailGenerator creates a new email generator with dependencies
func NewEmailGenerator(templateReader TemplateReader, promptProcessor PromptProcessor) *EmailGenerator {
	return &EmailGenerator{
		templateReader:  templateReader,
		promptProcessor: promptProcessor,
	}
}

// GenerateEmailContent generates a personalized nudge email using the injected dependencies.
// It decides whether to use a static template or an LLM based on the nudge type.
func (eg *EmailGenerator) GenerateEmailContent(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, user *internal.User, nudgeType NudgeType, emailData *NudgeEmailData) (*GeneratedEmail, error) {
	if templateName := nudgeType.EmailTemplatePath(); templateName != "" {
		return eg.generateFromTemplate(ctx, nudgeType, emailData)
	}
	return eg.generateFromLLM(ctx, fs, logger, user, nudgeType, emailData)
}

func (eg *EmailGenerator) generateFromLLM(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, user *internal.User, nudgeType NudgeType, emailData *NudgeEmailData) (*GeneratedEmail, error) {
	// Get the appropriate prompt template
	promptTemplate, err := eg.templateReader.ReadTemplate(nudgeType)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt template: %w", err)
	}

	// Generate the prompt with user data
	prompt, err := eg.promptProcessor.GeneratePrompt(promptTemplate, emailData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Call LLM to generate email content
	response, err := eg.promptProcessor.ProcessPrompt(ctx, fs, logger, user.ID, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to process prompt: %w", err)
	}

	// Parse the LLM response to extract subject and body
	email, err := parseLLMResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse email response: %w", err)
	}

	// Validate the generated email
	if err := eg.ValidateEmail(email); err != nil {
		return nil, fmt.Errorf("generated email validation failed: %w", err)
	}

	return email, nil
}

func (eg *EmailGenerator) generateFromTemplate(ctx context.Context, nudgeType NudgeType, emailData *NudgeEmailData) (*GeneratedEmail, error) {
	t, err := GetTemplate(nudgeType)
	if err != nil {
		return nil, fmt.Errorf("failed to get template from cache: %w", err)
	}

	templatePath := nudgeType.EmailTemplatePath()
	var subject, plainBody, htmlBody bytes.Buffer

	if t.Lookup("subject") == nil {
		return nil, fmt.Errorf("template %s is missing {{define \"subject\"}}", templatePath)
	}
	if err := t.ExecuteTemplate(&subject, "subject", emailData); err != nil {
		return nil, fmt.Errorf("failed to execute subject template: %w", err)
	}

	if t.Lookup("plainBody") != nil {
		if err := t.ExecuteTemplate(&plainBody, "plainBody", emailData); err != nil {
			return nil, fmt.Errorf("failed to execute plainBody template: %w", err)
		}
	}

	if t.Lookup("htmlBody") != nil {
		if err := t.ExecuteTemplate(&htmlBody, "htmlBody", emailData); err != nil {
			return nil, fmt.Errorf("failed to execute htmlBody template: %w", err)
		}
	}

	email := &GeneratedEmail{
		Subject:   strings.TrimSpace(subject.String()),
		PlainBody: strings.TrimSpace(plainBody.String()),
		HTMLBody:  strings.TrimSpace(htmlBody.String()),
	}

	if err := eg.ValidateEmail(email); err != nil {
		return nil, fmt.Errorf("generated email validation failed: %w", err)
	}

	return email, nil
}

// BuildEmailData creates the data structure for email template generation
func (eg *EmailGenerator) BuildEmailData(user *internal.User, nudgeType NudgeType, additionalData map[string]any) *NudgeEmailData {
	data := &NudgeEmailData{
		User:            user,
		NudgeType:       nudgeType.String(),
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		FullName:        user.Name,
		LastAccessed:    user.LastAccessedAt,
		LastStateChange: user.StatusUpdatedAt,
	}

	if nudgeType == NudgeTypeFiveStarMatches {
		if additionalData != nil {
			if matches, ok := additionalData["matches"].([]FiveStarMatchData); ok {
				data.Matches = matches
			}
		}
	}

	return data
}

// ValidateEmail performs basic validation on generated email content
func (eg *EmailGenerator) ValidateEmail(email *GeneratedEmail) error {
	if email == nil {
		return fmt.Errorf("email is nil")
	}

	if strings.TrimSpace(email.Subject) == "" {
		return fmt.Errorf("email subject is empty")
	}

	if strings.TrimSpace(email.PlainBody) == "" && strings.TrimSpace(email.HTMLBody) == "" {
		return fmt.Errorf("email body (both plain and HTML) is empty")
	}

	// Check subject line length (reasonable limits for email clients)
	if len(email.Subject) > 200 {
		return fmt.Errorf("email subject too long (%d characters, max 200)", len(email.Subject))
	}

	// Use plain body for checks if available, otherwise HTML body.
	bodyForCheck := email.PlainBody
	if email.PlainBody == "" {
		bodyForCheck = email.HTMLBody
	}

	// Check body length (reasonable limits)
	if len(bodyForCheck) > 5000 {
		return fmt.Errorf("email body too long (%d characters, max 5000)", len(bodyForCheck))
	}

	// Check for required signatures/content
	if !strings.Contains(bodyForCheck, "Shirely") {
		return fmt.Errorf("email body missing Shirley signature")
	}

	if !strings.Contains(bodyForCheck, "Talent Rodeo") {
		return fmt.Errorf("email body missing Talent Rodeo branding")
	}

	return nil
}

// parseLLMResponse extracts subject and body from LLM response
func parseLLMResponse(response string) (*GeneratedEmail, error) {
	// Use regex to extract email content from XML-like tags
	emailContentPattern := regexp.MustCompile(`(?s)<email_content>(.*?)</email_content>`)
	emailMatch := emailContentPattern.FindStringSubmatch(response)
	if len(emailMatch) < 2 {
		return nil, fmt.Errorf("no email_content found in LLM response")
	}

	emailContent := emailMatch[1]

	// Extract subject
	subjectPattern := regexp.MustCompile(`(?s)<subject>(.*?)</subject>`)
	subjectMatch := subjectPattern.FindStringSubmatch(emailContent)
	if len(subjectMatch) < 2 {
		return nil, fmt.Errorf("no subject found in email content")
	}

	// Extract body
	bodyPattern := regexp.MustCompile(`(?s)<body>(.*?)</body>`)
	bodyMatch := bodyPattern.FindStringSubmatch(emailContent)
	if len(bodyMatch) < 2 {
		return nil, fmt.Errorf("no body found in email content")
	}

	return &GeneratedEmail{
		Subject: strings.TrimSpace(subjectMatch[1]),
		// For LLMs, we assume the body is HTML, which can be used for plain text conversion later.
		HTMLBody: strings.TrimSpace(bodyMatch[1]),
	}, nil
}
