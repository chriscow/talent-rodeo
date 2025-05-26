package internal

import (
	"fmt"

	"github.com/pusher/pusher-http-go/v5"
)

func SendPusherMessage(psh *pusher.Client, userID string, message string, err *string) error {
	if psh == nil {
		fmt.Println("SendPusherMessage: pusher is not configured")
		return nil // ignore silently if pusher is not configured
	}

	data := map[string]any{
		"message": message,
		"error":   err,
	}

	if err := psh.Trigger(userID, "server-message", data); err != nil {
		fmt.Println("SendPusherMessage: failed to send message", err)
		return err
	}

	return nil
}

func SendPusherRedirect(psh *pusher.Client, userID string, url string) error {
	if psh == nil {
		fmt.Println("SendPusherRedirect: pusher is not configured")
		return nil // ignore silently if pusher is not configured
	}

	if err := psh.Trigger(userID, "server-redirect", map[string]string{"url": url}); err != nil {
		fmt.Println("SendPusherRedirect: failed to send redirect", err)
		return err
	}

	return nil
}
