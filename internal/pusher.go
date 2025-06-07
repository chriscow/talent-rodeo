package internal

import (
	"github.com/pusher/pusher-http-go/v5"
)

func SendPusherMessage(psh *pusher.Client, userID string, message string, err *string) error {
	if psh == nil {
		return nil // ignore silently if pusher is not configured
	}

	data := map[string]any{
		"message": message,
		"error":   err,
	}

	if err := psh.Trigger(userID, "server-message", data); err != nil {
		return err
	}

	return nil
}

func SendPusherRedirect(psh *pusher.Client, userID string, url string) error {
	if psh == nil {
		return nil // ignore silently if pusher is not configured
	}

	if err := psh.Trigger(userID, "server-redirect", map[string]string{"url": url}); err != nil {
		return err
	}

	return nil
}

func SendPusherProgress(psh *pusher.Client, userID string, message string, progress int, total int) error {
	if psh == nil {
		return nil // ignore silently if pusher is not configured
	}

	data := map[string]any{
		"message":  message,
		"progress": progress,
		"total":    total,
	}

	if err := psh.Trigger(userID, "server-progress", data); err != nil {
		return err
	}

	return nil
}
