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

type PromptHeader struct {
	Name    string `yaml:"name,omitempty"`
	Version string `yaml:"version,omitempty"`
	Format  string `yaml:"format,omitempty"`
	SHA256  string `yaml:"sha256,omitempty"`
}

type Prompt struct {
	Header   PromptHeader
	Template *template.Template
	body     string
}

func (p Prompt) Body() string {
	return p.body
}

func (p Prompt) Execute(data interface{}) (string, error) {
	var result strings.Builder
	if err := p.Template.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
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
	sha, err := SHA256Hash([]byte(body))
	if err != nil {
		return prompt, err
	}
	if header.SHA256 != "" && header.SHA256 != sha {
		return prompt, fmt.Errorf("SHA256 mismatch. Bump the version and update the SHA256")
	} else if header.SHA256 == "" {
		header.SHA256 = sha
		// if err := SavePromptTemplate(filepath, header, body); err != nil {
		// 	return prompt, fmt.Errorf("failed to save prompt template while updating hash: %w", err)
		// }
	}
	prompt.Header = header
	prompt.body = body

	tmpl, err := template.New(header.Name).Parse(body)
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
	var header PromptHeader
	parts := strings.Split(templateStr, delimiter)
	if len(parts) < 3 {
		return header, "", nil
	}

	meta := strings.TrimSpace(parts[1])
	if strings.HasPrefix(meta, "-") {
		// the user put too many dashes or it was not the right format
		return header, "", fmt.Errorf("invalid header format")
	}
	body := strings.Join(parts[2:], delimiter)

	if err := yaml.Unmarshal([]byte(meta), &header); err != nil {
		return header, "", err
	}

	return header, strings.TrimSpace(body), nil
}
