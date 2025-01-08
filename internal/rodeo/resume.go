package rodeo

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chriscow/minds"
	"github.com/fumiama/go-docx"
)

type StatusSender interface {
	SendStatus(sessionID, level, message string)
}

func convertResume(ctx context.Context, llm minds.ContentGenerator, resumeText string, sessionID string, status StatusSender) (string, error) {
	prompt, err := CreateTemplate("prompts/recruiter.convert-resume.txt")
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to create template")
		return "", fmt.Errorf("convertResume: failed to create template: %w", err)
	}

	status.SendStatus(sessionID, "info", "Analyzing resume content...")

	req := minds.NewRequest(minds.Messages{
		{Role: minds.RoleUser, Content: prompt.Body() + "\n\n" + resumeText},
	})

	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to generate content")
		return "", fmt.Errorf("analyzeResume: failed to generate content: %w", err)
	}

	analysis, ok := resp.Text()
	if !ok {
		status.SendStatus(sessionID, "error", "Failed to get text from response")
		return "", fmt.Errorf("analyzeResume: failed to get text from response")
	}

	return analysis, nil
}

func ExtractDOCX(file *os.File) (string, error) {
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

func ExtractPDF(file *os.File) (string, error) {
	// TODO: Implement PDF text extraction
	// You'll need to add a PDF library like github.com/ledongthuc/pdf
	return "", fmt.Errorf("PDF extraction not implemented")
}
