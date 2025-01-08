package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
	"github.com/fumiama/go-docx"
)

func (app *application) convertResume(text string, sessionID string) (string, error) {
	// Send status that we're starting
	app.sendStatus(sessionID, "info", "Starting resume analysis...")

	prompt := `Please create a short summary in YAML format of the resume with key facts, soft
skills, programming languages, technology stacks. For each, add a field called
ai_comment with your own observation on skill level and length of time since
last used. Also any accomplishments, education and any other pertinent
information.`

	app.sendStatus(sessionID, "info", "Creating Gemini provider...")

	ctx := context.Background()
	llm, err := gemini.NewProvider(ctx, gemini.WithSystemPrompt(prompt))
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to create Gemini provider")
		return "", fmt.Errorf("analyzeResume: failed to create Gemini provider: %w", err)
	}

	app.sendStatus(sessionID, "info", "Analyzing resume content...")

	req := minds.NewRequest(minds.Messages{{Role: minds.RoleUser, Content: text}})
	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to generate content")
		return "", fmt.Errorf("analyzeResume: failed to generate content: %w", err)
	}

	text, ok := resp.Text()
	if !ok {
		app.sendStatus(sessionID, "error", "Failed to get text from response")
		return "", fmt.Errorf("analyzeResume: failed to get text from response")
	}

	app.sendStatus(sessionID, "success", "Analysis complete!")

	app.logger.Debug("analyzeResume", "response", text[:40])
	return text, nil
}

func extractDOCX(file *os.File) (string, error) {
	fileinfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("extractDOCX: failed to get file info")
	}

	size := fileinfo.Size()

	doc, err := docx.Parse(file, size)
	if err != nil {
		return "", fmt.Errorf("extractDOCX: failed to parse DOCX file")
	}

	sb := strings.Builder{}
	for _, it := range doc.Document.Body.Items {
		switch it.(type) {
		case *docx.Paragraph, *docx.Table: // printable
			s, ok := it.(fmt.Stringer)
			if !ok {
				return "", fmt.Errorf("extractDOCX: failed to convert inteface to string")
			}
			sb.WriteString(s.String())
		}
	}

	return sb.String(), nil
}

func extractPDF(file *os.File) (string, error) {
	// TODO: Implement PDF text extraction
	// You'll need to add a PDF library like github.com/ledongthuc/pdf
	return "", fmt.Errorf("PDF extraction not implemented")
}
