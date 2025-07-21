package inbox

import (
	"testing"
	"time"
)

func TestResponseType_String(t *testing.T) {
	tests := []struct {
		name     string
		rt       ResponseType
		expected string
	}{
		{
			name:     "confirmation",
			rt:       ResponseTypeConfirmation,
			expected: "confirmation",
		},
		{
			name:     "partial",
			rt:       ResponseTypePartial,
			expected: "partial",
		},
		{
			name:     "escalation",
			rt:       ResponseTypeEscalation,
			expected: "escalation",
		},
		{
			name:     "invalid",
			rt:       ResponseType(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rt.String()
			if result != tt.expected {
				t.Errorf("ResponseType.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResponseType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		rt       ResponseType
		expected bool
	}{
		{
			name:     "confirmation valid",
			rt:       ResponseTypeConfirmation,
			expected: true,
		},
		{
			name:     "partial valid",
			rt:       ResponseTypePartial,
			expected: true,
		},
		{
			name:     "escalation valid",
			rt:       ResponseTypeEscalation,
			expected: true,
		},
		{
			name:     "invalid value",
			rt:       ResponseType(999),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rt.IsValid()
			if result != tt.expected {
				t.Errorf("ResponseType.IsValid() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmailResponse_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		response EmailResponse
		wantErr  bool
	}{
		{
			name: "valid response with body",
			response: EmailResponse{
				Type:    ResponseTypeConfirmation,
				Subject: "Test Subject",
				Body:    "Test Body",
			},
			wantErr: false,
		},
		{
			name: "valid response with HTML body",
			response: EmailResponse{
				Type:     ResponseTypeConfirmation,
				Subject:  "Test Subject",
				HTMLBody: "<p>Test Body</p>",
			},
			wantErr: false,
		},
		{
			name: "valid response with both bodies",
			response: EmailResponse{
				Type:     ResponseTypeConfirmation,
				Subject:  "Test Subject",
				Body:     "Test Body",
				HTMLBody: "<p>Test Body</p>",
			},
			wantErr: false,
		},
		{
			name: "missing subject",
			response: EmailResponse{
				Type: ResponseTypeConfirmation,
				Body: "Test Body",
			},
			wantErr: true,
		},
		{
			name: "missing body",
			response: EmailResponse{
				Type:    ResponseTypeConfirmation,
				Subject: "Test Subject",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			response: EmailResponse{
				Type:    ResponseType(999),
				Subject: "Test Subject",
				Body:    "Test Body",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("EmailResponse.IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEmailResponse_HasContent(t *testing.T) {
	tests := []struct {
		name     string
		response EmailResponse
		expected bool
	}{
		{
			name: "has summaries",
			response: EmailResponse{
				Summaries: []string{"Action 1", "Action 2"},
			},
			expected: true,
		},
		{
			name: "has escalated items",
			response: EmailResponse{
				EscalatedItems: []string{"Item 1", "Item 2"},
			},
			expected: true,
		},
		{
			name: "has both",
			response: EmailResponse{
				Summaries:      []string{"Action 1"},
				EscalatedItems: []string{"Item 1"},
			},
			expected: true,
		},
		{
			name: "no content",
			response: EmailResponse{
				Summaries:      []string{},
				EscalatedItems: []string{},
			},
			expected: false,
		},
		{
			name: "nil slices",
			response: EmailResponse{
				Summaries:      nil,
				EscalatedItems: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.HasContent()
			if result != tt.expected {
				t.Errorf("EmailResponse.HasContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmailResponse_HandledCount(t *testing.T) {
	tests := []struct {
		name     string
		response EmailResponse
		expected int
	}{
		{
			name: "multiple summaries",
			response: EmailResponse{
				Summaries: []string{"Action 1", "Action 2", "Action 3"},
			},
			expected: 3,
		},
		{
			name: "no summaries",
			response: EmailResponse{
				Summaries: []string{},
			},
			expected: 0,
		},
		{
			name: "nil summaries",
			response: EmailResponse{
				Summaries: nil,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.HandledCount()
			if result != tt.expected {
				t.Errorf("EmailResponse.HandledCount() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmailResponse_EscalatedCount(t *testing.T) {
	tests := []struct {
		name     string
		response EmailResponse
		expected int
	}{
		{
			name: "multiple escalated items",
			response: EmailResponse{
				EscalatedItems: []string{"Item 1", "Item 2", "Item 3"},
			},
			expected: 3,
		},
		{
			name: "no escalated items",
			response: EmailResponse{
				EscalatedItems: []string{},
			},
			expected: 0,
		},
		{
			name: "nil escalated items",
			response: EmailResponse{
				EscalatedItems: nil,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.EscalatedCount()
			if result != tt.expected {
				t.Errorf("EmailResponse.EscalatedCount() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmailResponse_GeneratedAt(t *testing.T) {
	now := time.Now()
	response := EmailResponse{
		Type:        ResponseTypeConfirmation,
		Subject:     "Test Subject",
		Body:        "Test Body",
		GeneratedAt: now,
	}

	if !response.GeneratedAt.Equal(now) {
		t.Errorf("EmailResponse.GeneratedAt = %v, want %v", response.GeneratedAt, now)
	}
}
