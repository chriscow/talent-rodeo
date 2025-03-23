package rodeo

import (
	"context"
	"fmt"
	"os"
	"strings"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	"github.com/fumiama/go-docx"
	"github.com/ledongthuc/pdf"
)

func convertResume(ctx context.Context, llm minds.ContentGenerator, resumeText string, sessionID string, status StatusSender) (string, error) {
	prompt, err := utils.CreateTemplate("prompts/recruiter.convert-resume.txt")
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

	analysis := resp.String()
	if analysis == "" {
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
	// Get file info for size
	fileinfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("extractPDF: failed to get file info: %w", err)
	}

	// Create a reader for the PDF
	reader, err := pdf.NewReader(file, fileinfo.Size())
	if err != nil {
		return "", fmt.Errorf("extractPDF: failed to create PDF reader: %w", err)
	}

	var sb strings.Builder
	numPages := reader.NumPage()

	// Extract text from each page
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue // Skip empty pages
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extractPDF: failed to extract text from page %d: %w", pageNum, err)
		}
		sb.WriteString(text)
	}

	return sb.String(), nil
}
