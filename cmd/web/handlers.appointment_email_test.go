package main

import (
	"strings"
	"talent-rodeo/assets"
	"testing"
)

// TestEmailPromptTemplatesExist verifies that the email templates and prompts were created
func TestEmailPromptTemplatesExist(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
	}{
		{
			name:     "Failure Email Template",
			filename: "emails/call-failure-email.html",
		},
		{
			name:     "Failure Email Prompt",
			filename: "prompts/call-failure-email.001.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Check if files exist by reading them
			data, err := assets.EmbeddedFiles.ReadFile(tc.filename)
			if err != nil {
				t.Fatalf("Template file %s does not exist or cannot be read: %v", tc.filename, err)
			}

			content := string(data)
			if len(content) == 0 {
				t.Errorf("Template file %s is empty", tc.filename)
			}

			// Basic content validation
			if strings.Contains(tc.filename, "failure") {
				if strings.Contains(tc.filename, "emails/") {
					// Check that failure email templates contain expected elements
					expectedElements := []string{"Shirely", "{{.PlainBody}}", "{{safeHTML .HtmlBody}}"}
					for _, element := range expectedElements {
						if !strings.Contains(content, element) {
							t.Errorf("Template %s missing expected element: %s", tc.filename, element)
						}
					}
				} else if strings.Contains(tc.filename, "prompts/") {
					// Check that failure prompts contain expected elements
					expectedElements := []string{"Shirely", "{{.FirstName}}", "{{.FailureReason}}"}
					for _, element := range expectedElements {
						if !strings.Contains(content, element) {
							t.Errorf("Template %s missing expected element: %s", tc.filename, element)
						}
					}
				}
			}

			t.Logf("Template %s exists and contains %d characters", tc.filename, len(content))
		})
	}
}

// TestFailureEmailPromptContent validates the content of the failure email prompt
func TestFailureEmailPromptContent(t *testing.T) {
	promptData, err := assets.EmbeddedFiles.ReadFile("prompts/call-failure-email.001.txt")
	if err != nil {
		t.Fatalf("Could not read failure email prompt: %v", err)
	}

	prompt := string(promptData)

	// Check that the prompt contains all necessary elements
	expectedElements := []string{
		"Shirely",
		"{{.FullName}}",
		"{{.FirstName}}",
		"{{.FailureReason}}",
		"<plain_email_body>",
		"<html_email_body>",
		"https://talent.rodeo",
		"schedule another call",
		"deliberately misspelled",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("Failure email prompt missing expected element: %s", element)
		}
	}

	// Log the prompt content for manual inspection
	t.Logf("\n=== Failure Email Prompt Content ===\n%s\n=====================================\n", prompt)
}

// TestFailureEmailTemplateContent validates the content of the failure email template
func TestFailureEmailTemplateContent(t *testing.T) {
	templateData, err := assets.EmbeddedFiles.ReadFile("emails/call-failure-email.html")
	if err != nil {
		t.Fatalf("Could not read failure email template: %v", err)
	}

	template := string(templateData)

	// Check that the template contains all necessary elements
	expectedElements := []string{
		`{{define "subject"}}`,
		`{{define "plainBody"}}`,
		`{{define "htmlBody"}}`,
		"Quick update about your call with Shirely",
		"{{.PlainBody}}",
		"{{safeHTML .HtmlBody}}",
	}

	for _, element := range expectedElements {
		if !strings.Contains(template, element) {
			t.Errorf("Failure email template missing expected element: %s", element)
		}
	}

	// Log the template content for manual inspection
	t.Logf("\n=== Failure Email Template Content ===\n%s\n======================================\n", template)
}

// TestCompareEmailTemplates compares failure and recap email templates to ensure consistency
func TestCompareEmailTemplates(t *testing.T) {
	failureTemplate, err := assets.EmbeddedFiles.ReadFile("emails/call-failure-email.html")
	if err != nil {
		t.Fatalf("Could not read failure email template: %v", err)
	}

	recapTemplate, err := assets.EmbeddedFiles.ReadFile("emails/recap-email.html")
	if err != nil {
		t.Fatalf("Could not read recap email template: %v", err)
	}

	failure := string(failureTemplate)
	recap := string(recapTemplate)

	// Both templates should have the same structure
	commonElements := []string{
		`{{define "subject"}}`,
		`{{define "plainBody"}}`,
		`{{define "htmlBody"}}`,
		"<!doctype html>",
		"{{.PlainBody}}",
		"{{safeHTML .HtmlBody}}",
	}

	for _, element := range commonElements {
		if !strings.Contains(failure, element) {
			t.Errorf("Failure template missing common element: %s", element)
		}
		if !strings.Contains(recap, element) {
			t.Errorf("Recap template missing common element: %s", element)
		}
	}

	t.Logf("Both email templates contain all expected common elements")
}
