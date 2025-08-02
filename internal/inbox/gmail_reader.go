package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailReader implements InboxReader for Gmail API
type GmailReader struct {
	service *gmail.Service
	logger  *slog.Logger
}

// NewGmailReader creates a new Gmail inbox reader
func NewGmailReader(config InboxConfig, logger *slog.Logger) (*GmailReader, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	logger = logger.With("pkg", "inbox.GmailReader")

	decodedCredentials := []byte{}
	if credentials := os.Getenv("GMAIL_READER_CREDENTIALS"); credentials != "" {
		var err error
		decodedCredentials, err = base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			return nil, fmt.Errorf("failed to decode GMAIL_READER_CREDENTIALS: %w", err)
		}
	}

	// Configure OAuth2
	oauthConfig, err := google.ConfigFromJSON(decodedCredentials, gmail.GmailReadonlyScope, gmail.GmailModifyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	// Use out-of-band flow instead of localhost redirect
	oauthConfig.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"

	client, err := getGmailClient(oauthConfig, config.TokenPath, logger)
	if err != nil {
		return nil, fmt.Errorf("unable to get Gmail client: %w", err)
	}

	service, err := gmail.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Gmail client: %w", err)
	}

	return &GmailReader{
		service: service,
		logger:  logger,
	}, nil
}

// GetUnreadEmails retrieves unread emails from Gmail
func (g *GmailReader) GetUnreadEmails(ctx context.Context, maxResults int) ([]*Email, error) {
	g.logger.Debug("fetching unread emails", "max_results", maxResults)

	// Search for unread emails
	query := "is:unread"
	req := g.service.Users.Messages.List("me").Q(query)
	if maxResults > 0 {
		req = req.MaxResults(int64(maxResults))
	}

	resp, err := req.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve messages: %w", err)
	}

	if len(resp.Messages) == 0 {
		g.logger.Debug("no unread emails found")
		return []*Email{}, nil
	}

	emails := make([]*Email, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		email, err := g.convertMessage(ctx, msg.Id)
		if err != nil {
			g.logger.Error("failed to convert message", "message_id", msg.Id, "error", err)
			continue
		}
		emails = append(emails, email)
	}

	g.logger.Info("retrieved unread emails", "count", len(emails))
	return emails, nil
}

// GetAllEmails retrieves all emails (both read and unread) from Gmail inbox
func (g *GmailReader) GetAllEmails(ctx context.Context, maxResults int) ([]*Email, error) {
	g.logger.Debug("fetching all emails", "max_results", maxResults)
	// Search for all emails in inbox (no query filter)
	req := g.service.Users.Messages.List("me")
	if maxResults > 0 {
		req = req.MaxResults(int64(maxResults))
	}

	resp, err := req.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve messages: %w", err)
	}

	if len(resp.Messages) == 0 {
		g.logger.Debug("no emails found")
		return []*Email{}, nil
	}

	emails := make([]*Email, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		email, err := g.convertMessage(ctx, msg.Id)
		if err != nil {
			g.logger.Error("failed to convert message", "message_id", msg.Id, "error", err)
			continue
		}
		emails = append(emails, email)
	}

	g.logger.Info("retrieved all emails", "count", len(emails))
	return emails, nil
}

// MarkAsRead marks an email as read in Gmail
func (g *GmailReader) MarkAsRead(ctx context.Context, emailID string) error {
	g.logger.Debug("marking email as read", "email_id", emailID)

	req := &gmail.ModifyMessageRequest{
		RemoveLabelIds: []string{"UNREAD"},
	}

	_, err := g.service.Users.Messages.Modify("me", emailID, req).Do()
	if err != nil {
		return fmt.Errorf("unable to mark message as read: %w", err)
	}

	g.logger.Debug("email marked as read", "email_id", emailID)
	return nil
}

// Close closes the Gmail connection (no-op for Gmail API)
func (g *GmailReader) Close() error {
	return nil
}

// convertMessage converts a Gmail message to our Email struct
func (g *GmailReader) convertMessage(ctx context.Context, messageID string) (*Email, error) {
	msg, err := g.service.Users.Messages.Get("me", messageID).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve message: %w", err)
	}

	email := &Email{
		ID:       msg.Id,
		ThreadID: msg.ThreadId,
		Labels:   msg.LabelIds,
		Headers:  make(map[string]string),
	}

	// Parse headers
	for _, header := range msg.Payload.Headers {
		email.Headers[header.Name] = header.Value

		switch strings.ToLower(header.Name) {
		case "from":
			email.From = header.Value
		case "to":
			email.To = strings.Split(header.Value, ",")
			for i, to := range email.To {
				email.To[i] = strings.TrimSpace(to)
			}
		case "subject":
			email.Subject = header.Value
		case "date":
			if date, err := time.Parse(time.RFC1123Z, header.Value); err == nil {
				email.Date = date
			} else if date, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", header.Value); err == nil {
				email.Date = date
			}
		}
	}

	// Check if message is read
	email.IsRead = true
	for _, label := range msg.LabelIds {
		if label == "UNREAD" {
			email.IsRead = false
			break
		}
	}

	// Extract body content
	email.Body, email.HTML = g.extractBody(msg.Payload)

	// Extract attachments
	email.Attachments = g.extractAttachments(msg.Payload)

	return email, nil
}

// extractBody extracts text and HTML body from Gmail message payload
func (g *GmailReader) extractBody(payload *gmail.MessagePart) (text, html string) {
	if payload.Body != nil && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			content := string(decoded)
			if payload.MimeType == "text/plain" {
				text = content
			} else if payload.MimeType == "text/html" {
				html = content
			}
		}
	}

	// Recursively check parts
	for _, part := range payload.Parts {
		partText, partHTML := g.extractBody(part)
		if partText != "" && text == "" {
			text = partText
		}
		if partHTML != "" && html == "" {
			html = partHTML
		}
	}

	return text, html
}

// extractAttachments extracts attachments from Gmail message payload
func (g *GmailReader) extractAttachments(payload *gmail.MessagePart) []Attachment {
	var attachments []Attachment

	if payload.Filename != "" && payload.Body != nil && payload.Body.AttachmentId != "" {
		attachment := Attachment{
			ID:       payload.Body.AttachmentId,
			Filename: payload.Filename,
			MimeType: payload.MimeType,
			Size:     payload.Body.Size,
		}
		attachments = append(attachments, attachment)
	}

	// Recursively check parts
	for _, part := range payload.Parts {
		attachments = append(attachments, g.extractAttachments(part)...)
	}

	return attachments
}

// getGmailClient retrieves an OAuth2 client for Gmail API
func getGmailClient(config *oauth2.Config, tokenPath string, logger *slog.Logger) (*http.Client, error) {
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		logger.Info("no existing token found, requesting new authorization")
		token, err = getTokenFromWeb(config, logger)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tokenPath, token); err != nil {
			logger.Warn("failed to save token", "error", err)
		}
	}

	return config.Client(context.Background(), token), nil
}

// getTokenFromWeb requests a token from the web using out-of-band flow
func getTokenFromWeb(config *oauth2.Config, logger *slog.Logger) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	logger.Info("=== Gmail OAuth Authorization Required ===")
	logger.Info("1. Go to the following link in your browser:")
	logger.Info(authURL)
	logger.Info("2. Sign in and authorize the application")
	logger.Info("3. Copy the authorization code displayed on the page")
	logger.Info("4. Paste it below and press Enter")
	fmt.Print("Enter authorization code: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	token, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %w", err)
	}
	return token, nil
}

// tokenFromFile retrieves a token from a local file
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// saveToken saves a token to a file path
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}
