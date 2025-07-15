package inbox

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

// mockOpenAIClient implements OpenAIClient for testing
type mockOpenAIClient struct {
	response string
	err      error
}

func (m *mockOpenAIClient) GenerateResponse(ctx context.Context, originalEmail *Email, results []*HandlingResult) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestNewEmailResponder(t *testing.T) {
	client := &mockOpenAIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	responder := NewEmailResponder(client, logger)

	if responder == nil {
		t.Error("NewEmailResponder should not return nil")
	}

	if responder.openAIClient != client {
		t.Error("NewEmailResponder should set the OpenAI client")
	}

	if responder.logger == nil {
		t.Error("NewEmailResponder should set the logger")
	}
}

func TestEmailResponder_GenerateResponse(t *testing.T) {
	tests := []struct {
		name            string
		originalEmail   *Email
		results         []*HandlingResult
		llmResponse     string
		llmError        error
		expectedType    ResponseType
		expectedError   bool
		expectedSubject string
		expectedBody    string
	}{
		{
			name: "successful confirmation response",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Update my search",
				Body:    "Please update my search preferences",
			},
			results: []*HandlingResult{
				{
					Handled: true,
					Summary: "Updated search preferences successfully",
				},
			},
			llmResponse: `<plain_email_body>
Subject: Re: Update my search

Hi there,

I've successfully updated your search preferences as requested.

Best regards,
Talent Rodeo Team
</plain_email_body>

<html_email_body>
<div style="font-family: Arial, sans-serif;">
<h1>Search Updated Successfully!</h1>
<p>Hi there,</p>
<p>I've successfully updated your search preferences as requested.</p>
<p>Best regards,<br>Talent Rodeo Team</p>
</div>
</html_email_body>`,
			expectedType:    ResponseTypeConfirmation,
			expectedSubject: "Re: Update my search",
			expectedBody:    "Hi there,\n\nI've successfully updated your search preferences as requested.\n\nBest regards,\nTalent Rodeo Team",
		},
		{
			name: "partial response with escalation",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Multiple requests",
				Body:    "Update my search and help with billing",
			},
			results: []*HandlingResult{
				{
					Handled: true,
					Summary: "Updated search preferences",
				},
				{
					Handled: false,
					Summary: "",
				},
			},
			llmResponse: `<plain_email_body>
Subject: Re: Multiple requests

Hi there,

I've updated your search preferences. Your billing question has been escalated to our team.

Best regards,
Talent Rodeo Team
</plain_email_body>

<html_email_body>
<div style="font-family: Arial, sans-serif;">
<h1>Partially Processed</h1>
<p>Hi there,</p>
<p>I've updated your search preferences. Your billing question has been escalated to our team.</p>
<p>Best regards,<br>Talent Rodeo Team</p>
</div>
</html_email_body>`,
			expectedType:    ResponseTypePartial,
			expectedSubject: "Re: Multiple requests",
			expectedBody:    "Hi there,\n\nI've updated your search preferences. Your billing question has been escalated to our team.\n\nBest regards,\nTalent Rodeo Team",
		},
		{
			name: "escalation only response",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Complex request",
				Body:    "I need help with something complex",
			},
			results: []*HandlingResult{
				{
					Handled: false,
					Summary: "",
				},
			},
			llmResponse: `<plain_email_body>
Subject: Re: Complex request

Hi there,

I've escalated your request to our team who will help you shortly.

Best regards,
Talent Rodeo Team
</plain_email_body>

<html_email_body>
<div style="font-family: Arial, sans-serif;">
<h1>Request Escalated</h1>
<p>Hi there,</p>
<p>I've escalated your request to our team who will help you shortly.</p>
<p>Best regards,<br>Talent Rodeo Team</p>
</div>
</html_email_body>`,
			expectedType:    ResponseTypeEscalation,
			expectedSubject: "Re: Complex request",
			expectedBody:    "Hi there,\n\nI've escalated your request to our team who will help you shortly.\n\nBest regards,\nTalent Rodeo Team",
		},
		{
			name:          "nil email",
			originalEmail: nil,
			results:       []*HandlingResult{},
			expectedError: true,
		},
		{
			name: "no results",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			results:       []*HandlingResult{},
			expectedError: true,
		},
		{
			name: "invalid email",
			originalEmail: &Email{
				ID:   "",
				From: "",
			},
			results: []*HandlingResult{
				{Handled: true, Summary: "Test"},
			},
			expectedError: true,
		},
		{
			name: "LLM error",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			results: []*HandlingResult{
				{Handled: true, Summary: "Test"},
			},
			llmError:      errors.New("LLM failed"),
			expectedError: true,
		},
		{
			name: "invalid LLM response",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			results: []*HandlingResult{
				{Handled: true, Summary: "Test"},
			},
			llmResponse:   "Invalid response without proper format",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockOpenAIClient{
				response: tt.llmResponse,
				err:      tt.llmError,
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			responder := NewEmailResponder(client, logger)

			response, err := responder.GenerateResponse(context.Background(), tt.originalEmail, tt.results)

			if (err != nil) != tt.expectedError {
				t.Errorf("GenerateResponse() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			if tt.expectedError {
				return
			}

			if response == nil {
				t.Error("GenerateResponse() should not return nil response")
				return
			}

			if response.Type != tt.expectedType {
				t.Errorf("GenerateResponse() type = %v, want %v", response.Type, tt.expectedType)
			}

			if response.Subject != tt.expectedSubject {
				t.Errorf("GenerateResponse() subject = %v, want %v", response.Subject, tt.expectedSubject)
			}

			if response.Body != tt.expectedBody {
				t.Errorf("GenerateResponse() body = %v, want %v", response.Body, tt.expectedBody)
			}

			if response.HTMLBody == "" {
				t.Error("GenerateResponse() should set HTMLBody")
			}

			if response.GeneratedAt.IsZero() {
				t.Error("GenerateResponse() should set GeneratedAt")
			}
		})
	}
}

