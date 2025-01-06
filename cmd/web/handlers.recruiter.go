package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/cmd/web/components/partials"
	"talent-rodeo/internal/response"
	"time"

	"github.com/alexedwards/flow"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
	"github.com/fumiama/go-docx"
	"github.com/tmaxmax/go-sse"
)

func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".docx"
}

func (app *application) resume(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "resume.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) uploadResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		partials.UploadError("Failed to parse form").Render(r.Context(), w)
		return
	}

	// Get phone number
	phone := r.FormValue("phone")
	if phone == "" {
		partials.UploadError("Phone number is required").Render(r.Context(), w)
		return
	}

	// Get resume file
	file, header, err := r.FormFile("resume")
	if err != nil {
		partials.UploadError("Failed to get resume file").Render(r.Context(), w)
		return
	}
	defer file.Close()

	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "resume-*.docx")
	if err != nil {
		http.Error(w, "Error creating temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name()) // Clean up temp file after we're done

	// Copy the uploaded file to the temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "Error copying file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	// Read file content
	var textContent string
	if strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
		textContent, err = extractDOCX(tempFile)
	} else if strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		textContent, err = extractPDF(tempFile)
	} else {
		partials.UploadError("Unsupported file format").Render(r.Context(), w)
		return
	}

	if err != nil {
		partials.UploadError("Failed to extract text from resume").Render(r.Context(), w)
		return
	}

	app.backgroundTask(r, func() error {
		ctx := context.Background()
		yaml, err := app.analyzeResume(ctx, textContent, "")
		if err != nil {
			app.logger.Error("uploadResume: failed to analyze resume", "error", err)
			return fmt.Errorf("uploadResume: failed to analyze resume: %w", err)
		}

		var sessionID string
		session, err := app.sessionStore.Get(r, "session")
		if err != nil {
			app.logger.Error("uploadResume: failed to get session", "error", err)
			return fmt.Errorf("uploadResume: failed to get session: %w", err)
		}

		sessionID, ok := session.Values["sessionID"].(string)
		if !ok {
			app.logger.Error("uploadResume: failed to get session ID",
				"session", session.Values, "ok", ok)

			return fmt.Errorf("uploadResume: failed to get session ID")
		}

		msg := &sse.Message{}
		msg.AppendData("Hello, " + sessionID)
		msg.AppendData(yaml)
		app.sse.Publish(msg, sessionID) // Pass sessionID as the topic

		app.logger.Info("uploadResume: resume analysis complete")

		return nil
	})

	// Render the result
	// partials.ResumeAnalysis(yaml).Render(r.Context(), w)
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/call-status.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) sendStatus(sessionID, status, message string) {
	msg := &sse.Message{}
	msg.AppendData(status)
	msg.AppendData(message)
	app.sse.Publish(msg, sessionID)
}

func (app *application) analyzeResume(ctx context.Context, text string, sessionID string) (string, error) {
	// Send status that we're starting
	app.sendStatus(sessionID, "status", "Starting resume analysis...")

	prompt := `Please create a short summary in YAML format of the resume with key facts, soft
skills, programming languages, technology stacks. For each, add a field called
ai_comment with your own observation on skill level and length of time since
last used. Also any accomplishments, education and any other pertinent
information.`

	app.sendStatus(sessionID, "status", "Creating Gemini provider...")

	llm, err := gemini.NewProvider(ctx, gemini.WithSystemPrompt(prompt))
	if err != nil {
		app.sendStatus(sessionID, "error", "Failed to create Gemini provider")
		return "", fmt.Errorf("analyzeResume: failed to create Gemini provider: %w", err)
	}

	app.sendStatus(sessionID, "status", "Analyzing resume content...")

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

func (app *application) watch(w http.ResponseWriter, r *http.Request) {
	sessionID := flow.Param(r.Context(), "session-id")
	app.logger.Debug("watch", "sessionID", sessionID)

	// Start session-specific publisher
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			msg := &sse.Message{}
			msg.AppendData(fmt.Sprintf("Session %s time: %s",
				sessionID[:6], time.Now().Format(time.RFC3339)))
			app.sse.Publish(msg, sessionID)
		}
	}()

	app.sse.OnSession = func(sess *sse.Session) (sse.Subscription, bool) {
		return sse.Subscription{
			Client: sess,
			Topics: []string{sessionID},
		}, true
	}

	app.sse.ServeHTTP(w, r)
}
