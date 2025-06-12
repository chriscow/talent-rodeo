package internal

import (
	"fmt"
	"strings"

	"github.com/pusher/pusher-http-go/v5"
)

// SendPusherEvent sends a generic event to a user via Pusher.
func SendPusherEvent(psh *pusher.Client, userID string, eventName string, data map[string]any) error {
	if psh == nil {
		return nil // ignore silently if pusher is not configured
	}
	if userID == "" || eventName == "" {
		return nil // silently ignore invalid calls
	}
	if err := psh.Trigger(userID, eventName, data); err != nil {
		return fmt.Errorf("SendPusherEvent: pusher.Trigger for event '%s': %w", eventName, err)
	}
	return nil
}

func SendPusherMessage(psh *pusher.Client, userID string, message string, err *string) error {
	fmt.Println(Gray("SendPusherMessage"), userID, message, err)
	data := map[string]any{
		"message": message,
		"error":   err,
	}
	return SendPusherEvent(psh, userID, "server-message", data)
}

func SendPusherRedirect(psh *pusher.Client, userID string, url string) error {
	fmt.Println(Gray("SendPusherRedirect"), userID, url)
	data := map[string]any{
		"url": url,
	}
	return SendPusherEvent(psh, userID, "server-redirect", data)
}

func SendPusherProgress(psh *pusher.Client, userID string, eventName string, message string, progress int, total int) error {
	fmt.Println(Gray("SendPusherProgress"), userID, message, progress, total)
	data := map[string]any{
		"message":  message,
		"progress": progress,
		"total":    total,
	}
	return SendPusherEvent(psh, userID, eventName, data)
}

// IsPusherUserPresent checks if a user is currently subscribed to their channel.
func IsPusherUserPresent(psh *pusher.Client, userID string) (bool, error) {
	if psh == nil {
		return false, fmt.Errorf("pusher client is nil")
	}
	if userID == "" {
		return false, fmt.Errorf("user ID is required")
	}

	/*
		type ChannelParams struct {
			// Info is comma-separated vales of `"user_count"`, for
			// presence-channels, and `"subscription_count"`, for all-channels.
			// Note that the subscription count is not allowed by default. Please
			// contact us at http://support.pusher.com if you wish to enable this.
			// Pass in `nil` if you do not wish to specify any query attributes.
			Info *string
		}
	*/

	info := "user_count"
	params := pusher.ChannelParams{
		Info: &info,
	}

	channel, err := psh.Channel(fmt.Sprintf("presence-%s", userID), params)
	if err != nil {
		// If channel doesn't exist, it means user is not subscribed
		if strings.Contains(err.Error(), "404 Not Found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to get channel state: %w", err)
	}

	return channel.UserCount > 0, nil
}
