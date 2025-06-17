package nudge

import (
	"context"
	"fmt"
)

// MockTemplateReader implements TemplateReader for testing
type MockTemplateReader struct {
	templates map[NudgeType][]byte
	readError error
}

func NewMockTemplateReader() *MockTemplateReader {
	return &MockTemplateReader{
		templates: make(map[NudgeType][]byte),
	}
}

func (m *MockTemplateReader) SetTemplate(nudgeType NudgeType, content []byte) {
	m.templates[nudgeType] = content
}

func (m *MockTemplateReader) SetReadError(err error) {
	m.readError = err
}

func (m *MockTemplateReader) ReadTemplate(nudgeType NudgeType) ([]byte, error) {
	if m.readError != nil {
		return nil, m.readError
	}

	if template, exists := m.templates[nudgeType]; exists {
		return template, nil
	}

	return nil, fmt.Errorf("template not found for nudge type: %s", nudgeType)
}

// MockPromptProcessor implements PromptProcessor for testing
type MockPromptProcessor struct {
	generatedPrompt string
	promptResponse  string
	promptError     error
	processError    error
}

func NewMockPromptProcessor() *MockPromptProcessor {
	return &MockPromptProcessor{}
}

func (m *MockPromptProcessor) SetGeneratedPrompt(prompt string) {
	m.generatedPrompt = prompt
}

func (m *MockPromptProcessor) SetPromptResponse(response string) {
	m.promptResponse = response
}

func (m *MockPromptProcessor) SetPromptError(err error) {
	m.promptError = err
}

func (m *MockPromptProcessor) SetProcessError(err error) {
	m.processError = err
}

func (m *MockPromptProcessor) GeneratePrompt(template []byte, data *NudgeEmailData) (string, error) {
	if m.promptError != nil {
		return "", m.promptError
	}
	return m.generatedPrompt, nil
}

func (m *MockPromptProcessor) ProcessPrompt(ctx context.Context, prompt string) (string, error) {
	if m.processError != nil {
		return "", m.processError
	}
	return m.promptResponse, nil
}

// MockEmailSender implements EmailSender for testing
type MockEmailSender struct {
	sentEmails []SentEmail
	sendError  error
}

type SentEmail struct {
	Recipient string
	Email     *GeneratedEmail
	NudgeType NudgeType
}

func NewMockEmailSender() *MockEmailSender {
	return &MockEmailSender{
		sentEmails: make([]SentEmail, 0),
	}
}

func (m *MockEmailSender) SetSendError(err error) {
	m.sendError = err
}

func (m *MockEmailSender) GetSentEmails() []SentEmail {
	return m.sentEmails
}

func (m *MockEmailSender) SendEmail(ctx context.Context, recipient string, email *GeneratedEmail, nudgeType NudgeType) error {
	if m.sendError != nil {
		return m.sendError
	}

	m.sentEmails = append(m.sentEmails, SentEmail{
		Recipient: recipient,
		Email:     email,
		NudgeType: nudgeType,
	})

	return nil
}
