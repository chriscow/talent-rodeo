package main

import (
	"net/http"
	"os"
	"strconv"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
)

func (app *application) showPusherTest(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data["PusherKey"] = os.Getenv("PUSHER_KEY")
	data["PusherCluster"] = os.Getenv("PUSHER_CLUSTER")
	// A test user ID, can be anything for this test
	data["TestUserID"] = "test-user-123"

	if err := response.Page(w, http.StatusOK, data, "pages/pusher-test.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) triggerPusherProgress(w http.ResponseWriter, r *http.Request) {
	userID := r.FormValue("userID")
	message := r.FormValue("message")
	progress := r.FormValue("progress")
	total := r.FormValue("total")

	progressInt, _ := strconv.Atoi(progress)
	totalInt, _ := strconv.Atoi(total)

	app.logger.Info("triggering pusher progress", "userID", userID, "message", message, "progress", progressInt, "total", totalInt)

	err := internal.SendPusherProgress(app.pusher, userID, message, progressInt, totalInt)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
