package utils

import (
	"testing"
)

func TestValidateTemplateContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "Valid double curly braces",
			content: "This is a {{valid}} template with {{.variable}}",
			wantErr: false,
		},
		{
			name:    "Single left curly brace",
			content: "This has a {single left brace",
			wantErr: true,
		},
		{
			name:    "Single right curly brace",
			content: "This has a single right brace}",
			wantErr: true,
		},
		{
			name:    "Both single braces",
			content: "This has {both} single braces",
			wantErr: true,
		},
		{
			name:    "Mixed valid and invalid",
			content: "This has {{valid}} and {invalid}",
			wantErr: true,
		},
		{
			name:    "Valid with escaped braces",
			content: "This has escaped \\{{ and \\}} braces",
			wantErr: false,
		},
		{
			name:    "Empty template",
			content: "",
			wantErr: false,
		},
		{
			name:    "Multiple double braces",
			content: "{{one}} {{two}} {{three}}",
			wantErr: false,
		},
		{
			name:    "Nested curly braces",
			content: "{{ if eq .Value \"something\" }}nested{{end}}",
			wantErr: false,
		},
		{
			name:    "Valid with code snippets",
			content: "Code snippet: ```if x { return y }```",
			wantErr: false, // Code snippets with braces are fine as they're not template variables
		},
		{
			name:    "Common mistake with space between braces",
			content: "This is { { invalid } } with space",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateContent(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTemplateContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