func TestEmailResponder_validateEmail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	responder := NewEmailResponder(&mockOpenAIClient{}, logger)

	tests := []struct {
		name      string
		email     *Email
		wantError bool
	}{
		{
			name: "valid email",
			email: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantError: false,
		},
		{
			name: "empty ID",
			email: &Email{
				ID:      "",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantError: true,
		},
		{
			name: "empty from",
			email: &Email{
				ID:      "test-id",
				From:    "",
				Subject: "Test",
				Body:    "Test body",
			},
			wantError: true,
		},
		{
			name: "empty subject and body",
			email: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "",
				Body:    "",
			},
			wantError: true,
		},
		{
			name: "empty subject but has body",
			email: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "",
				Body:    "Test body",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := responder.validateEmail(tt.email)
			if (err != nil) != tt.wantError {
				t.Errorf("validateEmail() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestEmailResponder_determineResponseType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	responder := NewEmailResponder(&mockOpenAIClient{}, logger)

	tests := []struct {
		name     string
		results  []*HandlingResult
		expected ResponseType
	}{
		{
			name: "all handled",
			results: []*HandlingResult{
				{Handled: true, Summary: "Action 1"},
				{Handled: true, Summary: "Action 2"},
			},
			expected: ResponseTypeConfirmation,
		},
		{
			name: "none handled",
			results: []*HandlingResult{
				{Handled: false, Summary: ""},
				{Handled: false, Summary: ""},
			},
			expected: ResponseTypeEscalation,
		},
		{
			name: "partially handled",
			results: []*HandlingResult{
				{Handled: true, Summary: "Action 1"},
				{Handled: false, Summary: ""},
			},
			expected: ResponseTypePartial,
		},
		{
			name: "single handled",
			results: []*HandlingResult{
				{Handled: true, Summary: "Action 1"},
			},
			expected: ResponseTypeConfirmation,
		},
		{
			name: "single unhandled",
			results: []*HandlingResult{
				{Handled: false, Summary: ""},
			},
			expected: ResponseTypeEscalation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := responder.determineResponseType(tt.results)
			if result != tt.expected {
				t.Errorf("determineResponseType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmailResponder_collectResultData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	responder := NewEmailResponder(&mockOpenAIClient{}, logger)

	tests := []struct {
		name                string
		results             []*HandlingResult
		expectedSummaries   []string
		expectedEscalations []string
	}{
		{
			name: "mixed results",
			results: []*HandlingResult{
				{Handled: true, Summary: "Action 1"},
				{Handled: false, Summary: ""},
				{Handled: true, Summary: "Action 2"},
			},
			expectedSummaries:   []string{"Action 1", "Action 2"},
			expectedEscalations: []string{"Part of your request could not be processed automatically"},
		},
		{
			name: "all handled",
			results: []*HandlingResult{
				{Handled: true, Summary: "Action 1"},
				{Handled: true, Summary: "Action 2"},
			},
			expectedSummaries:   []string{"Action 1", "Action 2"},
			expectedEscalations: []string{},
		},
		{
			name: "all unhandled",
			results: []*HandlingResult{
				{Handled: false, Summary: ""},
				{Handled: false, Summary: ""},
			},
			expectedSummaries: []string{},
			expectedEscalations: []string{
				"Part of your request could not be processed automatically",
				"Part of your request could not be processed automatically",
			},
		},
		{
			name: "handled without summary",
			results: []*HandlingResult{
				{Handled: true, Summary: ""},
			},
			expectedSummaries:   []string{},
			expectedEscalations: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summaries, escalations := responder.collectResultData(tt.results)

			if len(summaries) != len(tt.expectedSummaries) {
				t.Errorf("collectResultData() summaries length = %v, want %v", len(summaries), len(tt.expectedSummaries))
			}

			for i, summary := range summaries {
				if i < len(tt.expectedSummaries) && summary != tt.expectedSummaries[i] {
					t.Errorf("collectResultData() summary[%d] = %v, want %v", i, summary, tt.expectedSummaries[i])
				}
			}

			if len(escalations) != len(tt.expectedEscalations) {
				t.Errorf("collectResultData() escalations length = %v, want %v", len(escalations), len(tt.expectedEscalations))
			}

			for i, escalation := range escalations {
				if i < len(tt.expectedEscalations) && escalation != tt.expectedEscalations[i] {
					t.Errorf("collectResultData() escalation[%d] = %v, want %v", i, escalation, tt.expectedEscalations[i])
				}
			}
		})
	}
}

func TestEmailResponder_parseLLMResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	responder := NewEmailResponder(&mockOpenAIClient{}, logger)

	tests := []struct {
		name             string
		response         string
		expectedSubject  string
		expectedBody     string
		expectedHTMLBody string
		wantError        bool
	}{
		{
			name: "valid response with both formats",
			response: `<plain_email_body>
Subject: RE: Test Subject

Hi there!

This is a test plain text response.

Best regards,
Shirely
</plain_email_body>

<html_email_body>
<div style="font-family: Arial, sans-serif;">
<h1>Test Response</h1>
<p>This is a test HTML response.</p>
</div>
</html_email_body>`,
			expectedSubject: "RE: Test Subject",
			expectedBody:    "Hi there!\n\nThis is a test plain text response.\n\nBest regards,\nShirely",
			expectedHTMLBody: `<div style="font-family: Arial, sans-serif;">
<h1>Test Response</h1>
<p>This is a test HTML response.</p>
</div>`,
			wantError: false,
		},
		{
			name:      "missing plain_email_body",
			response:  `<html_email_body><p>HTML only</p></html_email_body>`,
			wantError: true,
		},
		{
			name:      "missing html_email_body",
			response:  `<plain_email_body>Subject: Test\nPlain only</plain_email_body>`,
			wantError: true,
		},
		{
			name:      "missing subject in plain body",
			response:  `<plain_email_body>No subject here</plain_email_body>\n<html_email_body><p>HTML</p></html_email_body>`,
			wantError: true,
		},
		{
			name:      "empty response",
			response:  "",
			wantError: true,
		},
		{
			name:      "invalid format",
			response:  "This is not a valid response format",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, plainBody, htmlBody, err := responder.parseLLMResponse(tt.response)

			if (err != nil) != tt.wantError {
				t.Errorf("parseLLMResponse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError {
				if subject != tt.expectedSubject {
					t.Errorf("parseLLMResponse() subject = %v, want %v", subject, tt.expectedSubject)
				}

				if plainBody != tt.expectedBody {
					t.Errorf("parseLLMResponse() plainBody = %v, want %v", plainBody, tt.expectedBody)
				}

				if htmlBody != tt.expectedHTMLBody {
					t.Errorf("parseLLMResponse() htmlBody = %v, want %v", htmlBody, tt.expectedHTMLBody)
				}
			}
		})
	}
}

func TestEmailResponder_convertToHTML(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	responder := NewEmailResponder(&mockOpenAIClient{}, logger)

	tests := []struct {
		name      string
		plainText string
		expected  string
	}{
		{
			name:      "simple text",
			plainText: "Hello world",
			expected:  "<html><body>Hello world</body></html>",
		},
		{
			name:      "text with newlines",
			plainText: "Line 1\nLine 2\nLine 3",
			expected:  "<html><body>Line 1<br>\nLine 2<br>\nLine 3</body></html>",
		},
		{
			name:      "empty text",
			plainText: "",
			expected:  "",
		},
		{
			name:      "text with multiple newlines",
			plainText: "Line 1\n\nLine 2\n\n\nLine 3",
			expected:  "<html><body>Line 1<br>\n<br>\nLine 2<br>\n<br>\n<br>\nLine 3</body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := responder.convertToHTML(tt.plainText)
			if result != tt.expected {
				t.Errorf("convertToHTML() = %v, want %v", result, tt.expected)
			}
		})
	}
}
