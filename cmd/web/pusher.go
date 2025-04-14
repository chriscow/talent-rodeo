package main

import (
	"fmt"
	"talent-rodeo/internal"
)

// sendUserStateUpdate sends a real-time notification about user state changes via Pusher
func (app *application) SendUserStateUpdate(user *internal.User) {
	data := map[string]any{
		"state":       user.State.Status,
		"status_text": user.State.Status.String(),
		"description": user.State.Status.Description(),
		"error":       user.State.Error,
	}

	app.logger.Debug("Sending user state update notification", "user_id", user.ID, "state", user.State.Status)
	err := app.pusher.Trigger(fmt.Sprintf("user-%s", user.ID), "state-update", data)
	if err != nil {
		app.logger.Error("Failed to send Pusher notification", "error", err, "user_id", user.ID)
	} else {
		app.logger.Info("Sent user state update notification", "user_id", user.ID, "state", user.State.Status)
	}
}
