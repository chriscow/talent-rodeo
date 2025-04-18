package utils

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/assets"

	"gopkg.in/yaml.v2"
)

type PromptHeader map[string]any

type Prompt struct {
	Header     map[string]any
	Template   *template.Template
	RawContent string
}

func (p Prompt) Execute(data any) (string, error) {
	var result strings.Builder
	if err := p.Template.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

func validateTemplateContent(content string) error {
	// Skip validation for code blocks that might contain valid single braces
	// Extract content outside of code blocks (indicated by ```...```)
	contentToCheck := content
	codeBlocks := strings.Split(content, "```")

	// If there are code blocks, we only want to check the non-code parts
	if len(codeBlocks) > 1 {
		// Rebuild the content without code blocks
		contentToCheck = ""
		for i, block := range codeBlocks {
			// Even indices are non-code blocks
			if i%2 == 0 {
				contentToCheck += block
			}
		}
	}

	// Check for unmatched single curly braces in non-code content
	doubleBraceCount := strings.Count(contentToCheck, "{{")
	doubleCloseBraceCount := strings.Count(contentToCheck, "}}")

	// Count total occurrences of { and }
	totalLeftBrace := strings.Count(contentToCheck, "{")
	totalRightBrace := strings.Count(contentToCheck, "}")

	// If there are more single braces than double braces, there must be a single brace
	if totalLeftBrace > doubleBraceCount*2 {
		return fmt.Errorf("template contains single left curly brace '{', use '{{' instead")
	}

	if totalRightBrace > doubleCloseBraceCount*2 {
		return fmt.Errorf("template contains single right curly brace '}', use '}}' instead")
	}

	return nil
}

func CreateTemplate(filepath string) (Prompt, error) {
	var prompt Prompt
	content, err := assets.EmbeddedFiles.ReadFile(filepath)
	if err != nil {
		return prompt, err
	}

	header, body, err := extractYAMLHeader(string(content))
	if err != nil {
		return prompt, err
	}

	// Validate template content for single curly braces
	if err := validateTemplateContent(body); err != nil {
		return prompt, err
	}

	sha, err := SHA256Hash([]byte(body))
	if err != nil {
		return prompt, err
	}

	header["SHA256"] = sha
	var name string
	if name, ok := header["name"].(string); !ok || name == "" {
		name = filepath
	}
	prompt.Header = header
	prompt.RawContent = body

	tmpl, err := template.New(name).Parse(body)
	if err != nil {
		return prompt, err
	}
	prompt.Template = tmpl

	return prompt, nil
}

func SavePromptTemplate(filePath string, header PromptHeader, body string) error {
	file, err := os.Create(filepath.Join("assets", filePath))
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString("---\n"); err != nil {
		return err
	}
	meta, err := yaml.Marshal(header)
	if err != nil {
		return err
	}
	if _, err := file.Write(meta); err != nil {
		return err
	}
	if _, err := file.WriteString("\n---\n"); err != nil {
		return err
	}
	if _, err := file.WriteString(body); err != nil {
		return err
	}
	return nil
}

func extractYAMLHeader(templateStr string) (PromptHeader, string, error) {
	const delimiter = "---"
	header := make(PromptHeader)

	parts := strings.Split(templateStr, delimiter)
	if len(parts) < 3 {
		return header, templateStr, nil
	}

	meta := strings.TrimSpace(parts[1])
	if strings.HasPrefix(meta, "-") {
		// the user put too many dashes or it was not the right format
		return header, templateStr, fmt.Errorf("invalid header format")
	}
	body := strings.Join(parts[2:], delimiter)

	if err := yaml.Unmarshal([]byte(meta), &header); err != nil {
		return header, templateStr, err
	}

	return header, strings.TrimSpace(body), nil
}
