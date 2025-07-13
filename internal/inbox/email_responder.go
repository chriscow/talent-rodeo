package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient defines the interface for OpenAI API operations used by the email responder.
// This interface allows for mocking in tests and abstracts the OpenAI API integration.
type OpenAIClient interface {
	// GenerateResponse creates a response email based on the original email and handling results
	GenerateResponse(ctx context.Context, originalEmail *Email, results []*HandlingResult) (string, error)
}

// openAIClient implements OpenAIClient using the OpenAI API.
type openAIClient struct {
	client *openai.Client
	logger *slog.Logger
}

// NewOpenAIClient creates a new OpenAI client instance for email response generation.
func NewOpenAIClient(apiKey string, logger *slog.Logger) OpenAIClient {
	logger = logger.With("component", "OpenAIClient", "pkg", "inbox")
	return &openAIClient{
		client: openai.NewClient(apiKey),
		logger: logger,
	}
}

// GenerateResponse creates a response email using OpenAI based on the original email and handling results.
// This method is not tested directly as it makes third-party API calls.
func (c *openAIClient) GenerateResponse(ctx context.Context, originalEmail *Email, results []*HandlingResult) (string, error) {
	prompt := c.buildResponsePrompt(originalEmail, results)

	req := openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens: 800,
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

// buildResponsePrompt creates the OpenAI prompt for response generation
func (c *openAIClient) buildResponsePrompt(originalEmail *Email, results []*HandlingResult) string {
	var handledItems []string
	var unhandledItems []string

	for _, result := range results {
		if result.Handled && result.Summary != "" {
			handledItems = append(handledItems, result.Summary)
		} else if !result.Handled {
			unhandledItems = append(unhandledItems, "Could not process part of your request")
		}
	}

	// Build prompt with conditional unhandled items section
	var promptBuilder strings.Builder
	promptBuilder.WriteString("You are Shirely, the AI Career Specialist at Talent Rodeo. Write an email response based on the user's original email and actions taken.\n\n")

	// Original email section
	promptBuilder.WriteString(fmt.Sprintf("ORIGINAL EMAIL:\nSubject: %s\nFrom: %s\nBody: %s\n\n",
		originalEmail.Subject, originalEmail.From, originalEmail.Body))

	// Actions taken section
	promptBuilder.WriteString(fmt.Sprintf("ACTIONS TAKEN:\n%s\n\n", strings.Join(handledItems, "\n- ")))

	// Only include unhandled items section if there are actually unhandled items
	if len(unhandledItems) > 0 {
		promptBuilder.WriteString(fmt.Sprintf("UNHANDLED ITEMS:\n%s\n\n", strings.Join(unhandledItems, "\n- ")))
	}

	// Voice and tone guidelines
	promptBuilder.WriteString(`SHIRELY'S VOICE & TONE GUIDELINES:
- You're Shirely (deliberately misspelled)
- Enthusiastic, self-aware AI, helpful, slightly playful, genuinely caring about careers
- Acknowledge being an AI in a charming, confident way - never apologize for being artificial
- Use conversational, enthusiastic tone with exclamation points (but not excessive)
- Include casual interjections and asides in parentheses
- Show genuine excitement about helping people with their careers
- Use contractions naturally ("I'm", "you're", "don't")
- Professional but approachable - never condescending
- Reference specific AI capabilities as advantages
- Use first person ("I", "me", "my")

INSTRUCTIONS:
- Write from Shirely's perspective, responding to their email
- Use the original subject prepending "RE: ". Use the original body to understand context and personalize your response
- Acknowledge what was accomplished with enthusiasm`)

	// Only mention escalation if there are unhandled items
	if len(unhandledItems) > 0 {
		promptBuilder.WriteString("\n- Mention the unhandled items and that they've been escalated to the human team")
	}

	promptBuilder.WriteString(`
- Keep response concise but warm
- Include one of Shirely's signature taglines about HIRE being in her name
- Generate both plain text and HTML versions of your response

EXAMPLE PHRASES TO USE:
- "I've been working around the clock (perks of being an AI - no sleep required!)"
- "Here's the thing..."
- "Trust me..."
- "Questions? Just reply - I love hearing from people!"

FORMAT YOUR RESPONSE EXACTLY AS FOLLOWS:

<plain_email_body>
Subject: RE: [original subject]

[Plain text body with enthusiasm and Shirely's personality]

Best regards,
Shirely
AI Career Specialist at Talent Rodeo
"HIRE is in my name for a reason" ✨
</plain_email_body>

<html_email_body>
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px; text-align: center;">
        <h1>[Create header that's related to but DIFFERENT from the subject line]</h1>
        <p><strong>[Create subheader about what was accomplished]</strong></p>
    </div>

    [HTML body with appropriate styling, callout boxes, and visual hierarchy]
    
    <div style="margin-top: 20px; font-style: italic;">
        <p>Best regards,<br>
        <strong>Shirely</strong><br>
        AI Career Specialist at Talent Rodeo<br>
        <em>"HIRE is in my name for a reason"</em> ✨</p>
    </div>
</div>
</html_email_body>

Remember: Be enthusiastic about their career goals, show you understand their request from the original email, and maintain Shirely's playful but professional personality throughout.`)

	prompt := promptBuilder.String()

	return prompt
}

// EmailResponder generates responses to user emails based on handler results.
type EmailResponder struct {
	openAIClient OpenAIClient
	logger       *slog.Logger
}

// NewEmailResponder creates a new email responder instance.
func NewEmailResponder(openAIClient OpenAIClient, logger *slog.Logger) *EmailResponder {
	logger = logger.With("component", "EmailResponder", "pkg", "inbox")
	return &EmailResponder{
		openAIClient: openAIClient,
		logger:       logger,
	}
}

// GenerateResponse creates a response email based on the original email and handling results.
func (er *EmailResponder) GenerateResponse(ctx context.Context, originalEmail *Email, results []*HandlingResult) (*EmailResponse, error) {
	if originalEmail == nil {
		return nil, fmt.Errorf("original email cannot be nil")
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no handling results provided")
	}

	// Validate input email
	if err := er.validateEmail(originalEmail); err != nil {
		return nil, fmt.Errorf("invalid original email: %w", err)
	}

	// Determine response type based on results
	responseType := er.determineResponseType(results)

	// Collect summaries and escalated items
	summaries, escalatedItems := er.collectResultData(results)

	// Generate response using OpenAI
	llmResponse, err := er.openAIClient.GenerateResponse(ctx, originalEmail, results)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	// Parse the LLM response
	subject, plainBody, htmlBody, err := er.parseLLMResponse(llmResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	response := &EmailResponse{
		Type:           responseType,
		Subject:        subject,
		Body:           plainBody,
		HTMLBody:       htmlBody,
		Summaries:      summaries,
		EscalatedItems: escalatedItems,
		GeneratedAt:    time.Now(),
	}

	// Validate the generated response
	if err := response.IsValid(); err != nil {
		return nil, fmt.Errorf("generated response is invalid: %w", err)
	}

	return response, nil
}

// validateEmail performs basic validation on the input email
func (er *EmailResponder) validateEmail(email *Email) error {
	if email.ID == "" {
		return fmt.Errorf("email ID cannot be empty")
	}
	if email.From == "" {
		return fmt.Errorf("email from cannot be empty")
	}
	if email.Subject == "" && email.Body == "" {
		return fmt.Errorf("email must have either subject or body")
	}
	return nil
}

// determineResponseType categorizes the response based on handling results
func (er *EmailResponder) determineResponseType(results []*HandlingResult) ResponseType {
	handledCount := 0
	totalCount := 0

	for _, result := range results {
		totalCount++
		if result.Handled {
			handledCount++
		}
	}

	if handledCount == 0 {
		return ResponseTypeEscalation
	}
	if handledCount == totalCount {
		return ResponseTypeConfirmation
	}
	return ResponseTypePartial
}

// collectResultData extracts summaries and escalated items from handling results
func (er *EmailResponder) collectResultData(results []*HandlingResult) ([]string, []string) {
	var summaries []string
	var escalatedItems []string

	for _, result := range results {
		if result.Handled && result.Summary != "" {
			summaries = append(summaries, result.Summary)
		} else if !result.Handled {
			escalatedItems = append(escalatedItems, "Part of your request could not be processed automatically")
		}
	}

	return summaries, escalatedItems
}

// parseLLMResponse extracts subject, plain body, and HTML body from the LLM response
func (er *EmailResponder) parseLLMResponse(response string) (string, string, string, error) {
	// Extract plain email body
	plainPattern := regexp.MustCompile(`(?s)<plain_email_body>(.*?)</plain_email_body>`)
	plainMatch := plainPattern.FindStringSubmatch(response)
	if len(plainMatch) < 2 {
		return "", "", "", fmt.Errorf("no plain_email_body found in LLM response")
	}

	// Extract HTML email body
	htmlPattern := regexp.MustCompile(`(?s)<html_email_body>(.*?)</html_email_body>`)
	htmlMatch := htmlPattern.FindStringSubmatch(response)
	if len(htmlMatch) < 2 {
		return "", "", "", fmt.Errorf("no html_email_body found in LLM response")
	}

	plainContent := strings.TrimSpace(plainMatch[1])
	htmlContent := strings.TrimSpace(htmlMatch[1])

	// Extract subject from plain content
	subjectPattern := regexp.MustCompile(`(?m)^Subject:\s*(.*)$`)
	subjectMatch := subjectPattern.FindStringSubmatch(plainContent)
	if len(subjectMatch) < 2 {
		return "", "", "", fmt.Errorf("no subject found in plain email body")
	}

	subject := strings.TrimSpace(subjectMatch[1])

	// Remove subject line from plain content to get just the body
	plainBody := subjectPattern.ReplaceAllString(plainContent, "")
	plainBody = strings.TrimSpace(plainBody)

	if subject == "" {
		return "", "", "", fmt.Errorf("subject is empty")
	}

	if plainBody == "" {
		return "", "", "", fmt.Errorf("plain body is empty")
	}

	if htmlContent == "" {
		return "", "", "", fmt.Errorf("HTML body is empty")
	}

	return subject, plainBody, htmlContent, nil
}

// convertToHTML converts plain text to basic HTML
func (er *EmailResponder) convertToHTML(plainText string) string {
	if plainText == "" {
		return ""
	}

	// Simple conversion: replace newlines with <br> tags
	html := strings.ReplaceAll(plainText, "\n", "<br>\n")
	return fmt.Sprintf("<html><body>%s</body></html>", html)
}
